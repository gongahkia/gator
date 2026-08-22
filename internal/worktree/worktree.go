// Package worktree creates isolated Git worktrees for coding-agent runs.
package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gongahkia/gator/internal/workspace"
)

var (
	runIDPattern  = regexp.MustCompile(`\A[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}\z`)
	commitPattern = regexp.MustCompile(`\A[0-9a-fA-F]{7,64}\z`)
)

// Worktree is one isolated checkout retained for agent review and recovery.
type Worktree struct {
	ID         string
	Repository string
	Path       string
	BaseCommit string
	Root       workspace.Root
}

// Options describes the explicit setup that may be applied to a fresh
// worktree. CopyIgnoredFiles is deliberately opt-in because such files often
// contain credentials and become model-accessible after copying.
type Options struct {
	BaseRef          string
	CopyIgnoredFiles bool
}

type ignoredFile struct {
	path string
	data []byte
}

// Create adds a detached worktree at a sibling location of the source
// repository. Keeping it outside the active checkout prevents the run's files
// and metadata from being confused with the developer's own changes.
func Create(ctx context.Context, repositoryPath, runID string) (Worktree, error) {
	return CreateWithOptions(ctx, repositoryPath, runID, Options{})
}

// CreateWithOptions creates a detached worktree from HEAD or an explicitly
// selected Git revision. It can copy only the exact ignored files listed in a
// tracked .gator/worktreeinclude file when the caller explicitly opts in.
func CreateWithOptions(ctx context.Context, repositoryPath, runID string, options Options) (Worktree, error) {
	if !runIDPattern.MatchString(runID) {
		return Worktree{}, fmt.Errorf("invalid run id %q", runID)
	}
	repository, err := repositoryRoot(ctx, repositoryPath)
	if err != nil {
		return Worktree{}, err
	}
	revisionSpec := "HEAD"
	if strings.TrimSpace(options.BaseRef) != "" {
		revisionSpec, err = resolveRevision(ctx, repository, options.BaseRef)
		if err != nil {
			return Worktree{}, err
		}
	}
	var setup []ignoredFile
	if options.CopyIgnoredFiles {
		setup, err = ignoredFileSetup(ctx, repository)
		if err != nil {
			return Worktree{}, err
		}
	}
	worktree, err := create(ctx, repository, runID, revisionSpec)
	if err != nil {
		return Worktree{}, err
	}
	if err := copyIgnoredFiles(worktree.Root, setup); err != nil {
		return worktree, fmt.Errorf("copy opted-in ignored files into retained worktree: %w", err)
	}
	return worktree, nil
}

// CreateAtRevision creates an isolated worktree at a verified immutable Git
// commit. It is used for session forks, which must reproduce an earlier turn
// rather than whatever HEAD happens to be when the fork begins.
func CreateAtRevision(ctx context.Context, repositoryPath, runID, revisionSpec string) (Worktree, error) {
	if !commitPattern.MatchString(revisionSpec) {
		return Worktree{}, fmt.Errorf("invalid immutable worktree revision %q", revisionSpec)
	}
	return create(ctx, repositoryPath, runID, revisionSpec)
}

// CreateAtReference resolves an explicitly selected Git ref to an immutable
// commit before creating the worktree. This avoids a branch moving between
// selection and worktree creation.
func CreateAtReference(ctx context.Context, repositoryPath, runID, reference string) (Worktree, error) {
	repository, err := repositoryRoot(ctx, repositoryPath)
	if err != nil {
		return Worktree{}, err
	}
	revisionSpec, err := resolveRevision(ctx, repository, reference)
	if err != nil {
		return Worktree{}, err
	}
	return create(ctx, repository, runID, revisionSpec)
}

func create(ctx context.Context, repositoryPath, runID, revisionSpec string) (Worktree, error) {
	if !runIDPattern.MatchString(runID) {
		return Worktree{}, fmt.Errorf("invalid run id %q", runID)
	}
	repository, err := repositoryRoot(ctx, repositoryPath)
	if err != nil {
		return Worktree{}, err
	}
	base := filepath.Join(filepath.Dir(repository), filepath.Base(repository)+"-gator-runs")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return Worktree{}, fmt.Errorf("create Gator worktree directory: %w", err)
	}
	path := filepath.Join(base, runID)
	if _, err := os.Lstat(path); err == nil {
		return Worktree{}, fmt.Errorf("Gator worktree already exists for run %q", runID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Worktree{}, fmt.Errorf("inspect Gator worktree path: %w", err)
	}

	command := exec.CommandContext(ctx, "git", "worktree", "add", "--detach", path, revisionSpec)
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err != nil {
		return Worktree{}, commandError("create isolated Git worktree", err, output)
	}
	root, err := workspace.Open(path)
	if err != nil {
		return Worktree{}, fmt.Errorf("open created Gator worktree: %w", err)
	}
	baseCommit, err := revision(ctx, root.Path())
	if err != nil {
		return Worktree{}, err
	}
	return Worktree{ID: runID, Repository: repository, Path: root.Path(), BaseCommit: baseCommit, Root: root}, nil
}

// OpenExisting validates a retained worktree before a resumed agent run uses
// it. The caller supplies its original repository identity from local session
// state; Git confirms that the target is still a worktree.
func OpenExisting(ctx context.Context, repository, path, runID string) (Worktree, error) {
	if !runIDPattern.MatchString(runID) {
		return Worktree{}, fmt.Errorf("invalid run id %q", runID)
	}
	root, err := workspace.Open(path)
	if err != nil {
		return Worktree{}, fmt.Errorf("open retained worktree: %w", err)
	}
	command := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	command.Dir = root.Path()
	output, err := command.CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "true" {
		if err != nil {
			return Worktree{}, commandError("validate retained Git worktree", err, output)
		}
		return Worktree{}, errors.New("retained path is not a Git worktree")
	}
	baseCommit, err := revision(ctx, root.Path())
	if err != nil {
		return Worktree{}, err
	}
	return Worktree{ID: runID, Repository: repository, Path: root.Path(), BaseCommit: baseCommit, Root: root}, nil
}

func revision(ctx context.Context, directory string) (string, error) {
	command := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		return "", commandError("read worktree base revision", err, output)
	}
	revision := strings.TrimSpace(string(output))
	if revision == "" {
		return "", errors.New("Git did not return a worktree base revision")
	}
	return revision, nil
}

func repositoryRoot(ctx context.Context, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("repository path is required")
	}
	command := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	command.Dir = path
	output, err := command.CombinedOutput()
	if err != nil {
		return "", commandError("find Git repository root", err, output)
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", errors.New("Git did not return a repository root")
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve Git repository root: %w", err)
	}
	return canonical, nil
}

func resolveRevision(ctx context.Context, repository, reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" || strings.ContainsAny(reference, "\x00\r\n") {
		return "", fmt.Errorf("invalid worktree base reference %q", reference)
	}
	command := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "--end-of-options", reference+"^{commit}")
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err != nil {
		return "", commandError("resolve worktree base reference", err, output)
	}
	revision := strings.TrimSpace(string(output))
	if !commitPattern.MatchString(revision) {
		return "", fmt.Errorf("Git returned an invalid worktree base revision for %q", reference)
	}
	return revision, nil
}

const (
	worktreeIncludePath = ".gator/worktreeinclude"
	maxSetupFiles       = 128
	maxSetupFileBytes   = 8 * 1024 * 1024
	maxSetupTotalBytes  = 32 * 1024 * 1024
)

func ignoredFileSetup(ctx context.Context, repository string) ([]ignoredFile, error) {
	if err := requireTrackedSetupFile(ctx, repository); err != nil {
		return nil, err
	}
	root, err := workspace.Open(repository)
	if err != nil {
		return nil, err
	}
	contents, err := root.ReadRegularFile(worktreeIncludePath, 64*1024)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", worktreeIncludePath, err)
	}
	var files []ignoredFile
	seen := make(map[string]struct{})
	total := 0
	for lineNumber, line := range strings.Split(string(contents), "\n") {
		relative := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if relative == "" || strings.HasPrefix(relative, "#") {
			continue
		}
		if len(files) == maxSetupFiles {
			return nil, fmt.Errorf("%s lists more than %d files", worktreeIncludePath, maxSetupFiles)
		}
		relative = filepath.ToSlash(relative)
		clean := path.Clean(relative)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
			return nil, fmt.Errorf("%s line %d is not a repository-relative file", worktreeIncludePath, lineNumber+1)
		}
		if _, duplicate := seen[clean]; duplicate {
			return nil, fmt.Errorf("%s lists %q more than once", worktreeIncludePath, clean)
		}
		if err := requireIgnored(ctx, repository, clean); err != nil {
			return nil, err
		}
		data, err := root.ReadRegularFile(filepath.FromSlash(clean), maxSetupFileBytes)
		if err != nil {
			return nil, fmt.Errorf("read ignored setup file %q: %w", clean, err)
		}
		if total+len(data) > maxSetupTotalBytes {
			return nil, fmt.Errorf("ignored setup files exceed the %d MiB combined limit", maxSetupTotalBytes/(1024*1024))
		}
		seen[clean] = struct{}{}
		total += len(data)
		files = append(files, ignoredFile{path: clean, data: data})
	}
	return files, nil
}

func requireTrackedSetupFile(ctx context.Context, repository string) error {
	command := exec.CommandContext(ctx, "git", "ls-files", "--error-unmatch", "--", worktreeIncludePath)
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		return commandError("require tracked "+worktreeIncludePath, err, output)
	}
	return nil
}

func requireIgnored(ctx context.Context, repository, relative string) error {
	command := exec.CommandContext(ctx, "git", "check-ignore", "-q", "--", relative)
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return fmt.Errorf("worktree setup file %q is not ignored; add it to .gitignore before copying it", relative)
		}
		return commandError("check ignored worktree setup file", err, output)
	}
	return nil
}

func copyIgnoredFiles(root workspace.Root, files []ignoredFile) error {
	directory, err := os.OpenRoot(root.Path())
	if err != nil {
		return fmt.Errorf("open target worktree root: %w", err)
	}
	defer directory.Close()
	for _, file := range files {
		parent := path.Dir(file.path)
		if parent != "." {
			if err := directory.MkdirAll(filepath.FromSlash(parent), 0o700); err != nil {
				return fmt.Errorf("create target directory for %q: %w", file.path, err)
			}
		}
		if err := directory.WriteFile(filepath.FromSlash(file.path), file.data, 0o600); err != nil {
			return fmt.Errorf("write ignored setup file %q: %w", file.path, err)
		}
	}
	return nil
}

func commandError(action string, err error, output []byte) error {
	message := strings.TrimSpace(string(output))
	if message == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, message)
}
