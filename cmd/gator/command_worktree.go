package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var worktreeID = regexp.MustCompile(`\A[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}\z`)

const worktreeUsage = `usage:
  gator worktree list
  gator worktree prune
  gator worktree remove RUN_ID --yes`

// worktreeCommand manages only Gator's sibling worktrees. It never performs
// implicit cleanup, because retained worktrees may contain reviewable changes.
func worktreeCommand(arguments []string, out io.Writer) error {
	if len(arguments) == 0 {
		return errors.New(worktreeUsage)
	}
	directory, err := os.Getwd()
	if err != nil {
		return err
	}
	repository, err := gitRepositoryRoot(directory)
	if err != nil {
		return errors.New("worktree management must start inside a Git checkout")
	}
	repository, err = filepath.EvalSymlinks(repository)
	if err != nil {
		return err
	}
	base := filepath.Join(filepath.Dir(repository), filepath.Base(repository)+"-gator-runs")
	switch arguments[0] {
	case "list":
		if len(arguments) != 1 {
			return errors.New(worktreeUsage)
		}
		return listManagedWorktrees(base, out)
	case "prune":
		if len(arguments) != 1 {
			return errors.New(worktreeUsage)
		}
		command := exec.CommandContext(context.Background(), "git", "worktree", "prune", "--verbose")
		command.Dir = repository
		output, err := command.CombinedOutput()
		if err != nil {
			return gitCommandError("prune stale Git worktree metadata", err, output)
		}
		if _, err := fmt.Fprint(out, string(output)); err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "Pruned stale Git worktree metadata. Retained Gator worktrees were not removed.")
		return err
	case "remove":
		if len(arguments) != 3 || arguments[2] != "--yes" || !worktreeID.MatchString(arguments[1]) {
			return errors.New("removing a retained worktree deletes its files; use: gator worktree remove RUN_ID --yes")
		}
		target := filepath.Join(base, arguments[1])
		info, err := os.Lstat(target)
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("retained Gator worktree %q does not exist", arguments[1])
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
		_, err = fmt.Fprintf(out, "Removed retained Gator worktree %s. Its private run records remain available for transcript or patch export.\n", target)
		return err
	default:
		return fmt.Errorf("unknown worktree command %q\n%s", arguments[0], worktreeUsage)
	}
}

func listManagedWorktrees(base string, out io.Writer) error {
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		_, err = fmt.Fprintln(out, "No retained Gator worktrees.")
		return err
	}
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() && entry.Type()&os.ModeSymlink == 0 && worktreeID.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		_, err = fmt.Fprintln(out, "No retained Gator worktrees.")
		return err
	}
	for _, name := range names {
		if _, err := fmt.Fprintf(out, "%s\t%s\n", name, filepath.Join(base, name)); err != nil {
			return err
		}
	}
	return nil
}

func gitCommandError(action string, err error, output []byte) error {
	message := strings.TrimSpace(string(output))
	if message == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, message)
}
