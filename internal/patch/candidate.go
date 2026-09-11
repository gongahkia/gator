package patch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workspace"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Verification struct {
	Argv       []string `json:"argv"`
	Passed     bool     `json:"passed"`
	Diagnostic string   `json:"diagnostic,omitempty"`
}
type Candidate struct {
	Before       map[string]string `json:"before"`
	PatchPath    string            `json:"patch_path,omitempty"`
	Version      int               `json:"version"`
	ID           string            `json:"id"`
	Baseline     string            `json:"baseline"`
	Patches      []string          `json:"patches"`
	ChangedPaths []string          `json:"changed_paths"`
	SHA256       string            `json:"sha256"`
	Status       string            `json:"status"`
	Error        string            `json:"error,omitempty"`
	Verification []Verification    `json:"verification"`
	Patch        []byte            `json:"-"`
}

// InitSnapshot copies a frozen tree into a private Git repository.
func InitSnapshot(ctx context.Context, source, destination string) error {
	if err := os.Mkdir(destination, 0700); err != nil {
		return err
	}
	root, err := workspace.Open(source)
	if err != nil {
		return err
	}
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if entry.Name() == ".git" && entry.IsDir() {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("snapshot contains symlink")
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		if !entry.Type().IsRegular() {
			return errors.New("snapshot contains special file")
		}
		data, err := root.ReadRegularFile(relative, 100*1024*1024)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm()|0200)
	})
	if err != nil {
		return err
	}
	for _, argv := range [][]string{{"init", "--quiet"}, {"add", "--all", "--force"}, {"-c", "user.name=Gator", "-c", "user.email=gator@invalid", "commit", "--quiet", "--allow-empty", "-m", "frozen source"}} {
		if _, err := gitOutput(ctx, destination, argv...); err != nil {
			return err
		}
	}
	return nil
}

func Integrate(ctx context.Context, source, destination string, patches [][]byte, names []string, verification [][]string, policy sandbox.Policy) (Candidate, error) {
	c := Candidate{Version: 1, Patches: append([]string(nil), names...), Status: "failed"}
	fail := func(err error) (Candidate, error) { c.Error = err.Error(); return c, err }
	if len(patches) == 0 || len(patches) > 16 || len(patches) != len(names) {
		return fail(errors.New("select between 1 and 16 patches"))
	}
	if err := InitSnapshot(ctx, source, destination); err != nil {
		return fail(err)
	}
	base, err := gitOutput(ctx, destination, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return fail(err)
	}
	c.Baseline = string(base)
	hash := sha256.New()
	hash.Write(base)
	for i, payload := range patches {
		hash.Write(payload)
		if err := applyPatch(ctx, destination, payload, true); err != nil {
			return fail(fmt.Errorf("conflicting patch %s: %w", names[i], err))
		}
		if err := applyPatch(ctx, destination, payload, false); err != nil {
			return fail(err)
		}
	}
	c.ID = "candidate-" + hex.EncodeToString(hash.Sum(nil))[:24]
	c.ChangedPaths, err = Paths(ctx, destination, "HEAD")
	if err != nil {
		return fail(err)
	}
	c.Before = map[string]string{}
	sourceRoot, err := workspace.Open(source)
	if err != nil {
		return fail(err)
	}
	for _, path := range c.ChangedPaths {
		data, err := sourceRoot.ReadRegularFile(path, 100*1024*1024)
		if errors.Is(err, os.ErrNotExist) {
			c.Before[path] = "absent"
			continue
		}
		if err != nil {
			return fail(err)
		}
		sum := sha256.Sum256(data)
		c.Before[path] = hex.EncodeToString(sum[:])
	}
	c.Patch, err = Export(ctx, destination, "HEAD")
	if err != nil {
		return fail(err)
	}
	sum := sha256.Sum256(c.Patch)
	c.SHA256 = hex.EncodeToString(sum[:])
	commands := append([][]string{{"git", "diff", "--check"}}, verification...)
	for _, argv := range commands {
		commandCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		prepared, err := sandbox.Prepare(commandCtx, sandbox.Request{Dir: destination, Argv: argv, Policy: policy})
		var output []byte
		if err == nil {
			output, _, err = runCommand(prepared.Command)
			prepared.Cleanup()
		}
		cancel()
		diagnostic := string(output)
		if len(diagnostic) > 4096 {
			diagnostic = diagnostic[:4096]
		}
		c.Verification = append(c.Verification, Verification{Argv: append([]string(nil), argv...), Passed: err == nil, Diagnostic: diagnostic})
		if err != nil {
			if message := strings.TrimSpace(diagnostic); message != "" {
				return fail(fmt.Errorf("combined verification failed: %w: %s", err, message))
			}
			return fail(fmt.Errorf("combined verification failed: %w", err))
		}
	}
	after, err := Export(ctx, destination, "HEAD")
	if err != nil {
		return fail(err)
	}
	check := sha256.Sum256(after)
	if hex.EncodeToString(check[:]) != c.SHA256 {
		return fail(errors.New("verifier changed candidate files"))
	}
	c.Status = "verified"
	return c, nil
}

// ApplyBytes performs explicit preflight and applies a selected sealed code patch.
func ApplyBytes(ctx context.Context, target string, payload []byte, checkOnly bool) error {
	if len(payload) == 0 || len(payload) > maxPatchBytes {
		return errors.New("invalid patch size")
	}
	root, err := workspace.Open(target)
	if err != nil {
		return err
	}
	if err := requireClean(ctx, root.Path()); err != nil {
		return err
	}
	if err := applyPatch(ctx, root.Path(), payload, true); err != nil {
		return err
	}
	if checkOnly {
		return nil
	}
	return applyPatch(ctx, root.Path(), payload, false)
}

// ApplyCandidate checks exact changed-file baselines as well as Git patch compatibility.
func ApplyCandidate(ctx context.Context, target string, candidate Candidate, payload []byte, checkOnly bool) error {
	if candidate.Version != 1 || candidate.Status != "verified" || len(candidate.Before) != len(candidate.ChangedPaths) {
		return errors.New("candidate has no verified baseline")
	}
	sum := sha256.Sum256(payload)
	if hex.EncodeToString(sum[:]) != candidate.SHA256 {
		return errors.New("candidate patch digest mismatch")
	}
	root, err := workspace.Open(target)
	if err != nil {
		return err
	}
	for _, path := range candidate.ChangedPaths {
		data, err := root.ReadRegularFile(path, 100*1024*1024)
		expected := candidate.Before[path]
		if expected == "absent" && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("target conflict at %s: %w", path, err)
		}
		sum := sha256.Sum256(data)
		if expected != hex.EncodeToString(sum[:]) {
			return fmt.Errorf("target conflict: %s changed from the reviewed baseline", path)
		}
	}
	return ApplyBytes(ctx, target, payload, checkOnly)
}
