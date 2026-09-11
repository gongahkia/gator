package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// prepareDarwin uses macOS Seatbelt through sandbox-exec. The generated policy
// intentionally grants only the worktree, a private scratch directory, the
// Git worktree metadata, explicit policy roots, and standard runtime paths.
func prepareDarwin(ctx context.Context, root, scratch string, argv, environment []string, policy Policy) (*exec.Cmd, error) {
	sandboxExec, err := exec.LookPath("sandbox-exec")
	if err != nil {
		return nil, errors.New("strict sandbox requires sandbox-exec on macOS")
	}
	executable, err := darwinExecutable(ctx, argv[0])
	if err != nil {
		return nil, err
	}
	readRoots, writeRoots, err := sandboxRoots(root, scratch, policy)
	if err != nil {
		return nil, err
	}
	profile := darwinProfile(readRoots, writeRoots, policy.Network)
	command := exec.CommandContext(ctx, sandboxExec, "-p", profile, "--", executable)
	command.Args = append(command.Args, argv[1:]...)
	command.Dir = root
	command.Env = environment
	return command, nil
}

// Apple's /usr/bin/git is an xcrun shim. Invoking that shim inside Seatbelt
// makes xcrun try to update a cache in the user's global temporary directory,
// which is intentionally outside Gator's sandbox. Resolve the selected
// developer-tool Git binary before entering the sandbox instead.
func darwinExecutable(ctx context.Context, name string) (string, error) {
	executable, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	if executable != "/usr/bin/git" {
		return executable, nil
	}
	command := exec.CommandContext(ctx, "/usr/bin/xcrun", "--find", "git")
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("resolve macOS developer-tool Git: %w", err)
	}
	git := strings.TrimSpace(string(output))
	if git == "" || !filepath.IsAbs(git) {
		return "", errors.New("xcrun returned an invalid Git executable")
	}
	return git, nil
}

func darwinProfile(readRoots, writeRoots []string, network Network) string {
	var lines []string
	lines = append(lines,
		"(version 1)",
		"(deny default)",
		"(import \"system.sb\")",
		"(allow process*)",
	)
	for _, directory := range ancestorDirectories(append(append([]string(nil), readRoots...), writeRoots...)) {
		lines = append(lines, "(allow file-read-metadata (literal \""+escapeSBPL(directory)+"\"))")
	}
	for _, root := range readRoots {
		lines = append(lines, "(allow file-read* (subpath \""+escapeSBPL(root)+"\"))")
	}
	for _, root := range writeRoots {
		lines = append(lines, "(allow file-write* (subpath \""+escapeSBPL(root)+"\"))")
	}
	if network == AllowNetwork {
		lines = append(lines, "(allow network*)")
	} else {
		lines = append(lines, "(deny network*)")
	}
	return strings.Join(lines, "\n")
}

// Seatbelt requires metadata access to each parent directory while a process
// resolves an allowed deep path. Granting only literal ancestor metadata keeps
// sibling contents unreadable while allowing Git worktree pointers to reach
// their separately retained common metadata directory.
func ancestorDirectories(roots []string) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, root := range roots {
		for parent := filepath.Dir(filepath.Clean(root)); parent != "." && parent != string(filepath.Separator); parent = filepath.Dir(parent) {
			if _, exists := seen[parent]; exists {
				continue
			}
			seen[parent] = struct{}{}
			result = append(result, parent)
		}
	}
	sort.Strings(result)
	return result
}

func escapeSBPL(value string) string {
	return strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(value)
}

func sandboxRoots(root, scratch string, policy Policy) ([]string, []string, error) {
	readRoots := []string{
		root,
		scratch,
		"/System",
		"/usr",
		"/bin",
		"/sbin",
		"/Library/Apple",
		"/Library/Developer",
		"/opt/homebrew",
		"/private/var/select",
	}
	if moduleCache := goModuleCache(); moduleCache != "" {
		readRoots = append(readRoots, moduleCache)
	}
	writeRoots := []string{root, scratch}
	if len(policy.WritablePaths) > 0 {
		writeRoots = []string{scratch}
		for _, path := range policy.WritablePaths {
			candidate := filepath.Join(root, path)
			resolved, err := filepath.EvalSymlinks(candidate)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, nil, err
			}
			relative, err := filepath.Rel(root, resolved)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return nil, nil, errors.New("writable path escapes worktree")
			}
			writeRoots = append(writeRoots, resolved)
		}
	}
	gitRead, gitWrite, err := gitMetadataRoots(root)
	if err != nil {
		return nil, nil, err
	}
	readRoots = append(readRoots, gitRead...)
	writeRoots = append(writeRoots, gitWrite...)
	readRoots = append(readRoots, policy.ReadOnlyRoots...)
	readRoots = append(readRoots, policy.WritableRoots...)
	writeRoots = append(writeRoots, policy.WritableRoots...)
	return uniqueExistingRoots(readRoots), uniqueExistingRoots(writeRoots), nil
}

func uniqueExistingRoots(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		clean := filepath.Clean(value)
		info, err := osStat(clean)
		if err != nil || (!info.IsDir() && !info.Mode().IsRegular()) {
			continue
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	sort.Strings(result)
	return result
}

func gitMetadataRoots(root string) ([]string, []string, error) {
	gitFile := filepath.Join(root, ".git")
	contents, err := osReadFile(gitFile)
	if err != nil {
		// A non-Git test workspace still benefits from sandboxing.
		return nil, nil, nil
	}
	const prefix = "gitdir: "
	value := strings.TrimSpace(string(contents))
	if !strings.HasPrefix(value, prefix) {
		return nil, nil, nil
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(value, prefix))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(root, gitDir)
	}
	gitDir, err = filepath.EvalSymlinks(gitDir)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve Git worktree metadata: %w", err)
	}
	readRoots := []string{gitDir}
	writeRoots := []string{gitDir}
	commonFile := filepath.Join(gitDir, "commondir")
	if common, readErr := osReadFile(commonFile); readErr == nil {
		commonDir := strings.TrimSpace(string(common))
		if !filepath.IsAbs(commonDir) {
			commonDir = filepath.Join(gitDir, commonDir)
		}
		commonDir, resolveErr := filepath.EvalSymlinks(commonDir)
		if resolveErr != nil {
			return nil, nil, fmt.Errorf("resolve common Git metadata: %w", resolveErr)
		}
		readRoots = append(readRoots, commonDir)
	}
	return readRoots, writeRoots, nil
}
