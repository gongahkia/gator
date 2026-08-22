// Package hooks runs explicitly trusted repository lifecycle hooks.
//
// Project hooks are executable code. A repository's tracked manifest and every
// declared executable are hashed as one bundle, and Gator activates the bundle
// only when that exact hash is present in user-owned settings.
package hooks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workspace"
)

const (
	manifestPath    = ".gator/hooks.json"
	manifestVersion = 1
	maxManifest     = 64 * 1024
	maxHooks        = 64
	maxOutput       = 32 * 1024
)

// Event is one lifecycle boundary where a hook may run.
type Event string

const (
	PreToolUse     Event = "pre_tool_use"
	PostToolUse    Event = "post_tool_use"
	PreCompaction  Event = "pre_compaction"
	PostCompaction Event = "post_compaction"
	SessionSave    Event = "session_save"
	Verification   Event = "verification"
)

// Trust is persisted in user-owned config after `gator hook trust`. Repository
// paths are canonical, and Hash is the exact active hook-bundle digest.
type Trust struct {
	Repository string `json:"repository"`
	Hash       string `json:"hash"`
}

type manifest struct {
	Version int        `json:"version"`
	Hooks   []hookSpec `json:"hooks"`
}

type hookSpec struct {
	Name    string   `json:"name"`
	Event   Event    `json:"event"`
	Tool    string   `json:"tool,omitempty"`
	Command []string `json:"command"`
	Timeout int      `json:"timeout_seconds,omitempty"`
}

type activeHook struct {
	spec    hookSpec
	command string
}

// Engine is a prepared hook set for a single worktree. A configured but
// untrusted engine is inert; callers can expose its Status to the developer.
type Engine struct {
	configuredHash string
	trusted        bool
	root           workspace.Root
	hooks          []activeHook
	Emit           func(Status)
}

// Status is a bounded, non-sensitive lifecycle record suitable for journals
// and terminal output. Raw hook stdin and stdout remain private to the hook.
type Status struct {
	Event   Event
	Hook    string
	Message string
}

// Hash returns the configured hook bundle's exact digest, or empty when a
// repository has no hooks manifest.
func (e Engine) Hash() string { return e.configuredHash }

// Configured reports whether the worktree contains a valid hooks manifest.
func (e Engine) Configured() bool { return e.configuredHash != "" }

// Trusted reports whether the configured bundle matched user-owned trust.
func (e Engine) Trusted() bool { return e.trusted }

// Load resolves an isolated worktree's hooks while matching trust against its
// canonical source repository identity. An untrusted but valid manifest is not
// an error: it is intentionally disabled until an explicit trust action.
func Load(worktreePath, repository, trustedHash string) (Engine, error) {
	root, err := workspace.Open(worktreePath)
	if err != nil {
		return Engine{}, err
	}
	loaded, digest, err := loadManifest(root)
	if err != nil {
		return Engine{}, err
	}
	if digest == "" {
		return Engine{root: root}, nil
	}
	engine := Engine{configuredHash: digest, root: root}
	if strings.TrimSpace(trustedHash) == "" || trustedHash != digest {
		return engine, nil
	}
	if strings.TrimSpace(repository) == "" {
		return Engine{}, errors.New("hook repository identity is required")
	}
	engine.trusted = true
	engine.hooks = loaded
	return engine, nil
}

// BundleHash validates and hashes a repository's hook configuration. It is
// used by the explicit trust command and never executes hook code.
func BundleHash(repository string) (string, error) {
	root, err := workspace.Open(repository)
	if err != nil {
		return "", err
	}
	_, digest, err := loadManifest(root)
	return digest, err
}

func loadManifest(root workspace.Root) ([]activeHook, string, error) {
	contents, err := root.ReadRegularFile(manifestPath, maxManifest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("read %s: %w", manifestPath, err)
	}
	var document manifest
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, "", fmt.Errorf("decode %s: %w", manifestPath, err)
	}
	if err := requireEOF(decoder); err != nil {
		return nil, "", fmt.Errorf("decode %s: %w", manifestPath, err)
	}
	if document.Version != manifestVersion {
		return nil, "", fmt.Errorf("unsupported hooks manifest version %d", document.Version)
	}
	if len(document.Hooks) > maxHooks {
		return nil, "", fmt.Errorf("hooks manifest contains more than %d hooks", maxHooks)
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(manifestPath + "\x00"))
	_, _ = digest.Write(contents)
	seenNames := make(map[string]struct{}, len(document.Hooks))
	seenCommands := make(map[string][]byte)
	loaded := make([]activeHook, 0, len(document.Hooks))
	for index, spec := range document.Hooks {
		if err := validateSpec(spec); err != nil {
			return nil, "", fmt.Errorf("hooks manifest hook %d: %w", index+1, err)
		}
		if _, exists := seenNames[spec.Name]; exists {
			return nil, "", fmt.Errorf("hooks manifest repeats hook name %q", spec.Name)
		}
		seenNames[spec.Name] = struct{}{}
		command := filepath.ToSlash(spec.Command[0])
		contents, exists := seenCommands[command]
		if !exists {
			contents, err = executableContents(root, command)
			if err != nil {
				return nil, "", fmt.Errorf("hooks manifest hook %q: %w", spec.Name, err)
			}
			seenCommands[command] = contents
		}
		loaded = append(loaded, activeHook{spec: spec, command: command})
	}
	commands := make([]string, 0, len(seenCommands))
	for command := range seenCommands {
		commands = append(commands, command)
	}
	sort.Strings(commands)
	for _, command := range commands {
		_, _ = digest.Write([]byte("\x00" + command + "\x00"))
		_, _ = digest.Write(seenCommands[command])
	}
	return loaded, hex.EncodeToString(digest.Sum(nil)), nil
}

func validateSpec(spec hookSpec) error {
	if strings.TrimSpace(spec.Name) == "" || len(spec.Name) > 128 || strings.ContainsAny(spec.Name, "\r\n") {
		return errors.New("hook name is required and must be at most 128 bytes")
	}
	switch spec.Event {
	case PreToolUse, PostToolUse, PreCompaction, PostCompaction, SessionSave, Verification:
	default:
		return fmt.Errorf("unsupported hook event %q", spec.Event)
	}
	if spec.Event != PreToolUse && spec.Event != PostToolUse && strings.TrimSpace(spec.Tool) != "" {
		return errors.New("tool matching is valid only for tool lifecycle hooks")
	}
	if len(spec.Command) == 0 || len(spec.Command) > 32 || strings.TrimSpace(spec.Command[0]) == "" {
		return errors.New("hook command requires 1-32 argv entries")
	}
	if filepath.IsAbs(spec.Command[0]) || !safeRelativePath(spec.Command[0]) {
		return errors.New("hook executable must be a repository-relative path")
	}
	for _, value := range spec.Command {
		if len(value) > 4096 || strings.ContainsRune(value, '\x00') {
			return errors.New("hook command argument is invalid")
		}
	}
	if spec.Timeout < 0 || spec.Timeout > 120 {
		return errors.New("hook timeout_seconds must be 1-120 when set")
	}
	return nil
}

func safeRelativePath(value string) bool {
	clean := filepath.Clean(value)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func executableContents(root workspace.Root, relative string) ([]byte, error) {
	resolved, err := root.ResolveFile(filepath.FromSlash(relative))
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return nil, errors.New("hook executable must be an executable regular file")
	}
	return root.ReadRegularFile(filepath.FromSlash(relative), maxOutput)
}

// Run invokes all trusted hooks matching event and tool. Payload is serialized
// to bounded JSON on stdin; hook output must be one bounded JSON decision.
func (e Engine) Run(ctx context.Context, event Event, tool string, payload any) error {
	if !e.trusted {
		return nil
	}
	input, err := json.Marshal(struct {
		Version int    `json:"version"`
		Event   Event  `json:"event"`
		Tool    string `json:"tool,omitempty"`
		Payload any    `json:"payload,omitempty"`
	}{Version: 1, Event: event, Tool: tool, Payload: payload})
	if err != nil {
		return fmt.Errorf("encode hook %s input: %w", event, err)
	}
	if len(input) > maxOutput {
		return fmt.Errorf("hook %s input exceeds %d bytes", event, maxOutput)
	}
	for _, hook := range e.hooks {
		if hook.spec.Event != event || (hook.spec.Tool != "" && hook.spec.Tool != tool) {
			continue
		}
		if err := e.runOne(ctx, hook, input); err != nil {
			return err
		}
	}
	return nil
}

func (e Engine) runOne(ctx context.Context, hook activeHook, input []byte) error {
	timeout := 30 * time.Second
	if hook.spec.Timeout > 0 {
		timeout = time.Duration(hook.spec.Timeout) * time.Second
	}
	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	argv := append([]string{filepath.Join(e.root.Path(), filepath.FromSlash(hook.command))}, hook.spec.Command[1:]...)
	prepared, err := sandbox.Prepare(runContext, sandbox.Request{Dir: e.root.Path(), Argv: argv, Policy: sandbox.DefaultPolicy()})
	if err != nil {
		return fmt.Errorf("prepare hook %q: %w", hook.spec.Name, err)
	}
	defer prepared.Cleanup()
	output := &limitedBuffer{limit: maxOutput}
	prepared.Command.Stdin = bytes.NewReader(input)
	prepared.Command.Stdout = output
	prepared.Command.Stderr = output
	if err := prepared.Command.Run(); err != nil {
		if errors.Is(runContext.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("hook %q timed out", hook.spec.Name)
		}
		return fmt.Errorf("hook %q failed: %s", hook.spec.Name, strings.TrimSpace(output.String()))
	}
	if output.truncated {
		return fmt.Errorf("hook %q response exceeds %d bytes", hook.spec.Name, maxOutput)
	}
	var response struct {
		Decision string `json:"decision"`
		Message  string `json:"message,omitempty"`
	}
	decoder := json.NewDecoder(strings.NewReader(output.String()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("decode hook %q response: %w", hook.spec.Name, err)
	}
	if err := requireEOF(decoder); err != nil {
		return fmt.Errorf("decode hook %q response: %w", hook.spec.Name, err)
	}
	switch response.Decision {
	case "allow":
		e.emit(Status{Event: hook.spec.Event, Hook: hook.spec.Name, Message: response.Message})
		return nil
	case "deny":
		e.emit(Status{Event: hook.spec.Event, Hook: hook.spec.Name, Message: response.Message})
		if strings.TrimSpace(response.Message) == "" {
			return fmt.Errorf("hook %q denied %s", hook.spec.Name, hook.spec.Event)
		}
		return fmt.Errorf("hook %q denied %s: %s", hook.spec.Name, hook.spec.Event, response.Message)
	default:
		return fmt.Errorf("hook %q returned invalid decision %q", hook.spec.Name, response.Decision)
	}
}

func (e Engine) emit(status Status) {
	if e.Emit != nil {
		e.Emit(status)
	}
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(value), nil
	}
	if len(value) > remaining {
		_, _ = b.buffer.Write(value[:remaining])
		b.truncated = true
		return len(value), nil
	}
	_, _ = b.buffer.Write(value)
	return len(value), nil
}

func (b *limitedBuffer) String() string { return b.buffer.String() }

func requireEOF(decoder *json.Decoder) error {
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected second JSON value")
		}
		return err
	}
	return nil
}
