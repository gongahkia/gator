package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/extension"
	"github.com/gongahkia/gator/internal/hooks"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/lsp"
	"github.com/gongahkia/gator/internal/mcp"
	modelprovider "github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/patch"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/tui"
)

type tuiManagementBackend struct {
	repository  string
	stateDir    string
	settings    config.Store
	extensions  extension.Store
	lspRegistry *lsp.Registry
}

func newTUIManagementBackend(repository, stateDir string, settings config.Store, lspRegistry *lsp.Registry) (tui.ManagementBackend, error) {
	extensions, err := extension.DefaultStore()
	if err != nil {
		return nil, err
	}
	return &tuiManagementBackend{repository: repository, stateDir: stateDir, settings: settings, extensions: extensions, lspRegistry: lspRegistry}, nil
}

func (backend *tuiManagementBackend) Snapshot(runRecord string) (tui.ManagementSnapshot, error) {
	settings, err := backend.settings.Load()
	if err != nil {
		return tui.ManagementSnapshot{}, err
	}
	trusts, err := backend.trustSnapshot(settings)
	if err != nil {
		return tui.ManagementSnapshot{}, err
	}
	worktrees, err := backend.worktreeSnapshot()
	if err != nil {
		return tui.ManagementSnapshot{}, err
	}
	extensions, err := backend.extensionSnapshot(settings)
	if err != nil {
		return tui.ManagementSnapshot{}, err
	}
	mcpAuth, err := backend.mcpAuthSnapshot(settings)
	if err != nil {
		return tui.ManagementSnapshot{}, err
	}
	children, batches, err := childSnapshot(runRecord)
	if err != nil {
		return tui.ManagementSnapshot{}, err
	}
	runs, err := journal.ListRecentRuns(backend.stateDir, backend.repository, 50)
	if err != nil {
		return tui.ManagementSnapshot{}, err
	}
	managedRuns := make([]tui.ManagedRun, 0, len(runs))
	for _, run := range runs {
		managedRuns = append(managedRuns, tui.ManagedRun{
			ID: run.RunID, StatePath: run.StatePath, WorktreePath: run.WorktreePath,
			Provider: run.Provider, Model: run.Model, Task: run.Task,
			UpdatedAt: run.UpdatedAt, Available: run.Available,
		})
	}
	policy := settings.Execution.Normalize()
	return tui.ManagementSnapshot{
		Settings: tui.ManagedSettings{
			SandboxMode: string(policy.Mode), Network: string(policy.Network),
			DefaultProvider: settings.Defaults.Provider, DefaultModel: settings.Defaults.Model,
		},
		Trusts: trusts, MCPAuth: mcpAuth, Runs: managedRuns, Worktrees: worktrees, Children: children, Batches: batches, Extensions: extensions,
		LSPRuntime: backend.lspRuntimeSnapshot(),
	}, nil
}

func (backend *tuiManagementBackend) lspRuntimeSnapshot() []tui.ManagedLSPRuntime {
	snapshot := backend.lspRegistry.Snapshot()
	result := make([]tui.ManagedLSPRuntime, 0, len(snapshot))
	for _, entry := range snapshot {
		servers := make([]tui.ManagedLSPServer, 0, len(entry.Servers))
		for _, server := range entry.Servers {
			servers = append(servers, tui.ManagedLSPServer{Name: server.Name, Language: server.Language, Started: server.Started})
		}
		result = append(result, tui.ManagedLSPRuntime{
			Worktree: entry.Worktree, Hash: entry.Hash, Active: entry.Active, Retired: entry.Retired, Servers: servers,
		})
	}
	return result
}

// mcpAuthSnapshot reports per-server authorization state without contacting
// any remote service. It reads the manifest only to enumerate configured
// Streamable HTTP server names; tokens never leave the private store.
func (backend *tuiManagementBackend) mcpAuthSnapshot(settings config.Settings) ([]tui.ManagedMCPAuth, error) {
	repository, digest, trusted, err := backend.mcpTrustState(settings)
	if err != nil || digest == "" {
		return nil, err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return nil, err
	}
	statuses, err := mcp.HTTPAuthorizationStatuses(repository, credentials, time.Now())
	if err != nil {
		return nil, err
	}
	result := make([]tui.ManagedMCPAuth, 0, len(statuses))
	for _, status := range statuses {
		result = append(result, tui.ManagedMCPAuth{
			Server:        status.Server,
			Authenticated: status.Authenticated,
			Expired:       status.Expired,
			BundleTrusted: trusted,
		})
	}
	return result, nil
}

func (backend *tuiManagementBackend) mcpTrustState(settings config.Settings) (string, string, bool, error) {
	repository, err := mcp.CanonicalRepository(backend.repository)
	if err != nil {
		return "", "", false, err
	}
	digest, err := mcp.BundleHash(repository)
	if err != nil {
		return "", "", false, err
	}
	return repository, digest, digest != "" && mcpTrustFor(settings.MCPTrusts, repository) == digest, nil
}

// BeginMCPOAuthLogin re-checks the trusted manifest hash in the command layer
// before the MCP package discovers an authorization server, so a stale TUI
// snapshot can never produce a browser URL for untrusted project config.
func (backend *tuiManagementBackend) BeginMCPOAuthLogin(server string) (tui.OAuthLogin, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return nil, errors.New("choose a configured Streamable HTTP MCP server")
	}
	settings, err := backend.settings.Load()
	if err != nil {
		return nil, err
	}
	repository, digest, trusted, err := backend.mcpTrustState(settings)
	if err != nil {
		return nil, err
	}
	if digest == "" {
		return nil, errors.New("no .gator/mcp.json exists to authenticate")
	}
	if !trusted {
		return nil, errors.New("project MCP configuration is not trusted; review and trust it before authenticating a remote server")
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return nil, err
	}
	login, err := mcp.BeginOAuthLogin(context.Background(), repository, server, digest, credentials, mcp.OAuthLoginOptions{
		ClientID:    strings.TrimSpace(os.Getenv("GATOR_MCP_CLIENT_ID")),
		RedirectURL: strings.TrimSpace(os.Getenv("GATOR_MCP_REDIRECT_URL")),
	})
	if err != nil {
		return nil, err
	}
	return login, nil
}

// RemoveMCPCredential deletes exactly one Gator-owned MCP token. It resolves
// the credential key from the current manifest so an unrelated stored entry
// cannot be deleted by name.
func (backend *tuiManagementBackend) RemoveMCPCredential(server string) error {
	server = strings.TrimSpace(server)
	if server == "" {
		return errors.New("choose a configured Streamable HTTP MCP server")
	}
	repository, err := mcp.CanonicalRepository(backend.repository)
	if err != nil {
		return err
	}
	key, err := mcp.OAuthCredentialKeyForServer(repository, server)
	if err != nil {
		return err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return err
	}
	if err := credentials.Delete(key); err != nil {
		return fmt.Errorf("remove MCP OAuth credential: %w", err)
	}
	return nil
}

func (backend *tuiManagementBackend) trustSnapshot(settings config.Settings) ([]tui.ManagedTrust, error) {
	type source struct {
		kind      string
		canonical func(string) (string, error)
		hash      func(string) (string, error)
		trusted   func(string) string
	}
	sources := []source{
		{kind: "hooks", canonical: hooks.CanonicalRepository, hash: hooks.BundleHash, trusted: func(repository string) string { return hookTrustFor(settings.HookTrusts, repository) }},
		{kind: "lsp", canonical: lsp.CanonicalRepository, hash: lsp.BundleHash, trusted: func(repository string) string { return lspTrustFor(settings.LSPTrusts, repository) }},
		{kind: "mcp", canonical: mcp.CanonicalRepository, hash: mcp.BundleHash, trusted: func(repository string) string { return mcpTrustFor(settings.MCPTrusts, repository) }},
		{kind: "extensions", canonical: extension.CanonicalRepository, hash: extension.BundleHash, trusted: func(repository string) string { return extensionTrustFor(settings.ExtensionTrusts, repository) }},
	}
	result := make([]tui.ManagedTrust, 0, len(sources))
	for _, candidate := range sources {
		repository, err := candidate.canonical(backend.repository)
		if err != nil {
			return nil, err
		}
		digest, err := candidate.hash(repository)
		if err != nil {
			return nil, err
		}
		result = append(result, tui.ManagedTrust{
			Kind:       candidate.kind,
			Hash:       digest,
			Configured: digest != "",
			Trusted:    digest != "" && candidate.trusted(repository) == digest,
		})
	}
	return result, nil
}

func (backend *tuiManagementBackend) worktreeSnapshot() ([]tui.ManagedWorktree, error) {
	repository, err := filepath.EvalSymlinks(backend.repository)
	if err != nil {
		return nil, err
	}
	base := filepath.Join(filepath.Dir(repository), filepath.Base(repository)+"-gator-runs")
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]tui.ManagedWorktree, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && entry.Type()&os.ModeSymlink == 0 && worktreeID.MatchString(entry.Name()) {
			result = append(result, tui.ManagedWorktree{ID: entry.Name(), Path: filepath.Join(base, entry.Name())})
		}
	}
	sort.Slice(result, func(first, second int) bool { return result[first].ID < result[second].ID })
	return result, nil
}

func (backend *tuiManagementBackend) extensionSnapshot(settings config.Settings) ([]tui.ManagedExtension, error) {
	installed, err := backend.extensions.List()
	if err != nil {
		return nil, err
	}
	enabled := extensionEnabled(settings.Extensions)
	result := make([]tui.ManagedExtension, 0, len(installed))
	for _, candidate := range installed {
		result = append(result, tui.ManagedExtension{
			ID:          candidate.Manifest.ID,
			Name:        candidate.Manifest.Name,
			Description: candidate.Manifest.Description,
			Enabled:     enabled[candidate.Manifest.ID],
			Tools:       len(candidate.Manifest.Tools),
		})
	}
	return result, nil
}

func childSnapshot(runRecord string) ([]tui.ManagedChild, []tui.ManagedBatch, error) {
	if strings.TrimSpace(runRecord) == "" {
		return nil, nil, nil
	}
	manifests, err := journal.ListChildManifests(runRecord)
	if errors.Is(err, os.ErrNotExist) {
		manifests = nil
		err = nil
	}
	if err != nil {
		return nil, nil, err
	}
	result := make([]tui.ManagedChild, 0, len(manifests))
	for _, manifest := range manifests {
		result = append(result, tui.ManagedChild{
			ID:           manifest.ID,
			Status:       string(manifest.Status),
			Role:         manifest.Role,
			BatchID:      manifest.BatchID,
			WorktreePath: manifest.WorktreePath,
			PatchBytes:   manifest.PatchBytes,
			Error:        manifest.Error,
		})
	}
	batchManifests, err := journal.ListChildBatchManifests(runRecord)
	if errors.Is(err, os.ErrNotExist) {
		batchManifests = nil
	} else if err != nil {
		return nil, nil, err
	}
	batches := make([]tui.ManagedBatch, 0, len(batchManifests))
	for _, batch := range batchManifests {
		conflicts := make([]tui.ManagedConflict, 0, len(batch.Conflicts))
		for _, conflict := range batch.Conflicts {
			conflicts = append(conflicts, tui.ManagedConflict{
				Kind: conflict.Kind, ChildIDs: append([]string(nil), conflict.ChildIDs...),
				Paths: append([]string(nil), conflict.Paths...), Detail: conflict.Detail,
			})
		}
		batches = append(batches, tui.ManagedBatch{
			ID: batch.ID, Status: string(batch.Status), ChildIDs: append([]string(nil), batch.ChildIDs...),
			Conflicts: conflicts, Error: batch.Error,
		})
	}
	return result, batches, nil
}

func (backend *tuiManagementBackend) SetExecutionPolicy(mode, network string) error {
	settings, err := backend.settings.Load()
	if err != nil {
		return err
	}
	policy := settings.Execution.Normalize()
	policy.Mode = sandbox.Mode(strings.TrimSpace(mode))
	policy.Network = sandbox.Network(strings.TrimSpace(network))
	if err := policy.Validate(); err != nil {
		return err
	}
	settings.Execution = policy
	return backend.settings.Save(settings)
}

func (backend *tuiManagementBackend) SetDefaults(provider, model string) error {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if provider == "" || len(provider) > 128 || strings.ContainsAny(provider, "\r\n") {
		return errors.New("default provider is invalid")
	}
	if len(model) > 512 || strings.ContainsAny(model, "\r\n") {
		return errors.New("default model is invalid")
	}
	settings, err := backend.settings.Load()
	if err != nil {
		return err
	}
	if custom, found := configuredCustomProvider(settings, provider); found {
		if model == "" {
			model = custom.DefaultModel
		}
		if !customProviderSupportsModel(custom, model) {
			return fmt.Errorf("model %q is not configured for custom provider %q", model, custom.ID)
		}
		provider = custom.ID
	} else {
		parsed, err := modelprovider.ParseProvider(provider)
		if err != nil {
			return err
		}
		provider = string(parsed)
		model = modelprovider.EffectiveModel(parsed, model)
	}
	settings.Defaults.Provider = provider
	settings.Defaults.Model = model
	return backend.settings.Save(settings)
}

func (backend *tuiManagementBackend) SetTrust(kind string, trusted bool) error {
	settings, err := backend.settings.Load()
	if err != nil {
		return err
	}
	switch kind {
	case "hooks":
		repository, err := hooks.CanonicalRepository(backend.repository)
		if err != nil {
			return err
		}
		if trusted {
			digest, err := hooks.BundleHash(repository)
			if err != nil {
				return err
			}
			if digest == "" {
				return errors.New("no .gator/hooks.json exists to trust")
			}
			settings.HookTrusts = setHookTrust(settings.HookTrusts, hooks.Trust{Repository: repository, Hash: digest})
		} else {
			settings.HookTrusts = removeHookTrust(settings.HookTrusts, repository)
		}
	case "lsp":
		repository, err := lsp.CanonicalRepository(backend.repository)
		if err != nil {
			return err
		}
		if trusted {
			digest, err := lsp.BundleHash(repository)
			if err != nil {
				return err
			}
			if digest == "" {
				return errors.New("no .gator/lsp.json exists to trust")
			}
			settings.LSPTrusts = setLSPTrust(settings.LSPTrusts, lsp.Trust{Repository: repository, Hash: digest})
		} else {
			settings.LSPTrusts = removeLSPTrust(settings.LSPTrusts, repository)
		}
	case "mcp":
		repository, err := mcp.CanonicalRepository(backend.repository)
		if err != nil {
			return err
		}
		if trusted {
			digest, err := mcp.BundleHash(repository)
			if err != nil {
				return err
			}
			if digest == "" {
				return errors.New("no .gator/mcp.json exists to trust")
			}
			settings.MCPTrusts = setMCPTrust(settings.MCPTrusts, mcp.Trust{Repository: repository, Hash: digest})
		} else {
			settings.MCPTrusts = removeMCPTrust(settings.MCPTrusts, repository)
		}
	case "extensions":
		repository, err := extension.CanonicalRepository(backend.repository)
		if err != nil {
			return err
		}
		if trusted {
			digest, err := extension.BundleHash(repository)
			if err != nil {
				return err
			}
			if digest == "" {
				return errors.New("no .gator/extensions bundle exists to trust")
			}
			settings.ExtensionTrusts = setExtensionTrust(settings.ExtensionTrusts, config.ExtensionTrust{Repository: repository, Hash: digest})
		} else {
			settings.ExtensionTrusts = removeExtensionTrust(settings.ExtensionTrusts, repository)
		}
	default:
		return errors.New("unknown project trust kind")
	}
	return backend.settings.Save(settings)
}

func (backend *tuiManagementBackend) SetExtensionEnabled(id string, enabled bool) error {
	installed, err := backend.extensions.List()
	if err != nil {
		return err
	}
	found := false
	for _, candidate := range installed {
		if candidate.Manifest.ID == id {
			found = true
			break
		}
	}
	if !found {
		return errors.New("extension is not installed")
	}
	settings, err := backend.settings.Load()
	if err != nil {
		return err
	}
	settings.Extensions = setExtensionSetting(settings.Extensions, id, enabled)
	return backend.settings.Save(settings)
}

// PrepareExtension stages one source and returns display-safe metadata. It
// deliberately does not write settings: a staged bundle is unreferenced code
// until CommitExtensionInstall publishes the reviewed hash.
func (backend *tuiManagementBackend) PrepareExtension(source string) (tui.ExtensionInstallPreview, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return tui.ExtensionInstallPreview{}, errors.New("enter a local bundle directory or an https Git repository URL")
	}
	if err := tuiInstallableExtensionSource(source); err != nil {
		return tui.ExtensionInstallPreview{}, err
	}
	context, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	prepared, err := backend.extensions.PrepareSource(context, source)
	if err != nil {
		return tui.ExtensionInstallPreview{}, err
	}
	return tui.ExtensionInstallPreview{
		Token: prepared.Token, Source: source, ID: prepared.ID, Name: prepared.Name,
		Description: prepared.Description, Hash: prepared.Hash, Skills: prepared.Skills,
		Prompts: prepared.Prompts, Commands: prepared.Commands, UI: prepared.UI,
		Tools: prepared.Tools, AlreadyInstalled: prepared.AlreadyInstalled,
	}, nil
}

// CommitExtensionInstall publishes the reviewed staged bytes and only then
// enables the extension, so a cancelled review never leaves an enabled entry.
func (backend *tuiManagementBackend) CommitExtensionInstall(token, hash string, replace bool) error {
	installed, err := backend.extensions.CommitPrepared(token, hash, replace)
	if err != nil {
		return err
	}
	settings, err := backend.settings.Load()
	if err != nil {
		return err
	}
	settings.Extensions = setExtensionSetting(settings.Extensions, installed.Manifest.ID, true)
	return backend.settings.Save(settings)
}

func (backend *tuiManagementBackend) DiscardExtensionPrepare(token string) error {
	return backend.extensions.DiscardPrepared(token)
}

// tuiInstallableExtensionSource repeats the TUI's narrower source rule in the
// command layer so a stale or manipulated UI cannot reach ssh, git@, or plain
// HTTP fetching through the one-key install path.
func tuiInstallableExtensionSource(source string) error {
	if strings.ContainsAny(source, "\r\n\x00") || strings.HasPrefix(source, "-") {
		return errors.New("extension source contains unsupported characters")
	}
	if strings.HasPrefix(source, "git@") || strings.HasPrefix(source, "ssh://") {
		return errors.New("install ssh and git@ sources with 'gator extension install'; the TUI accepts only local directories and https URLs")
	}
	parsed, err := url.Parse(source)
	if err == nil && parsed.Scheme != "" {
		if parsed.Scheme != "https" {
			return errors.New("remote extension sources must use https")
		}
		if parsed.Host == "" || parsed.User != nil {
			return errors.New("https extension sources require a host and must not embed credentials")
		}
		return nil
	}
	info, statErr := os.Lstat(source)
	if statErr != nil {
		return errors.New("extension source must be an existing local directory or an https Git repository URL")
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("local extension sources must be a real bundle directory")
	}
	return nil
}

func (backend *tuiManagementBackend) RemoveExtension(id string) error {
	if err := backend.extensions.Remove(id); err != nil {
		return err
	}
	settings, err := backend.settings.Load()
	if err != nil {
		return err
	}
	settings.Extensions = removeExtensionSetting(settings.Extensions, id)
	return backend.settings.Save(settings)
}

func (backend *tuiManagementBackend) PruneWorktrees() error {
	command := exec.CommandContext(context.Background(), "git", "worktree", "prune", "--verbose")
	command.Dir = backend.repository
	output, err := command.CombinedOutput()
	if err != nil {
		return gitCommandError("prune stale Git worktree metadata", err, output)
	}
	return nil
}

func (backend *tuiManagementBackend) RemoveWorktree(id string) error {
	if !worktreeID.MatchString(id) {
		return errors.New("invalid retained worktree ID")
	}
	repository, err := filepath.EvalSymlinks(backend.repository)
	if err != nil {
		return err
	}
	target := filepath.Join(filepath.Dir(repository), filepath.Base(repository)+"-gator-runs", id)
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("retained Gator worktree does not exist")
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("retained Gator worktree target must be a real directory")
	}
	command := exec.CommandContext(context.Background(), "git", "worktree", "remove", "--force", "--", target)
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err != nil {
		return gitCommandError("remove retained Gator worktree", err, output)
	}
	return nil
}

func (backend *tuiManagementBackend) ExportArtifact(kind, runRecord string) (string, error) {
	session, runID, err := backend.managedSession(runRecord)
	if err != nil {
		return "", err
	}
	exports := filepath.Join(backend.stateDir, "gator", "exports")
	if err := os.MkdirAll(exports, 0o700); err != nil {
		return "", fmt.Errorf("create private export directory: %w", err)
	}
	suffix := ".patch"
	if kind == "transcript" {
		suffix = ".html"
	} else if kind != "patch" {
		return "", errors.New("unknown artifact export kind")
	}
	file, err := os.CreateTemp(exports, runID+"-*"+suffix)
	if err != nil {
		return "", fmt.Errorf("create private export: %w", err)
	}
	path := file.Name()
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return "", err
	}
	if kind == "patch" {
		exported, err := patch.Export(context.Background(), session.WorktreePath, session.BaseCommit)
		if err != nil {
			return "", err
		}
		if len(exported) == 0 {
			return "", errors.New("retained worktree has no patch to export")
		}
		if _, err := file.Write(exported); err != nil {
			return "", fmt.Errorf("write private patch export: %w", err)
		}
	} else if err := exportTranscript([]string{runRecord}, file); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync private export: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close private export: %w", err)
	}
	keep = true
	return path, nil
}

func (backend *tuiManagementBackend) CheckPatch(runRecord string) (int, error) {
	session, _, err := backend.managedSession(runRecord)
	if err != nil {
		return 0, err
	}
	result, err := patch.Check(context.Background(), session.WorktreePath, session.BaseCommit, backend.repository)
	if err != nil {
		return 0, err
	}
	return result.Bytes, nil
}

func (backend *tuiManagementBackend) ApplyPatch(runRecord string) (int, error) {
	session, _, err := backend.managedSession(runRecord)
	if err != nil {
		return 0, err
	}
	result, err := patch.Apply(context.Background(), session.WorktreePath, session.BaseCommit, backend.repository)
	if err != nil {
		return 0, err
	}
	return result.Bytes, nil
}

func (backend *tuiManagementBackend) managedSession(runRecord string) (journal.Session, string, error) {
	runRecord, err := journal.ManagedRunRecord(backend.stateDir, backend.repository, runRecord)
	if err != nil {
		return journal.Session{}, "", err
	}
	session, err := journal.LoadSession(runRecord)
	if err != nil {
		return journal.Session{}, "", err
	}
	expected, err := filepath.EvalSymlinks(backend.repository)
	if err != nil {
		return journal.Session{}, "", err
	}
	actual, err := filepath.EvalSymlinks(session.Repository)
	if err != nil {
		return journal.Session{}, "", err
	}
	if filepath.Clean(expected) != filepath.Clean(actual) {
		return journal.Session{}, "", errors.New("retained run belongs to another repository")
	}
	runID := filepath.Base(filepath.Clean(runRecord))
	if !worktreeID.MatchString(runID) {
		return journal.Session{}, "", errors.New("retained run has an invalid ID")
	}
	return session, runID, nil
}
