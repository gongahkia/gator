package workapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestInitialPlanCorrectionWaitsForOldOwnerThenRequiresFreshReview(t *testing.T) {
	for _, scenario := range []string{"success", "second correction before cutover", "failure and retry", "cancel before cutover", "keyboard cancel before cutover", "mismatched keyboard cancellation"} {
		t.Run(scenario, func(t *testing.T) {
			config, store := dialogueConfig(t)
			config.RouteConversation = revisionRoute
			config.Run = func(string, string, string, RunOptions) RunResult {
				t.Fatal("unreviewed task executed")
				return RunResult{}
			}
			started := make(chan struct{})
			config.PreparePlan = func(ctx context.Context, _ worksession.Request) (worksession.PlanContent, error) {
				close(started)
				<-ctx.Done()
				return dialoguePlanContent(), nil
			}
			revisions, fail := 0, scenario == "failure and retry"
			config.RevisePlan = func(_ context.Context, request worksession.Request) (worksession.PlanContent, error) {
				revisions++
				if scenario == "second correction before cutover" && (len(request.Plan.Edit.Prior) != 1 || request.Plan.Edit.Instruction != "Also include accessible venues") {
					t.Fatal("initial handoff lost latest correction or history")
				}
				if lease, err := store.AcquireExecution(request.Plan.Binding.ConversationID); err == nil {
					lease.Close()
					t.Fatal("replacement did not own the execution lease")
				}
				if fail {
					return worksession.PlanContent{}, errors.New("provider disconnected")
				}
				content := dialoguePlanContent()
				content.Outcome = "Corrected initial venue plan"
				return content, nil
			}
			m := New(config).FocusBridgeSession(true).ConfigureDialogueHost(DialogueHost{CanStart: true})
			next, prepare := m.SubmitBridgeUploads(BridgeUploadSubmission{RequestID: "initial-original-task", Context: "conversation:" + m.conversation, Text: "Compare venues", PlanFirst: true, Files: []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/brief.txt"}})
			m = next.(Model)
			original := m.requests.Requests[0]
			finished := make(chan tea.Msg, 1)
			go func() { finished <- prepare() }()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("initial preparation did not start")
			}
			defer func() {
				if m.cancel != nil {
					m.cancel()
				}
			}()
			// An initial correction replaces a slot, even when no extra task fits.
			m = m.ConfigureDialogueHost(DialogueHost{CanStart: false})
			m, route := dialogueSubmit(t, m, "correct-initial-preparation", "Keep the venue comparison small")
			m, revision := admitLocalRevision(t, m, route)
			if revision != nil || !m.running || m.planPreparing != original.ID || m.planRevising != original.ID || m.requests.Requests[0].Plan.Phase != "revising" || revisions != 0 {
				t.Fatal("correction started before the original owner released its lease")
			}
			m, hello := dialogueSubmit(t, m, "hello-initial-cutover", "Hello during the revision")
			m = routeDialogue(t, m, hello)
			if m.dialogueTurns[1].State != "answered" || !m.running {
				t.Fatal("conversation stalled behind preparation cutover")
			}
			if scenario == "second correction before cutover" {
				m, route = dialogueSubmit(t, m, "second-initial-cutover", "Also include accessible venues")
				m, revision = admitLocalRevision(t, m, route)
				if revision != nil {
					t.Fatal("second correction raced initial preparation")
				}
			}
			if scenario == "cancel before cutover" {
				r := m.requests.Requests[0]
				next, _ := m.ApplyTaskPlan(TaskPlanAction{ConversationID: m.conversation, TaskID: r.ID, Revision: r.Plan.Revision, Digest: r.Plan.Digest, Action: "cancel", RequestID: "cancel-initial-cutover"})
				m = next.(Model)
				if m.planResult.Error != "" {
					t.Fatal(m.planResult.Error)
				}
			}
			if scenario == "keyboard cancel before cutover" {
				next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
				m = next.(Model)
			}
			if scenario == "mismatched keyboard cancellation" {
				m.planEditTurn = "a-different-retained-correction"
				next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
				m = next.(Model)
			}
			select {
			case value := <-finished:
				next, revision = m.Update(value)
				m = next.(Model)
			case <-time.After(5 * time.Second):
				t.Fatal("obsolete preparation did not retire")
			}
			if scenario == "mismatched keyboard cancellation" {
				retained, err := store.LoadRequests(m.conversation)
				if err != nil {
					t.Fatal(err)
				}
				if revision != nil || retained.Requests[0].State != "queued" || retained.Requests[0].Plan.Edit.State != "preparing" || revisions != 0 {
					t.Fatal("a mismatched correction owner cancelled or started other work")
				}
				return
			}
			if scenario == "cancel before cutover" || scenario == "keyboard cancel before cutover" {
				if revision != nil || m.running || revisions != 0 || m.requests.Requests[0].State != "discarded" || m.dialogueTurns[0].State != "interrupted" {
					t.Fatal("cancelled correction started a replacement or lost its outcome")
				}
				return
			}
			if revision == nil || !m.running || m.planPreparing != "" {
				t.Fatal("replacement did not acquire its original task owner")
			}
			next, _ = m.Update(revision())
			m = next.(Model)
			if scenario == "failure and retry" {
				if m.running || m.requests.Requests[0].State != "interrupted" || m.dialogueTurns[0].State != "failed" || m.conversationTasks()[0].CanRevisePlan {
					t.Fatal("initial failure lost the retained task or explicit retry control")
				}
				m = m.ConfigureDialogueHost(DialogueHost{CanStart: true})
				if !m.conversationTasks()[0].CanRevisePlan {
					t.Fatal("a free task slot did not restore explicit retry")
				}
				config.StartConversationID = m.conversation
				m = New(config).FocusBridgeSession(true).ConfigureDialogueHost(DialogueHost{CanStart: true})
				if revisions != 1 || m.running || m.requests.Requests[0].ID != original.ID {
					t.Fatal("restart replayed or replaced the failed preparation")
				}
				fail = false
				m, route = dialogueSubmit(t, m, "retry-original-initial-plan", "Retry that original plan")
				m, revision = admitLocalRevision(t, m, route)
				if revision == nil {
					t.Fatal("explicit retry was not scheduled")
				}
				next, _ = m.Update(revision())
				m = next.(Model)
			}
			r := m.requests.Requests[0]
			var before, after bytes.Buffer
			if json.Compact(&before, original.Options) != nil || json.Compact(&after, r.Options) != nil || !bytes.Equal(before.Bytes(), after.Bytes()) || r.ID != original.ID || r.Text != original.Text || r.Plan.Budget != original.Plan.Budget || r.Plan.Binding != original.Plan.Binding {
				t.Fatal("correction changed task identity, inputs, scope or limits")
			}
			if m.running || r.Plan.Content == nil || r.Plan.Content.Outcome != "Corrected initial venue plan" || r.Plan.Approval != nil || r.Plan.Phase != "review" {
				t.Fatal("corrected proposal acquired execution authority or was not retained")
			}
			if revisions != map[bool]int{false: 1, true: 2}[scenario == "failure and retry"] {
				t.Fatal("correction replayed model work")
			}
		})
	}
}

func TestInitialPreparationCorrectionCanCrossOnlyItsFirstUnapprovedProposal(t *testing.T) {
	for _, change := range []string{"first proposal", "manual edit", "approval", "stop"} {
		t.Run(change, func(t *testing.T) {
			config, store := dialogueConfig(t)
			config.PreparePlan = func(context.Context, worksession.Request) (worksession.PlanContent, error) {
				return dialoguePlanContent(), nil
			}
			config.RevisePlan = func(_ context.Context, r worksession.Request) (worksession.PlanContent, error) {
				if r.Plan.Edit.BaseRevision != 1 || r.Plan.Content == nil {
					t.Fatal("correction did not bind the current first proposal")
				}
				content := dialoguePlanContent()
				content.Outcome = "Corrected first proposal"
				return content, nil
			}
			config.RouteConversation = revisionRoute
			if change == "stop" {
				config.RouteConversation = func(_ context.Context, r workrun.ConversationRequest, _ ConversationInput) (workrun.ConversationDecision, error) {
					return workrun.ConversationDecision{Kind: "stop", Target: r.Tasks[0].ID}, nil
				}
			}
			m := New(config).FocusBridgeSession(true).ConfigureDialogueHost(DialogueHost{CanStart: true})
			next, prepare := m.SubmitBridgeUploads(BridgeUploadSubmission{RequestID: "initial-boundary-task", Context: "conversation:" + m.conversation, Text: "Compare venues", PlanFirst: true})
			m = next.(Model)
			original := m.requests.Requests[0]
			m, route := dialogueSubmit(t, m, "initial-boundary-correction", "Keep the venue comparison small")
			// Routing captured initial preparation; its first proposal arrives now.
			next, _ = m.Update(prepare())
			m = next.(Model)
			current := m.requests.Requests[0]
			if change == "manual edit" {
				content := dialoguePlanContent()
				content.Outcome = "An independently edited proposal"
				journal, err := store.ReviseTaskPlan(m.conversation, current.ID, current.Plan.Revision, current.Plan.Digest, content)
				if err != nil {
					t.Fatal(err)
				}
				m.requests = journal
			}
			if change == "approval" {
				journal, claimed, err := store.ApproveTaskPlan(m.conversation, current.ID, current.Plan.Revision, current.Plan.Digest)
				if err != nil || !claimed {
					t.Fatalf("approval precondition: %v", err)
				}
				m.requests = journal
			}
			if change != "first proposal" {
				next, command := m.Update(route())
				m = next.(Model)
				if command != nil || m.dialogueTurns[0].State != "failed" || m.planRevising != "" {
					t.Fatal("a changed review or non-correction reused the preparation control")
				}
				return
			}
			m, command := admitLocalRevision(t, m, route)
			if command == nil {
				t.Fatal("automatic first proposal forced the user to resend an unambiguous correction")
			}
			next, _ = m.Update(command())
			m = next.(Model)
			r := m.requests.Requests[0]
			if r.ID != original.ID || r.Plan.Content.Outcome != "Corrected first proposal" || r.Plan.Approval != nil || r.Plan.Revision != 3 {
				t.Fatal("correction replaced task identity or reused old review")
			}
		})
	}
}
