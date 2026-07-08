package gather

import (
	"context"
	"os/exec"
	"strings"

	"github.com/gongahkia/paw/internal/envelope"
)

const truncateMarker = "\n[truncated]\n"

func collectGitUnits(ctx context.Context, cwd string, maxBytes int) ([]envelope.RawUnit, error) {
	if cwd == "" {
		return nil, nil
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxFileBytes
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, nil
	}
	if !insideGitWorktree(ctx, cwd) {
		return nil, nil
	}
	status, err := gitOutput(ctx, cwd, "status", "--porcelain", "--untracked-files=all", "--", ".")
	if err != nil || strings.TrimSpace(status) == "" {
		return nil, nil
	}
	var units []envelope.RawUnit
	units = appendGitUnit(units, "git_status", "", status, maxBytes)
	if stat, err := gitOutput(ctx, cwd, "diff", "--relative", "--stat", "--", "."); err == nil {
		units = appendGitUnit(units, "git_diff_stat", "", stat, maxBytes)
	}
	if stagedStat, err := gitOutput(ctx, cwd, "diff", "--cached", "--relative", "--stat", "--", "."); err == nil {
		units = appendGitUnit(units, "git_diff_stat", "", stagedStat, maxBytes)
	}
	if diff, err := gitOutput(ctx, cwd, "diff", "--relative", "--", "."); err == nil {
		units = appendGitUnit(units, "git_diff_unstaged", "", diff, maxBytes)
	}
	if diff, err := gitOutput(ctx, cwd, "diff", "--cached", "--relative", "--", "."); err == nil {
		units = appendGitUnit(units, "git_diff_staged", "", diff, maxBytes)
	}
	return units, nil
}

func insideGitWorktree(ctx context.Context, cwd string) bool {
	out, err := gitOutput(ctx, cwd, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

func gitOutput(ctx context.Context, cwd string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func appendGitUnit(units []envelope.RawUnit, kind, path, text string, maxBytes int) []envelope.RawUnit {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return units
	}
	return append(units, envelope.RawUnit{
		Kind: kind,
		Path: path,
		Text: truncateUnitText(text+"\n", maxBytes),
	})
}

func truncateUnitText(text string, maxBytes int) string {
	if maxBytes <= 0 || len(text) <= maxBytes {
		return text
	}
	if maxBytes <= len(truncateMarker) {
		return text[:maxBytes]
	}
	return text[:maxBytes-len(truncateMarker)] + truncateMarker
}
