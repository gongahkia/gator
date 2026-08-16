// Package auth persists Gator-owned provider credentials. It deliberately
// stores no conversation content, journal data, or credentials owned by other
// applications.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	apiKeyType = "api_key"
	oauthType  = "oauth"

	lockTimeout = 10 * time.Second
	lockRetry   = 25 * time.Millisecond
)

// Credential is one provider-scoped credential. API key credentials use Key.
// OAuth credentials use Access, Refresh, and Expires. Extra holds public
// protocol metadata such as an account identifier or a Copilot endpoint; it
// must never contain another credential.
type Credential struct {
	Type    string            `json:"type"`
	Key     string            `json:"key,omitempty"`
	Access  string            `json:"access,omitempty"`
	Refresh string            `json:"refresh,omitempty"`
	Expires int64             `json:"expires,omitempty"`
	Extra   map[string]string `json:"extra,omitempty"`
}

// IsOAuth reports whether this credential contains OAuth token material.
func (c Credential) IsOAuth() bool {
	return c.Type == oauthType
}

// IsAPIKey reports whether this credential contains an API key.
func (c Credential) IsAPIKey() bool {
	return c.Type == apiKeyType
}

// Expired reports whether an OAuth credential needs refreshing. Non-OAuth
// credentials and permanent OAuth-derived keys do not expire.
func (c Credential) Expired(now time.Time) bool {
	return c.IsOAuth() && c.Expires > 0 && now.UnixMilli() >= c.Expires
}

// Store owns Gator's auth.json. StateDir is the same base selected for the
// journal, so the default path is ~/.local/state/gator/auth.json on Linux.
// The file is private (0600) and its containing directory is private (0700).
type Store struct {
	path string
}

// New constructs a credential store under stateDir. It performs no I/O until a
// read or mutation is requested.
func New(stateDir string) (Store, error) {
	if strings.TrimSpace(stateDir) == "" {
		return Store{}, errors.New("credential state directory is required")
	}
	absolute, err := filepath.Abs(stateDir)
	if err != nil {
		return Store{}, fmt.Errorf("resolve credential state directory: %w", err)
	}
	return Store{path: filepath.Join(absolute, "gator", "auth.json")}, nil
}

// Path returns the credential file location without exposing its contents.
func (s Store) Path() string {
	return s.path
}

// Read returns a copy of one stored credential. A missing provider is not an
// error. Callers must not log the returned value.
func (s Store) Read(provider string) (Credential, bool, error) {
	provider, err := validProvider(provider)
	if err != nil {
		return Credential{}, false, err
	}
	credentials, err := s.readAll()
	if err != nil {
		return Credential{}, false, err
	}
	credential, ok := credentials[provider]
	return cloneCredential(credential), ok, nil
}

// Providers returns credential metadata only, suitable for status UIs.
func (s Store) Providers() ([]string, error) {
	credentials, err := s.readAll()
	if err != nil {
		return nil, err
	}
	providers := make([]string, 0, len(credentials))
	for provider := range credentials {
		providers = append(providers, provider)
	}
	return providers, nil
}

// Put atomically stores a credential for one provider. It is serialized with
// concurrent Gator credential updates and preserves all other provider entries.
func (s Store) Put(provider string, credential Credential) error {
	provider, err := validProvider(provider)
	if err != nil {
		return err
	}
	if err := validateCredential(credential); err != nil {
		return err
	}
	return s.withLock(func(credentials map[string]Credential) (bool, error) {
		credentials[provider] = cloneCredential(credential)
		return true, nil
	})
}

// Delete removes one stored credential. It succeeds when no credential exists.
func (s Store) Delete(provider string) error {
	provider, err := validProvider(provider)
	if err != nil {
		return err
	}
	return s.withLock(func(credentials map[string]Credential) (bool, error) {
		if _, ok := credentials[provider]; !ok {
			return false, nil
		}
		delete(credentials, provider)
		return true, nil
	})
}

func (s Store) withLock(update func(map[string]Credential) (bool, error)) error {
	if strings.TrimSpace(s.path) == "" {
		return errors.New("credential store is not initialized")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create credential directory: %w", err)
	}
	release, err := acquireLock(s.path + ".lock")
	if err != nil {
		return err
	}
	defer release()
	credentials, err := s.readAll()
	if err != nil {
		return err
	}
	changed, err := update(credentials)
	if err != nil || !changed {
		return err
	}
	return s.writeAll(credentials)
}

func (s Store) readAll() (map[string]Credential, error) {
	if strings.TrimSpace(s.path) == "" {
		return nil, errors.New("credential store is not initialized")
	}
	info, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Credential{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat credential file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("credential file is not a regular file")
	}
	if info.Size() > 1024*1024 {
		return nil, errors.New("credential file exceeds 1 MiB")
	}
	contents, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("read credential file: %w", err)
	}
	credentials := map[string]Credential{}
	if err := json.Unmarshal(contents, &credentials); err != nil {
		return nil, fmt.Errorf("decode credential file: %w", err)
	}
	for provider, credential := range credentials {
		if _, err := validProvider(provider); err != nil {
			return nil, fmt.Errorf("invalid credential provider %q", provider)
		}
		if err := validateCredential(credential); err != nil {
			return nil, fmt.Errorf("invalid credential for %q: %w", provider, err)
		}
	}
	return credentials, nil
}

func (s Store) writeAll(credentials map[string]Credential) error {
	payload, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credential file: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".auth-*")
	if err != nil {
		return fmt.Errorf("create credential file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("set credential file permissions: %w", err)
	}
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		temporary.Close()
		return fmt.Errorf("write credential file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync credential file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close credential file: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("publish credential file: %w", err)
	}
	return nil
}

func acquireLock(path string) (func(), error) {
	deadline := time.Now().Add(lockTimeout)
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			return func() {
				file.Close()
				_ = os.Remove(path)
			}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("create credential lock: %w", err)
		}
		if time.Now().After(deadline) {
			return nil, errors.New("credential store is busy; retry shortly")
		}
		time.Sleep(lockRetry)
	}
}

func validProvider(value string) (string, error) {
	provider := strings.TrimSpace(strings.ToLower(value))
	if provider == "" || filepath.Base(provider) != provider || strings.ContainsAny(provider, "\\/") {
		return "", fmt.Errorf("invalid credential provider %q", value)
	}
	return provider, nil
}

func validateCredential(credential Credential) error {
	switch credential.Type {
	case apiKeyType:
		if strings.TrimSpace(credential.Key) == "" || credential.Access != "" || credential.Refresh != "" || credential.Expires != 0 {
			return errors.New("API key credential is incomplete or contains OAuth fields")
		}
	case oauthType:
		if strings.TrimSpace(credential.Access) == "" || credential.Expires < 0 || credential.Key != "" {
			return errors.New("OAuth credential is incomplete or contains an API key")
		}
	default:
		return fmt.Errorf("unsupported credential type %q", credential.Type)
	}
	for key, value := range credential.Extra {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return errors.New("credential metadata contains an empty key or value")
		}
	}
	return nil
}

func cloneCredential(credential Credential) Credential {
	clone := credential
	if credential.Extra != nil {
		clone.Extra = make(map[string]string, len(credential.Extra))
		for key, value := range credential.Extra {
			clone.Extra[key] = value
		}
	}
	return clone
}
