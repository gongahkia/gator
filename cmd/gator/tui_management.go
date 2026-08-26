package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/extension"
	"github.com/gongahkia/gator/internal/hooks"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/lsp"
	"github.com/gongahkia/gator/internal/mcp"
	"github.com/gongahkia/gator/internal/tui"
)

type tuiManagementBackend struct {
	repository string
	settings   config.Store
	extensions extension.Store
}

func newTUIManagementBackend(repository string, settings config.Store) (tui.ManagementBackend, error) {
	extensions, err := extension.DefaultStore()
	if err != nil {
		return nil, err
	}
	return &tuiManagementBackend{repository: repository, settings: settings, extensions: extensions}, nil
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
	children, err := childSnapshot(runRecord)
	if err != nil {
		return tui.ManagementSnapshot{}, err
	}
	return tui.ManagementSnapshot{Trusts: trusts, Worktrees: worktrees, Children: children, Extensions: extensions}, nil
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

func childSnapshot(runRecord string) ([]tui.ManagedChild, error) {
	if strings.TrimSpace(runRecord) == "" {
		return nil, nil
	}
	manifests, err := journal.ListChildManifests(runRecord)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
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
	return result, nil
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
