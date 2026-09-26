package worktui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/delivery"
	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestGlobalHistoryUsesCanonicalStoreForAllWorkDetailAndReview(t *testing.T) {
	state, project := t.TempDir(), t.TempDir()
	sessions, err := worksession.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := sessions.Create("Project work", project, "snap-history", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	history, err := workhistory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	deliveries, err := delivery.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	learnings, err := learning.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Add(-time.Minute)
	writeHistoryRecord(t, history, "work-completed", "Prepare the project report", conversation.ID, action.Draft, workhistory.Completed, base, filepath.Join(state, "bundles", "work-completed", "manifest.json"))
	writeHistoryRecord(t, history, "work-failed", "Repair the failed import", "", action.Draft, workhistory.Failed, base.Add(10*time.Second), "")
	writeHistoryRecord(t, history, "work-running", "Inspect the active issue", "", action.Inspect, workhistory.Running, base.Add(20*time.Second), "")
	if _, err := learnings.RecordObservation(learning.ObservationInput{ID: "observation-history-ui", Signal: learning.UserCorrected, WorkID: "work-completed", Summary: "Use concise sections."}); err != nil {
		t.Fatal(err)
	}
	if _, err := learnings.Create(learning.Create{ID: "learning-history-ui", Type: learning.Preference, Key: "report-style", Content: "Use concise sections.", Scope: learning.Scope{Kind: learning.Project, Value: project}, Origin: learning.Inferred, Confidence: 80, Provenance: learning.Provenance{WorkIDs: []string{"work-completed"}, EvidenceRefs: []string{"gator/observations/observation-history-ui.json"}}}); err != nil {
		t.Fatal(err)
	}
	var reviewPath string
	model := New(Config{
		CurrentFolder: project, HistoryStore: &history, DeliveryStore: &deliveries, LearningStore: &learnings, SessionStore: &sessions,
		BundleAction: func(request BundleActionRequest) (string, error) {
			reviewPath = request.BundlePath
			if request.Action != "preview" {
				t.Fatalf("history action = %#v", request)
			}
			return "Reviewed retained artifact.", nil
		},
	})
	model.width, model.height = 120, 60
	model = submitTUICommand(t, model, "/history")
	if model.section != "history" || len(model.historyItems) != 3 || model.historyItems[0].Record.ID != "work-running" || !strings.Contains(model.View(), "Prepare the project report") || !strings.Contains(model.View(), "Repair the failed import") {
		t.Fatalf("global history = %#v\n%s", model.historyItems, model.View())
	}
	for index, item := range model.historyItems {
		if item.Record.ID == "work-completed" {
			model.selected = index
		}
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.historyDetail == nil || model.historyDetail.Record.ID != "work-completed" || model.historyDetail.Project != project {
		t.Fatalf("history detail = %#v", model.historyDetail)
	}
	view := model.View()
	for _, want := range []string{"Snapshot: snap-history", "not delivered", "user_corrected", "Use concise sections."} {
		if !strings.Contains(view, want) {
			t.Fatalf("history detail omitted %q:\n%s", want, view)
		}
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model = updated.(Model)
	if reviewPath != filepath.Join(state, "bundles", "work-completed") || !strings.Contains(model.View(), "Reviewed retained artifact.") {
		t.Fatalf("history review did not use shared bundle action: path=%q\n%s", reviewPath, model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	model = updated.(Model)
	if model.historyFilter != workhistory.Running || len(model.historyItems) != 1 || model.historyItems[0].Record.ID != "work-running" {
		t.Fatalf("history status filter = %q %#v", model.historyFilter, model.historyItems)
	}
}

func TestLearningsUseCanonicalStoreForStatusProvenanceApprovalReversalAndEdit(t *testing.T) {
	state, project := t.TempDir(), t.TempDir()
	store, err := learning.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := store.Create(learning.Create{ID: "learning-candidate-ui", Type: learning.Preference, Key: "package-manager", Content: "Use pnpm.", Scope: learning.Scope{Kind: learning.Project, Value: project}, Origin: learning.Inferred, Confidence: 91, Provenance: learning.Provenance{WorkIDs: []string{"work-correction"}, EvidenceRefs: []string{"gator/observations/observation-correction.json"}}})
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.Create(learning.Create{ID: "learning-active-ui", Type: learning.Preference, Key: "report-format", Content: "Use Markdown.", Scope: learning.Scope{Kind: learning.Global}, Origin: learning.UserAuthored})
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := store.Create(learning.Create{ID: "learning-disabled-ui", Type: learning.Procedure, Key: "check", Content: "Run the focused check.", Scope: learning.Scope{Kind: learning.Global}, Origin: learning.UserAuthored})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Disable(disabled.ID); err != nil {
		t.Fatal(err)
	}
	rejected, err := store.Create(learning.Create{ID: "learning-rejected-ui", Type: learning.Preference, Key: "old-format", Content: "Use PDF.", Scope: learning.Scope{Kind: learning.Global}, Origin: learning.Inferred, Confidence: 40})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reject(rejected.ID); err != nil {
		t.Fatal(err)
	}
	model := New(Config{CurrentFolder: project, LearningStore: &store})
	model.width, model.height = 120, 60
	model = submitTUICommand(t, model, "/learnings")
	if model.section != "learnings" || len(model.learningItems) != 4 {
		t.Fatalf("learning list = %#v\n%s", model.learningItems, model.View())
	}
	for _, status := range []string{"active", "candidate", "disabled", "rejected"} {
		if !strings.Contains(model.View(), status) {
			t.Fatalf("learning list omitted %s:\n%s", status, model.View())
		}
	}
	selectLearningID(t, &model, candidate.ID)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.learningDetail == nil || !strings.Contains(model.View(), "work-correction") || !strings.Contains(model.View(), "observation-correction") || !strings.Contains(model.View(), "Confidence: 91") {
		t.Fatalf("candidate provenance detail = %#v\n%s", model.learningDetail, model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(Model)
	if record, err := store.Load(candidate.ID); err != nil || record.Status != learning.Active || model.learningDetail.Status != learning.Active {
		t.Fatalf("candidate approval = %#v, %v", record, err)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(Model)
	if record, err := store.Load(candidate.ID); err != nil || record.Status != learning.Disabled {
		t.Fatalf("TUI disable = %#v, %v", record, err)
	}
	projected, err := store.Projection(learning.Context{Project: project})
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range projected {
		if record.ID == candidate.ID {
			t.Fatalf("disabled learning remained active for future Work: %#v", projected)
		}
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model = updated.(Model)
	if model.learningForm == nil {
		t.Fatal("new learning did not open the TUI form")
	}
	model.learningForm.Scope = learning.Scope{Kind: learning.Project, Value: project}
	model.learningForm.Key = "research-output"
	model.learningForm.Content = "Use plain Markdown."
	model.saveLearningForm()
	if model.learningDetail == nil || model.learningDetail.Origin != learning.UserAuthored || model.learningDetail.Status != learning.Active {
		t.Fatalf("TUI explicit learning = %#v", model.learningDetail)
	}
	createdID := model.learningDetail.ID
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	model = updated.(Model)
	if model.learningForm == nil || model.learningForm.EditingID != createdID {
		t.Fatalf("TUI edit form = %#v", model.learningForm)
	}
	model.learningForm.Content = "Use concise Markdown."
	model.saveLearningForm()
	if record, err := store.Load(createdID); err != nil || record.Content != "Use concise Markdown." {
		t.Fatalf("TUI edit did not use canonical learning store: %#v, %v", record, err)
	}
	if active.ID == "" {
		t.Fatal("active fixture was not created")
	}
}

func writeHistoryRecord(t *testing.T, store workhistory.Store, id, objective, conversationID string, mode action.Mode, status workhistory.Status, started time.Time, manifestPath string) {
	t.Helper()
	if _, err := store.Start(workhistory.Start{ID: id, Objective: objective, Mode: mode, ExternalActions: action.Forbid, ConversationID: conversationID, StartedAt: started}); err != nil {
		t.Fatal(err)
	}
	if status == workhistory.Running {
		return
	}
	succeeded := status == workhistory.Completed
	verification := workhistory.VerificationPassed
	artifactStatus := "completed"
	if !succeeded {
		verification, artifactStatus = workhistory.VerificationFailed, "failed"
	}
	if _, err := store.Finish(id, workhistory.Finish{ConversationID: conversationID, RevisionID: id, SnapshotID: "snap-history", ArtifactManifestPath: manifestPath, ArtifactStatus: artifactStatus, VerificationStatus: verification, Succeeded: succeeded, FinishedAt: started.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
}

func submitTUICommand(t *testing.T, model Model, command string) Model {
	t.Helper()
	model.input = command
	updated, run := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if run != nil {
		t.Fatalf("%s unexpectedly scheduled a command", command)
	}
	return updated.(Model)
}

func selectLearningID(t *testing.T, model *Model, id string) {
	t.Helper()
	for index, record := range model.learningItems {
		if record.ID == id {
			model.selected = index
			return
		}
	}
	t.Fatalf("learning %q was not listed: %#v", id, model.learningItems)
}
