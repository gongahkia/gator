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
	readRoots, writeRoots, err := sandboxRoots(root, scratch, policy)
	if err != nil {
		return nil, err
	}
	profile := darwinProfile(readRoots, writeRoots, policy.Network)
	command := exec.CommandContext(ctx, sandboxExec, "-p", profile, "--", argv[0])
	command.Args = append(command.Args, argv[1:]...)
	command.Dir = root
	command.Env = environment
	return command, nil
}

func darwinProfile(readRoots, writeRoots []string, network Network) string {
	var lines []string
	lines = append(lines,
		"(version 1)",
		"(deny default)",
		"(import \"system.sb\")",
		"(allow process*)",
	)
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
		if err != nil || !info.IsDir() {
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
