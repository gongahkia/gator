package extension

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
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
	return Installed{Manifest: manifest, Root: abs, Project: project}, nil
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
	if len(manifest.Description) > 1024 || len(manifest.Skills)+len(manifest.Prompts) > 64 || len(manifest.Tools) > 32 {
		return errors.New("extension manifest exceeds a resource limit")
	}
	resources := make(map[string]struct{}, len(manifest.Skills)+len(manifest.Prompts))
	for _, resource := range append(append([]string(nil), manifest.Skills...), manifest.Prompts...) {
		if _, err := safePath("extension", resource); err != nil {
			return fmt.Errorf("invalid extension resource %q: %w", resource, err)
		}
		if _, found := resources[resource]; found {
			return fmt.Errorf("extension resource %q is declared more than once", resource)
		}
		resources[resource] = struct{}{}
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
