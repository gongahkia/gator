package workapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/followup"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
)

func productKey(m Model, key tea.KeyMsg) (Model, tea.Cmd) {
	next, cmd := m.Update(key)
	return next.(Model), cmd
}

func TestFollowupRequiresSeparateReviewAndSave(t *testing.T) {
	calls := 0
	m := New(Config{CurrentFolder: t.TempDir(), SaveFollowup: func(d followup.Draft) (goal.Record, jobs.Definition, error) {
		calls++
		if err := d.Validate(); err != nil {
			t.Fatal(err)
		}
		return goal.Record{ID: "goal-" + strings.Repeat("a", 24), Title: d.Title, SourcePath: d.SourcePath}, jobs.Definition{Name: d.Title}, nil
	}})
	next, _ := m.SubmitText("/followup new")
	m = next.(Model)
	if m.followupForm == nil || calls != 0 {
		t.Fatal("opening setup performed a write")
	}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyCtrlR})
	if !m.followupForm.Review || calls != 0 {
		t.Fatal("review performed a write")
	}
	panel := m.BridgeState().Panel
	if panel.Kind != "confirmation" {
		t.Fatal("agreement is not a confirmation")
	}
	for _, want := range []string{"Outcome:", "Sources:", "When:", "Notify when:", "Permitted actions:", "Limits:"} {
		if !strings.Contains(m.snapshotText(), want) {
			t.Fatalf("review omitted %s", want)
		}
	}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.followupForm.Review || calls != 0 {
		t.Fatal("edit did not return to the form")
	}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyCtrlR})
	m, cmd := productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if calls != 0 || !m.followupSaving || cmd == nil {
		t.Fatal("save should be asynchronous")
	}
	message := cmd()
	next, _ = m.Update(message)
	m = next.(Model)
	if calls != 1 || m.followupForm != nil || m.goalID == "" {
		t.Fatalf("save calls=%d goal=%s", calls, m.goalID)
	}
}

func TestCloudRecallNeedsExplicitUploadReview(t *testing.T) {
	calls := 0
	m := New(Config{RecallStatus: func(context.Context) (RecallStatus, error) { return RecallStatus{State: "Basic recall available"}, nil }, ConfigureRecall: func(provider string, upload bool) error {
		calls++
		if provider != "openai" || !upload {
			t.Fatal("wrong cloud choice")
		}
		return nil
	}})
	next, cmd := m.SubmitText("/memory")
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	if calls != 0 || m.recallConfirm != "openai" {
		t.Fatal("cloud choice bypassed review")
	}
	p := m.BridgeState().Panel
	if !strings.Contains(strings.Join(p.Blocks[0].Lines, "\n"), "uploads eligible saved memory and note text") {
		t.Fatal("missing upload explanation")
	}
	m, cmd = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if calls != 1 || cmd == nil {
		t.Fatal("cloud choice was not saved")
	}
}

func TestRecallInstallCancelsWithoutLeavingBusyState(t *testing.T) {
	started := make(chan struct{})
	m := New(Config{InstallRecall: func(ctx context.Context, progress func(string)) error { close(started); <-ctx.Done(); return ctx.Err() }})
	m.section, m.recallConfirm = "memory", "install"
	m, wait := productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	<-started
	if !m.recallBusy || wait == nil {
		t.Fatal("download did not start asynchronously")
	}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	next, _ := m.Update(wait())
	m = next.(Model)
	if m.recallBusy || !strings.Contains(m.sectionNotice, "stopped") {
		t.Fatalf("cancel status: %s", m.sectionNotice)
	}
}

func TestContextOmissionAppliesToExactlyTheNextRun(t *testing.T) {
	var supplied RunOptions
	m := New(Config{CurrentFolder: t.TempDir(), ReadContext: func(string, string) (ContextState, error) {
		return ContextState{Items: []KnowledgeItem{{ID: "mem-example", Title: "Preference", Text: "short", Scope: "global", Available: true}}}, nil
	}, Run: func(_, _, _ string, o RunOptions) RunResult { supplied = o; return RunResult{FinalText: "Done"} }})
	next, _ := m.SubmitText("/context")
	m = next.(Model)
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	next, cmd := m.SubmitText("Explain the plan?")
	m = next.(Model)
	cmd()
	if !sliceContains(supplied.ExcludedKnowledgeIDs, "mem-example") || len(m.options.ExcludedKnowledgeIDs) != 0 {
		t.Fatal("one-run exclusion was lost or persisted")
	}
}

func TestProductProgressKeepsSpecialistMechanicsInDiagnostics(t *testing.T) {
	m := New(Config{})
	next, _ := m.Update(agent.Event{Kind: agent.EventSubagent, TaskID: "task-private", Text: `{"ID":"task-private","Role":"source_researcher","Status":"running"}`})
	m = next.(Model)
	if strings.Contains(m.status, "task-private") || strings.Contains(m.status, "source_researcher") {
		t.Fatalf("internal progress leaked: %s", m.status)
	}
	if !strings.Contains(m.status, "sources") {
		t.Fatal("missing useful progress")
	}
	if _, ok := m.tasks["task-private"]; !ok {
		t.Fatal("diagnostics discarded")
	}
}

func TestFollowupFailedSavePreservesReviewedAgreement(t *testing.T) {
	m := New(Config{CurrentFolder: t.TempDir(), SaveFollowup: func(followup.Draft) (goal.Record, jobs.Definition, error) {
		return goal.Record{}, jobs.Definition{}, errors.New("source access changed")
	}})
	m.newFollowup()
	m.followupForm.Review = true
	m, cmd := productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.followupForm == nil || !m.followupForm.Review || !strings.Contains(m.sectionNotice, "source access") {
		t.Fatal("failed save lost the agreement")
	}
}

func TestFollowupShortcutsReturnToTheSameDraft(t *testing.T) {
	m := New(Config{CurrentFolder: t.TempDir()})
	m.newFollowup()
	m.followupForm.Draft.Outcome = "Keep the revised launch on track"
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyF1})
	panel := m.BridgeState().Panel
	if panel.Kind != "help" || !strings.Contains(strings.Join(panel.Blocks[0].Lines, "\n"), "Ctrl+R") {
		t.Fatal("form shortcuts are unreachable")
	}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.productHelp || m.followupForm == nil || m.followupForm.Draft.Outcome != "Keep the revised launch on track" {
		t.Fatal("returning from shortcuts lost the current draft")
	}
}
