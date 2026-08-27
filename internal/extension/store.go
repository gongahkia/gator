package extension

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Store owns installed extension bundles below one data directory.
type Store struct {
	path string
}

// ResolveDataDir follows XDG data placement. GATOR_DATA_DIR is an explicit
// override for managed installs and tests.
func ResolveDataDir(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return absoluteDirectory(override)
	}
	if configured := os.Getenv("GATOR_DATA_DIR"); strings.TrimSpace(configured) != "" {
		return absoluteDirectory(configured)
	}
	if configured := os.Getenv("XDG_DATA_HOME"); strings.TrimSpace(configured) != "" {
		return absoluteDirectory(configured)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user data directory: %w", err)
	}
	return filepath.Join(home, ".local", "share"), nil
}

// NewStore creates a store below dataRoot.
func NewStore(dataRoot string) (Store, error) {
	if strings.TrimSpace(dataRoot) == "" {
		return Store{}, errors.New("extension data directory is required")
	}
	root, err := absoluteDirectory(dataRoot)
	if err != nil {
		return Store{}, err
	}
	return Store{path: filepath.Join(root, "gator", "extensions")}, nil
}

// DefaultStore opens Gator's standard extension data store.
func DefaultStore() (Store, error) {
	directory, err := ResolveDataDir("")
	if err != nil {
		return Store{}, err
	}
	return NewStore(directory)
}

// Path returns the installation directory without creating it.
func (s Store) Path() string { return s.path }

// List loads each installed extension in stable ID order.
func (s Store) List() ([]Installed, error) {
	entries, err := os.ReadDir(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list installed extensions: %w", err)
	}
	installed := make([]Installed, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("installed extension %q must be a real directory", entry.Name())
		}
		candidate, err := load(filepath.Join(s.path, entry.Name()), false)
		if err != nil {
			return nil, err
		}
		if candidate.Manifest.ID != entry.Name() {
			return nil, fmt.Errorf("installed extension directory %q does not match manifest ID %q", entry.Name(), candidate.Manifest.ID)
		}
		installed = append(installed, candidate)
	}
	sort.Slice(installed, func(first, second int) bool { return installed[first].Manifest.ID < installed[second].Manifest.ID })
	return installed, nil
}

// Install copies a local extension directory into the global store. Replacing
// an existing bundle is explicit because its executable tools are trusted code.
func (s Store) Install(source string, replace bool) (Installed, error) {
	if strings.TrimSpace(s.path) == "" {
		return Installed{}, errors.New("extension store is not initialized")
	}
	info, err := os.Lstat(source)
	if err != nil {
		return Installed{}, fmt.Errorf("stat extension source: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Installed{}, errors.New("extension source must be a real directory")
	}
	root, err := filepath.Abs(source)
	if err != nil {
		return Installed{}, fmt.Errorf("resolve extension source: %w", err)
	}
	manifest, err := load(root, false)
	if err != nil {
		return Installed{}, err
	}
	if err := os.MkdirAll(s.path, 0o700); err != nil {
		return Installed{}, fmt.Errorf("create extension store: %w", err)
	}
	target := filepath.Join(s.path, manifest.Manifest.ID)
	if _, err := os.Lstat(target); err == nil && !replace {
		return Installed{}, fmt.Errorf("extension %q is already installed; use --replace to replace it", manifest.Manifest.ID)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Installed{}, fmt.Errorf("inspect installed extension: %w", err)
	}
	temporary, err := os.MkdirTemp(s.path, ".install-")
	if err != nil {
		return Installed{}, fmt.Errorf("create extension staging directory: %w", err)
	}
	defer os.RemoveAll(temporary)
	if err := copyTree(root, temporary); err != nil {
		return Installed{}, err
	}
	if replace {
		if err := os.RemoveAll(target); err != nil {
			return Installed{}, fmt.Errorf("replace extension %q: %w", manifest.Manifest.ID, err)
		}
	}
	if err := os.Rename(temporary, target); err != nil {
		return Installed{}, fmt.Errorf("publish extension %q: %w", manifest.Manifest.ID, err)
	}
	return load(target, false)
}

// InstallSource installs either a local bundle directory or an explicit Git
// repository URL. Re-running with --replace is the intentional package-update
// mechanism; Gator never refreshes executable extensions in the background.
func (s Store) InstallSource(ctx context.Context, source string, replace bool) (Installed, error) {
	info, err := os.Stat(source)
	if err == nil {
		if !info.IsDir() {
			return Installed{}, errors.New("extension source must be a directory or Git repository URL")
		}
		return s.Install(source, replace)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Installed{}, fmt.Errorf("inspect extension source: %w", err)
	}
	if !validGitSource(source) {
		return Installed{}, errors.New("extension source must be a local directory or an https, ssh, or git@ repository URL")
	}
	temporary, err := os.MkdirTemp("", "gator-extension-clone-")
	if err != nil {
		return Installed{}, fmt.Errorf("create extension clone directory: %w", err)
	}
	defer os.RemoveAll(temporary)
	checkout := filepath.Join(temporary, "source")
	command := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--", source, checkout)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return Installed{}, fmt.Errorf("clone extension source: %w", err)
		}
		return Installed{}, fmt.Errorf("clone extension source: %s", message)
	}
	return s.Install(checkout, replace)
}

// Prepared is display-safe metadata for a staged, not-yet-installed bundle.
// It deliberately omits the staging path, file list, and prompt bodies: the
// hash is the object a reviewer confirms, not untrusted bundle text.
type Prepared struct {
	Token            string
	ID               string
	Name             string
	Description      string
	Hash             string
	Skills           int
	Prompts          int
	Commands         int
	UI               int
	Tools            int
	AlreadyInstalled bool
}

const preparedPrefix = ".prepare-"

// PrepareSource snapshots a local directory or clones an HTTPS/SSH Git source
// exactly once into a private staging directory and validates it. Nothing is
// published and no configuration changes until CommitPrepared runs with the
// reviewed hash, so the bytes a developer approves are the bytes installed.
func (s Store) PrepareSource(ctx context.Context, source string) (Prepared, error) {
	if strings.TrimSpace(s.path) == "" {
		return Prepared{}, errors.New("extension store is not initialized")
	}
	if err := os.MkdirAll(s.path, 0o700); err != nil {
		return Prepared{}, fmt.Errorf("create extension store: %w", err)
	}
	staging, err := os.MkdirTemp(s.path, preparedPrefix)
	if err != nil {
		return Prepared{}, fmt.Errorf("create extension staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(staging)
		}
	}()
	if err := os.Chmod(staging, 0o700); err != nil {
		return Prepared{}, fmt.Errorf("secure extension staging directory: %w", err)
	}
	root, err := s.materializeSource(ctx, source, staging)
	if err != nil {
		return Prepared{}, err
	}
	candidate, err := load(root, false)
	if err != nil {
		return Prepared{}, err
	}
	installed := false
	if _, err := os.Lstat(filepath.Join(s.path, candidate.Manifest.ID)); err == nil {
		installed = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return Prepared{}, fmt.Errorf("inspect installed extension: %w", err)
	}
	published = true
	return Prepared{
		Token:            filepath.Base(staging),
		ID:               candidate.Manifest.ID,
		Name:             candidate.Manifest.Name,
		Description:      candidate.Manifest.Description,
		Hash:             candidate.Hash,
		Skills:           len(candidate.Manifest.Skills),
		Prompts:          len(candidate.Manifest.Prompts),
		Commands:         len(candidate.Manifest.Commands),
		UI:               len(candidate.Manifest.UI),
		Tools:            len(candidate.Manifest.Tools),
		AlreadyInstalled: installed,
	}, nil
}

// materializeSource copies a local bundle or clones a remote repository into
// staging exactly once. The returned root is the validated bundle directory.
func (s Store) materializeSource(ctx context.Context, source, staging string) (string, error) {
	bundle := filepath.Join(staging, "bundle")
	info, err := os.Stat(source)
	if err == nil {
		if !info.IsDir() {
			return "", errors.New("extension source must be a directory or Git repository URL")
		}
		local, err := os.Lstat(source)
		if err != nil {
			return "", fmt.Errorf("stat extension source: %w", err)
		}
		if !local.IsDir() || local.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("extension source must be a real directory")
		}
		absolute, err := filepath.Abs(source)
		if err != nil {
			return "", fmt.Errorf("resolve extension source: %w", err)
		}
		if err := os.MkdirAll(bundle, 0o700); err != nil {
			return "", fmt.Errorf("create extension staging directory: %w", err)
		}
		if err := copyTree(absolute, bundle); err != nil {
			return "", err
		}
		return bundle, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect extension source: %w", err)
	}
	if !validGitSource(source) {
		return "", errors.New("extension source must be a local directory or an https, ssh, or git@ repository URL")
	}
	checkout := filepath.Join(staging, "checkout")
	command := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--", source, checkout)
	if output, err := command.CombinedOutput(); err != nil {
		if message := strings.TrimSpace(string(output)); message != "" {
			return "", fmt.Errorf("clone extension source: %s", message)
		}
		return "", fmt.Errorf("clone extension source: %w", err)
	}
	if err := os.MkdirAll(bundle, 0o700); err != nil {
		return "", fmt.Errorf("create extension staging directory: %w", err)
	}
	if err := copyTree(checkout, bundle); err != nil {
		return "", err
	}
	if err := os.RemoveAll(checkout); err != nil {
		return "", fmt.Errorf("discard extension clone: %w", err)
	}
	return bundle, nil
}

// CommitPrepared publishes exactly the staged bytes whose hash was reviewed.
// It re-reads and re-hashes the staging directory, so a bundle mutated after
// review, or a source that changed between preparation and confirmation, can
// never be installed under an approved hash.
func (s Store) CommitPrepared(token, expectedHash string, replace bool) (Installed, error) {
	staging, err := s.preparedPath(token)
	if err != nil {
		return Installed{}, err
	}
	expectedHash = strings.TrimSpace(expectedHash)
	if expectedHash == "" {
		return Installed{}, errors.New("commit requires the reviewed extension bundle hash")
	}
	candidate, err := load(filepath.Join(staging, "bundle"), false)
	if err != nil {
		return Installed{}, err
	}
	if candidate.Hash != expectedHash {
		_ = os.RemoveAll(staging)
		return Installed{}, errors.New("staged extension no longer matches the reviewed bundle hash")
	}
	target := filepath.Join(s.path, candidate.Manifest.ID)
	if _, err := os.Lstat(target); err == nil {
		if !replace {
			return Installed{}, fmt.Errorf("extension %q is already installed; confirm replacement to overwrite it", candidate.Manifest.ID)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Installed{}, fmt.Errorf("inspect installed extension: %w", err)
	}
	if replace {
		if err := os.RemoveAll(target); err != nil {
			return Installed{}, fmt.Errorf("replace extension %q: %w", candidate.Manifest.ID, err)
		}
	}
	if err := os.Rename(filepath.Join(staging, "bundle"), target); err != nil {
		return Installed{}, fmt.Errorf("publish extension %q: %w", candidate.Manifest.ID, err)
	}
	if err := os.RemoveAll(staging); err != nil {
		return Installed{}, fmt.Errorf("discard extension staging directory: %w", err)
	}
	published, err := load(target, false)
	if err != nil {
		return Installed{}, err
	}
	if published.Hash != expectedHash {
		return Installed{}, errors.New("published extension does not match the reviewed bundle hash")
	}
	return published, nil
}

// DiscardPrepared removes one staged bundle. Cancelling a review, navigating
// away, and shutting the session down all discard staged executable code.
func (s Store) DiscardPrepared(token string) error {
	staging, err := s.preparedPath(token)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("discard extension staging directory: %w", err)
	}
	return nil
}

func (s Store) preparedPath(token string) (string, error) {
	if strings.TrimSpace(s.path) == "" {
		return "", errors.New("extension store is not initialized")
	}
	if !strings.HasPrefix(token, preparedPrefix) || token != filepath.Base(token) || strings.ContainsAny(token, `/\`) {
		return "", errors.New("invalid staged extension token")
	}
	staging := filepath.Join(s.path, token)
	info, err := os.Lstat(staging)
	if errors.Is(err, os.ErrNotExist) {
		return "", errors.New("staged extension is no longer available; prepare the source again")
	}
	if err != nil {
		return "", fmt.Errorf("inspect staged extension: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("staged extension must be a real directory")
	}
	return staging, nil
}

func validGitSource(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "-") {
		return false
	}
	if strings.HasPrefix(value, "git@") {
		return true
	}
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "ssh") && parsed.Host != ""
}

// Remove deletes exactly one globally installed extension. Callers must make
// deletion explicit in their UI or command flow.
func (s Store) Remove(id string) error {
	if !extensionIDPattern.MatchString(id) {
		return fmt.Errorf("invalid extension ID %q", id)
	}
	target := filepath.Join(s.path, id)
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("extension %q is not installed", id)
	}
	if err != nil {
		return fmt.Errorf("inspect extension %q: %w", id, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("installed extension %q is not a real directory", id)
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("remove extension %q: %w", id, err)
	}
	return nil
}

func load(root string, project bool) (Installed, error) {
	manifestPath := filepath.Join(root, manifestName)
	info, err := os.Lstat(manifestPath)
	if err != nil {
		return Installed{}, fmt.Errorf("read extension manifest: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxManifestBytes {
		return Installed{}, fmt.Errorf("extension manifest must be a regular file no larger than %d bytes", maxManifestBytes)
	}
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		return Installed{}, fmt.Errorf("read extension manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return Installed{}, fmt.Errorf("decode extension manifest: %w", err)
	}
	if err := validateManifest(manifest); err != nil {
		return Installed{}, err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Installed{}, fmt.Errorf("resolve extension root: %w", err)
	}
	digest, err := extensionHash(abs)
	if err != nil {
		return Installed{}, fmt.Errorf("hash extension bundle: %w", err)
	}
	return Installed{Manifest: manifest, Root: abs, Project: project, Hash: digest}, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.Version != manifestVersion {
		return fmt.Errorf("unsupported extension manifest version %d", manifest.Version)
	}
	if !extensionIDPattern.MatchString(manifest.ID) {
		return fmt.Errorf("invalid extension ID %q", manifest.ID)
	}
	if strings.TrimSpace(manifest.Name) == "" || len(manifest.Name) > 128 {
		return errors.New("extension name is required and must be at most 128 bytes")
	}
	if len(manifest.Description) > 1024 || len(manifest.Skills)+len(manifest.Prompts)+len(manifest.Commands)+len(manifest.UI) > 64 || len(manifest.Commands) > 32 || len(manifest.UI) > 16 || len(manifest.Tools) > 32 {
		return errors.New("extension manifest exceeds a resource limit")
	}
	resources := make(map[string]struct{}, len(manifest.Skills)+len(manifest.Prompts)+len(manifest.Commands))
	for _, resource := range append(append([]string(nil), manifest.Skills...), manifest.Prompts...) {
		if _, err := safePath("extension", resource); err != nil {
			return fmt.Errorf("invalid extension resource %q: %w", resource, err)
		}
		if _, found := resources[resource]; found {
			return fmt.Errorf("extension resource %q is declared more than once", resource)
		}
		resources[resource] = struct{}{}
	}
	commands := make(map[string]struct{}, len(manifest.Commands))
	for _, command := range manifest.Commands {
		if !toolNamePattern.MatchString(command.Name) {
			return fmt.Errorf("invalid extension command name %q", command.Name)
		}
		if strings.TrimSpace(command.Description) == "" || len(command.Description) > 1024 {
			return fmt.Errorf("extension command %q requires a description no longer than 1024 bytes", command.Name)
		}
		if _, err := safePath("extension", command.Prompt); err != nil {
			return fmt.Errorf("extension command %q prompt: %w", command.Name, err)
		}
		if _, exists := commands[command.Name]; exists {
			return fmt.Errorf("extension command %q is declared more than once", command.Name)
		}
		commands[command.Name] = struct{}{}
	}
	uiIDs := make(map[string]struct{}, len(manifest.UI))
	for _, item := range manifest.UI {
		if !toolNamePattern.MatchString(item.ID) {
			return fmt.Errorf("invalid extension UI contribution id %q", item.ID)
		}
		if item.Slot != "composer" && item.Slot != "review" {
			return fmt.Errorf("extension UI contribution %q slot must be composer or review", item.ID)
		}
		if strings.TrimSpace(item.Title) == "" || len(item.Title) > 128 || strings.TrimSpace(item.Description) == "" || len(item.Description) > 1024 {
			return fmt.Errorf("extension UI contribution %q requires a title and description within their limits", item.ID)
		}
		if item.Prompt != "" {
			if _, err := safePath("extension", item.Prompt); err != nil {
				return fmt.Errorf("extension UI contribution %q prompt: %w", item.ID, err)
			}
		}
		if _, exists := uiIDs[item.ID]; exists {
			return fmt.Errorf("extension UI contribution %q is declared more than once", item.ID)
		}
		uiIDs[item.ID] = struct{}{}
	}
	tools := make(map[string]struct{}, len(manifest.Tools))
	for _, tool := range manifest.Tools {
		if !toolNamePattern.MatchString(tool.Name) {
			return fmt.Errorf("invalid extension tool name %q", tool.Name)
		}
		if len(externalToolName(manifest.ID, tool.Name)) > maximumExtensionName {
			return fmt.Errorf("extension tool %q has a name that is too long", tool.Name)
		}
		if strings.TrimSpace(tool.Description) == "" || len(tool.Description) > 1024 {
			return fmt.Errorf("extension tool %q requires a description no longer than 1024 bytes", tool.Name)
		}
		if len(tool.Command) == 0 || len(tool.Command) > 32 {
			return fmt.Errorf("extension tool %q requires 1-32 command arguments", tool.Name)
		}
		if _, err := safePath("extension", tool.Command[0]); err != nil {
			return fmt.Errorf("extension tool %q command: %w", tool.Name, err)
		}
		for _, argument := range tool.Command {
			if len(argument) > 4096 {
				return fmt.Errorf("extension tool %q command argument is too long", tool.Name)
			}
		}
		var schema map[string]any
		if len(tool.Parameters) == 0 || json.Unmarshal(tool.Parameters, &schema) != nil || schema == nil {
			return fmt.Errorf("extension tool %q parameters must be a JSON object", tool.Name)
		}
		if tool.TimeoutSeconds < 0 || tool.TimeoutSeconds > int(maximumToolTimeout/time.Second) {
			return fmt.Errorf("extension tool %q timeout must be 1-%d seconds", tool.Name, int(maximumToolTimeout/time.Second))
		}
		name := externalToolName(manifest.ID, tool.Name)
		if _, found := tools[name]; found {
			return fmt.Errorf("extension tool %q is declared more than once", tool.Name)
		}
		tools[name] = struct{}{}
	}
	return nil
}

func copyTree(source, destination string) error {
	total := int64(0)
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("extension source contains symlink %q", relative)
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("extension source contains unsupported file %q", relative)
		}
		total += info.Size()
		if total > maxExtensionBytes {
			return fmt.Errorf("extension source exceeds the %d-byte limit", maxExtensionBytes)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func absoluteDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve directory: %w", err)
	}
	return abs, nil
}
