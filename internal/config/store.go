// Package config stores non-secret Gator settings.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const version = 1

// Settings is the single user-owned configuration document. Credentials do
// not belong here; they remain in Gator's private auth store.
type Settings struct {
	Version            int                 `json:"version"`
	Defaults           Defaults            `json:"defaults"`
	Extensions         []Extension         `json:"extensions,omitempty"`
	TrustedRepositories []string            `json:"trusted_repositories,omitempty"`
}

// Defaults applies when an interactive session or scripted run does not name
// a provider or model explicitly.
type Defaults struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// Extension records whether a globally installed extension is available to
// native runs. Extension files live in Gator's data directory, not config.json.
type Extension struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// Store owns config.json below one configuration root.
type Store struct {
	path string
}

// Default returns usable settings without requiring a file on disk.
func Default() Settings {
	return Settings{Version: version}
}

// ResolveDir resolves Gator's configuration root. An explicit override and
// GATOR_CONFIG_DIR are useful for tests and managed installations.
func ResolveDir(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return absoluteDirectory(override)
	}
	if configured := os.Getenv("GATOR_CONFIG_DIR"); strings.TrimSpace(configured) != "" {
		return absoluteDirectory(configured)
	}
	if configured := os.Getenv("XDG_CONFIG_HOME"); strings.TrimSpace(configured) != "" {
		return absoluteDirectory(configured)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(home, ".config"), nil
}

// New creates a settings store below configurationRoot.
func New(configurationRoot string) (Store, error) {
	if strings.TrimSpace(configurationRoot) == "" {
		return Store{}, errors.New("configuration directory is required")
	}
	root, err := absoluteDirectory(configurationRoot)
	if err != nil {
		return Store{}, err
	}
	return Store{path: filepath.Join(root, "gator", "config.json")}, nil
}

// DefaultStore opens the standard user settings location.
func DefaultStore() (Store, error) {
	directory, err := ResolveDir("")
	if err != nil {
		return Store{}, err
	}
	return New(directory)
}

// Path returns the settings path without reading its contents.
func (s Store) Path() string {
	return s.path
}

// Load reads settings. A missing file deliberately resolves to defaults so a
// new install is immediately usable with environment credentials.
func (s Store) Load() (Settings, error) {
	if strings.TrimSpace(s.path) == "" {
		return Settings{}, errors.New("configuration store is not initialized")
	}
	info, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("stat configuration file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Settings{}, errors.New("configuration file is not a regular file")
	}
	if info.Size() > 1024*1024 {
		return Settings{}, errors.New("configuration file exceeds 1 MiB")
	}
	contents, err := os.ReadFile(s.path)
	if err != nil {
		return Settings{}, fmt.Errorf("read configuration file: %w", err)
	}
	settings := Default()
	if err := json.Unmarshal(contents, &settings); err != nil {
		return Settings{}, fmt.Errorf("decode configuration file: %w", err)
	}
	if err := validate(settings); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

// Save atomically publishes the entire settings document. Callers normally
// load, change one field, then save so the document stays understandable when
// inspected or managed outside Gator.
func (s Store) Save(settings Settings) error {
	if strings.TrimSpace(s.path) == "" {
		return errors.New("configuration store is not initialized")
	}
	if settings.Version == 0 {
		settings.Version = version
	}
	if err := validate(settings); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create configuration directory: %w", err)
	}
	payload, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode configuration file: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".config-*")
	if err != nil {
		return fmt.Errorf("create configuration file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set configuration file permissions: %w", err)
	}
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write configuration file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close configuration file: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("publish configuration file: %w", err)
	}
	return nil
}

func validate(settings Settings) error {
	if settings.Version != version {
		return fmt.Errorf("unsupported configuration version %d", settings.Version)
	}
	if len(settings.Defaults.Provider) > 128 || len(settings.Defaults.Model) > 512 {
		return errors.New("configuration default exceeds its size limit")
	}
	if len(settings.Extensions) > 256 {
		return errors.New("configuration has too many extensions")
	}
	enabled := make(map[string]struct{}, len(settings.Extensions))
	for _, extension := range settings.Extensions {
		id := strings.TrimSpace(extension.ID)
		if id == "" || len(id) > 128 {
			return errors.New("extension ID is required and must be at most 128 bytes")
		}
		if _, exists := enabled[id]; exists {
			return fmt.Errorf("extension %q is configured more than once", id)
		}
		enabled[id] = struct{}{}
	}
	if len(settings.TrustedRepositories) > 256 {
		return errors.New("configuration has too many trusted repositories")
	}
	trusted := make(map[string]struct{}, len(settings.TrustedRepositories))
	for _, repository := range settings.TrustedRepositories {
		path := strings.TrimSpace(repository)
		if path == "" || len(path) > 4*1024 {
			return errors.New("trusted repository path is invalid")
		}
		if _, exists := trusted[path]; exists {
			return fmt.Errorf("repository %q is trusted more than once", path)
		}
		trusted[path] = struct{}{}
	}
	return nil
}

func absoluteDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve configuration directory: %w", err)
	}
	return abs, nil
}
