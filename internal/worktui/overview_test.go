package worktui

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workhome"
)

func TestOverviewHandoffRequiresExplicitReviewAndKeepsSecretsOutOfBridge(t *testing.T) {
	calls := 0
	m := New(Config{BridgeMode: true, ControlCommand: func(args []string) *exec.Cmd {
		calls++
		if strings.Join(args, " ") != "work answer run-one 1" {
			t.Fatal("wrong exact decision")
		}
		command := exec.Command("true")
		command.Env = append(os.Environ(), "SECRET_DO_NOT_PROJECT=credential-value", "GATOR_STATE_DIR=/selected-profile")
		return command
	}, Controls: func(context.Context, string, string, []string) (ControlScreen, error) {
		return ControlScreen{Panel: uisnapshot.Panel{Kind: "overview", Title: "Pending review"}, Actions: []ControlAction{{Key: "A", Label: "Open exact review", Review: true, External: []string{"work", "answer", "run-one", "1"}, Description: "Open the exact action review; this does not approve it."}}}, nil
	}})
	cmd := m.openControls("overview", nil)
	next, _ := m.Update(cmd())
	m = next.(Model)
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if calls != 0 || m.BridgeState().External != nil || !m.controlReview {
		t.Fatal("review was skipped")
	}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	state := m.BridgeState()
	if calls != 1 || state.External == nil || state.External.Kind != "control" {
		t.Fatal("terminal review was not handed off")
	}
	encoded, _ := json.Marshal(state)
	if strings.Contains(string(encoded), "credential-value") || strings.Contains(string(encoded), "SECRET_DO_NOT_PROJECT") {
		t.Fatal("external handoff exposed ambient credentials")
	}
	if !strings.Contains(string(encoded), "/selected-profile") {
		t.Fatal("state profile override was lost")
	}
	next, cmd = m.FinishBridgeExternal("")
	m = next.(Model)
	if m.bridgeExternal != nil || cmd == nil {
		t.Fatal("handoff did not return to the overview")
	}
}
func TestOverviewLinksOpenExactGoalAndPreserveHistory(t *testing.T) {
	selected := ""
	m := New(Config{SelectGoal: func(id string) (GoalState, error) {
		selected = id
		return GoalState{Title: "Project", SourcePath: "/project"}, nil
	}, PlanAction: func(string) (string, error) { return "Review project plan", nil }})
	m.controlFamily = "overview"
	m.controlScreen = ControlScreen{Panel: uisnapshot.Panel{Kind: "overview"}, Actions: []ControlAction{{Key: "G", Label: "Review this plan", GoalID: "goal-project", Command: "/plan"}}}
	m.section = "controls"
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if selected != "goal-project" || m.goalID != "goal-project" || m.title != "Project" {
		t.Fatal("link opened the wrong goal")
	}
}
func TestOverviewRejectsStaleReadAfterDeletingHistory(t *testing.T) {
	m := New(Config{})
	generation := m.overviewGeneration
	m.clearRemovedData(ControlScreen{ResetConversation: true})
	next, _ := m.Update(overviewRefreshed{Generation: generation, View: workhome.Overview{At: time.Now()}})
	m = next.(Model)
	if !m.overview.At.IsZero() {
		t.Fatal("an old async result resurrected deleted overview content")
	}
}
func TestRelinkUpdatesCurrentWorkspaceWithoutErasingTranscript(t *testing.T) {
	old, nextDir := t.TempDir(), t.TempDir()
	m := New(Config{})
	m.source = old
	m.messages = []message{{role: "Gator", text: "Retained historical decision"}}
	m.clearRemovedData(ControlScreen{SourceMoves: []SourceMove{{From: old, To: nextDir}}})
	if m.source != nextDir || !m.options.RefreshSource || len(m.messages) != 1 {
		t.Fatal("source relink lost conversation or reused stale source")
	}
}

func TestControlFormTabDoesNotOpenTheWorkspacePicker(t *testing.T) {
	m := New(Config{Controls: func(context.Context, string, string, []string) (ControlScreen, error) {
		return ControlScreen{Panel: uisnapshot.Panel{Kind: "data", Title: "Source folders"}, Actions: []ControlAction{{Key: "L", Label: "Relink", Action: "save", Fields: []ControlField{{Label: "Old folder"}, {Label: "New folder"}}, Review: true}}}, nil
	}})
	cmd := m.openControls("data", nil)
	next, _ := m.Update(cmd())
	m = next.(Model)
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.launcher || m.section != "controls" || m.controlField != 1 {
		t.Fatal("Tab escaped the form instead of selecting its next field")
	}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/new/project")})
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyCtrlR})
	if !m.controlReview || m.controlForm.Fields[1].Value != "/new/project" {
		t.Fatal("the replacement folder was not retained for review")
	}
}

func TestOverviewEscReturnsThroughSectionsToTheEverydayHome(t *testing.T) {
	m := New(Config{Controls: func(_ context.Context, _ string, _ string, args []string) (ControlScreen, error) {
		return ControlScreen{Panel: uisnapshot.Panel{Kind: "overview", Title: "Overview"}}, nil
	}})
	next, cmd := m.SubmitText("/overview")
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	m.controlRootArgs = []string{"blocked", "source-one"}
	for remaining := 1; remaining >= 0; remaining-- {
		m, cmd = productKey(m, tea.KeyMsg{Type: tea.KeyEsc})
		if len(m.controlRootArgs) != remaining || cmd == nil {
			t.Fatal("Esc skipped an overview navigation level")
		}
		next, _ = m.Update(cmd())
		m = next.(Model)
	}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if !m.home || m.section != "" {
		t.Fatal("overview did not return to its original home")
	}
}

func TestOverviewBackgroundRefreshKeepsTheSelectedItemAndCanBeCancelled(t *testing.T) {
	m := New(Config{Controls: func(context.Context, string, string, []string) (ControlScreen, error) {
		return ControlScreen{Location: "needs", Panel: uisnapshot.Panel{Kind: "overview", Title: "What needs me"}, Actions: []ControlAction{{Key: "1", Action: "item", Args: []string{"needs", "new"}}, {Key: "2", Action: "item", Args: []string{"needs", "existing"}}}}, nil
	}})
	m.section, m.controlFamily = "controls", "overview"
	m.controlRootArgs = []string{"needs"}
	m.controlScreen = ControlScreen{Location: "needs", Actions: []ControlAction{{Key: "1", Action: "item", Args: []string{"needs", "existing"}}}}
	m.selected = 0
	cmd := m.refreshControlBackground()
	if m.controlBusy || !m.controlBackground || strings.Contains(m.controlsPanel().Notice, "Working") {
		t.Fatal("periodic refresh interrupted the reader")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.selected != 1 || m.controlBackground {
		t.Fatal("a new item reset the user's selection")
	}
	cmd = m.refreshControlBackground()
	stale := cmd()
	m.cancelControlBackground()
	next, _ = m.Update(stale)
	m = next.(Model)
	if m.controlBackground || m.selected != 1 {
		t.Fatal("cancelled background refresh replaced navigation state")
	}
}
