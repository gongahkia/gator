package cliagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gongahkia/gator/internal/sandbox"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// prepareCLI always establishes the outer OS sandbox before adding credentials.
// A persistent home is host-created and must be separate from the source copy.
func prepareCLI(ctx context.Context, cfg Config, directory, home string, argv []string, edit bool, scopes []string, key string) (sandbox.Prepared, error) {
	var err error
	policy := sandbox.Policy{Mode: sandbox.Strict, Network: sandbox.AllowNetwork, ReadOnlyRoots: append([]string(nil), cfg.ReadOnlyRoots...)}
	policy.ReadOnlyRoots = append(policy.ReadOnlyRoots, cfg.Executable)
	if home != "" {
		policy.WritableRoots = []string{home}
	}
	// Rust TLS cannot consult macOS Keychain services inside Seatbelt. Use the
	// system's public CA bundle, retaining normal certificate verification.
	certificate := ""
	if runtime.GOOS == "darwin" {
		certificate, err = filepath.EvalSymlinks("/etc/ssl/cert.pem")
		if err != nil {
			return sandbox.Prepared{}, fmt.Errorf("system CA bundle: %w", err)
		}
		policy.ReadOnlyRoots = append(policy.ReadOnlyRoots, certificate)
	}
	policy.WritablePaths = append([]string(nil), scopes...)
	if !edit {
		// No worktree writes in inspection mode; private HOME/cache remains writable.
		policy.WritablePaths = []string{".gator-inspect-no-writes"}
	}
	prepared, err := sandbox.Prepare(ctx, sandbox.Request{Dir: directory, Argv: append([]string{cfg.Executable}, argv...), Policy: policy})
	if err != nil {
		return sandbox.Prepared{}, err
	}

	ready := false
	defer func() {
		if !ready {
			prepared.Cleanup()
		}
	}()
	cmd := prepared.Command
	if home != "" {
		cmd.Env = append(cmd.Env, "HOME="+home)
	}
	if cfg.Kind == "claude" && cfg.BaseURL != "" {
		cmd.Env = append(cmd.Env, "ANTHROPIC_BASE_URL="+cfg.BaseURL)
	}
	if cfg.Kind == "pi" {
		privateHome := ""
		for _, env := range cmd.Env {
			if strings.HasPrefix(env, "HOME=") {
				privateHome = strings.TrimPrefix(env, "HOME=")
			}
		}
		if privateHome == "" {
			return sandbox.Prepared{}, errors.New("sandbox has no private HOME")
		}
		agentDir := filepath.Join(privateHome, ".pi", "agent")
		if err := os.MkdirAll(agentDir, 0700); err != nil {
			return sandbox.Prepared{}, err
		}
		provider := cfg.Provider
		if provider == "" {
			provider = "openai"
		}
		selected := map[string]string{"apiKey": "$" + cfg.APIKeyEnv}
		if cfg.BaseURL != "" {
			selected["baseUrl"] = cfg.BaseURL
		}
		data, err := json.Marshal(map[string]any{"providers": map[string]any{provider: selected}})
		if err != nil {
			return sandbox.Prepared{}, err
		}
		if err := os.WriteFile(filepath.Join(agentDir, "models.json"), data, 0600); err != nil {
			return sandbox.Prepared{}, err
		}
		cmd.Env = append(cmd.Env, "PI_CODING_AGENT_DIR="+agentDir)
	}
	// Inject exactly one explicitly selected credential into the child, never argv
	// or config. All user config/home discovery is disabled or isolated.
	if certificate != "" {
		cmd.Env = append(cmd.Env, "SSL_CERT_FILE="+certificate, "NODE_EXTRA_CA_CERTS="+certificate)
	}
	cmd.Env = append(cmd.Env, cfg.CredentialTarget()+"="+key, "PI_TELEMETRY=0")
	ready = true
	return prepared, nil
}
