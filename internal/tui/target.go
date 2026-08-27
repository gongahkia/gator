package tui

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/worktree"
)

func splitTargetAndInstruction(remainder string) (string, string) {
	remainder = strings.TrimSpace(remainder)
	if remainder == "" {
		return "", ""
	}
	fields := strings.Fields(remainder)
	if len(fields) == 1 {
		return fields[0], ""
	}
	instruction := strings.TrimSpace(strings.TrimPrefix(remainder, fields[0]))
	return fields[0], instruction
}

func (m Model) bindRetainedTarget(operation, remainder string) (tea.Model, tea.Cmd) {
	target, instruction := splitTargetAndInstruction(remainder)
	if target == "" {
		switch operation {
		case "resume":
			return m.openRecentRuns()
		case "fork":
			if m.resumeStatePath == "" {
				m.notice = notice{text: "Continue a retained thread before choosing a turn to fork, or pass a thread ID / run record path.", kind: noticeInfo}
				return m, nil
			}
			return m.openThreadTree(m.screen)
		case "clone":
			if m.resumeStatePath == "" {
				m.notice = notice{text: "Continue a retained thread before cloning it, or pass a thread ID / run record path.", kind: noticeInfo}
				return m, nil
			}
			return m.beginFork(m.resumeStatePath)
		}
	}
	resolved, err := journal.ResolveRetainedTarget(m.config.StateDir, m.config.RepositoryPath, target, m.recentAll)
	if err != nil {
		m.notice = notice{text: err.Error(), kind: noticeError}
		return m, nil
	}
	var next tea.Model
	var command tea.Cmd
	switch operation {
	case "fork", "clone":
		next, command = m.beginFork(resolved.HeadStatePath)
	default:
		next, command = m.beginContinuation(resolved.HeadStatePath)
	}
	updated := next.(Model)
	if instruction == "" {
		return updated, command
	}
	updated.task.SetValue(instruction)
	updated.refreshPreflight()
	return updated.startRun()
}

func (m Model) openReviewRecord(statePath string) (tea.Model, tea.Cmd) {
	session, err := journal.LoadSession(statePath)
	if err != nil {
		m.notice = notice{text: "Load retained review run: " + err.Error(), kind: noticeError}
		return m, nil
	}
	info, err := os.Stat(session.WorktreePath)
	if err != nil || !info.IsDir() {
		m.notice = notice{text: "The retained worktree for this run record no longer exists.", kind: noticeError}
		return m, nil
	}
	m.outcome = &gatorrun.Outcome{
		StatePath: statePath,
		ThreadID:  session.ThreadID,
		Worktree: worktree.Worktree{
			Repository: session.Repository,
			Path:       session.WorktreePath,
			BaseCommit: session.BaseCommit,
		},
	}
	m.screen = reviewScreen
	m.notice = notice{text: "Refreshing the retained-worktree review…", kind: noticeInfo}
	return m, loadReview(*m.outcome)
}
