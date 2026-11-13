package workapp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

func revisionRoute(_ context.Context, request workrun.ConversationRequest, _ ConversationInput) (workrun.ConversationDecision, error) {
	if request.Text == "Hello during the revision" {
		return workrun.ConversationDecision{Kind: "reply", Text: "You can keep talking while I revise the plan."}, nil
	}
	for _, task := range request.Tasks {
		if task.CanRevisePlan {
			return workrun.ConversationDecision{Kind: "revise_plan", Target: task.ID, Instruction: request.Text}, nil
		}
	}
	return workrun.ConversationDecision{Kind: "clarify", Text: "Which pending plan should I revise?"}, nil
}

func prepareRevisionFixture(t *testing.T, config Config, text string) Model {
	t.Helper()
	m := New(config).FocusBridgeSession(true).ConfigureDialogueHost(DialogueHost{CanStart: true})
	next, prepare := m.SubmitBridgeUploads(BridgeUploadSubmission{RequestID: "revision-original-plan", Context: "conversation:" + m.conversation, Text: text, PlanFirst: true, Files: []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/brief.txt"}})
	m = next.(Model)
	if prepare == nil {
		t.Fatal("original proposal was not scheduled", m.bridgeUploadResult)
	}
	next, _ = m.Update(prepare())
	m = next.(Model)
	if m.requests.Requests[0].Plan.Phase != "review" {
		t.Fatal("original plan not retained")
	}
	return m
}

func admitLocalRevision(t *testing.T, m Model, route tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	next, _ := m.Update(route())
	m = next.(Model)
	if m.dialogueDispatch == nil || m.dialogueDispatch.Kind != "revise_plan" || m.dialogueDispatch.ChildID != m.conversation {
		t.Fatal("revision did not cross host admission")
	}
	dispatch := *m.dialogueDispatch
	m, command := m.ControlDialogue(DialogueControl{ParentID: m.conversation, TurnID: dispatch.OwnerTask, RequestID: dispatch.TurnID, RunID: dispatch.TargetRun, Action: "revise_plan", Instruction: dispatch.Instruction})
	if m.dialogueControlResult.Error != "" {
		t.Fatal(m.dialogueControlResult.Error)
	}
	next, _ = m.FinishDialogueDispatch(DialogueDispatchResult{TurnID: dispatch.TurnID, ChildID: dispatch.ChildID, RevisingPlan: true})
	return next.(Model), command
}

func TestConversationalPlanRevisionKeepsTalkingAndRequiresFreshReview(t *testing.T) {
	config, store := dialogueConfig(t)
	runs := 0
	config.Run = func(string, string, string, RunOptions) RunResult {
		runs++
		return RunResult{FinalText: "The freshly reviewed task finished."}
	}
	config.PreparePlan = func(context.Context, worksession.Request) (worksession.PlanContent, error) {
		return dialoguePlanContent(), nil
	}
	config.RouteConversation = revisionRoute
	started := make(chan worksession.Request, 1)
	release := make(chan struct{})
	config.RevisePlan = func(ctx context.Context, r worksession.Request) (worksession.PlanContent, error) {
		started <- r
		select {
		case <-release:
			content := dialoguePlanContent()
			content.Outcome = "A smaller reviewed comparison"
			return content, nil
		case <-ctx.Done():
			return worksession.PlanContent{}, ctx.Err()
		}
	}
	m := prepareRevisionFixture(t, config, "Compare venues")
	original := m.requests.Requests[0]
	m.options.MaxSteps = 99 // Future preferences cannot replace the task's frozen budget.
	m, route := dialogueSubmit(t, m, "revision-request-0001", "Keep the comparison small")
	m, revision := admitLocalRevision(t, m, route)
	var next tea.Model
	if revision == nil || !m.running || m.planRevising != original.ID || m.dialogueTurns[0].State != "editing_plan" {
		t.Fatal("correction became separate work or was not admitted")
	}
	finished := make(chan tea.Msg, 1)
	go func() { finished <- revision() }()
	var captured worksession.Request
	select {
	case captured = <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("revision model did not start")
	}
	defer m.cancel()
	var options RunOptions
	if json.Unmarshal(captured.Options, &options) != nil || captured.ID != original.ID || captured.Text != original.Text || captured.Plan.Budget != original.Plan.Budget || options.MaxSteps == 99 || len(options.Uploads) != 1 || captured.Plan.Approval != nil || captured.Plan.Edit.Instruction != "Keep the comparison small" {
		t.Fatal("revision lost original identity, inputs or scope")
	}
	m, hello := dialogueSubmit(t, m, "revision-greeting-001", "Hello during the revision")
	m = routeDialogue(t, m, hello)
	if !m.running || m.dialogueTurns[1].State != "answered" || runs != 0 {
		t.Fatal("conversation waited for the model revision or ran unreviewed work")
	}
	close(release)
	select {
	case value := <-finished:
		next, _ = m.Update(value)
		m = next.(Model)
	case <-time.After(5 * time.Second):
		t.Fatal("revision did not finish")
	}
	revised := m.requests.Requests[0]
	if m.running || revised.ID != original.ID || revised.Plan.Phase != "review" || revised.Plan.Content.Outcome != "A smaller reviewed comparison" || m.dialogueTurns[0].State != "guided" || revised.Plan.Approval != nil || runs != 0 {
		t.Fatal("revision outcome was not retained for fresh review")
	}
	old := TaskPlanAction{ConversationID: m.conversation, TaskID: original.ID, Revision: original.Plan.Revision, Digest: original.Plan.Digest, Action: "approve", RequestID: "old-review"}
	next, command := m.ApplyTaskPlan(old)
	m = next.(Model)
	if command != nil || m.planResult.Error == "" || runs != 0 {
		t.Fatal("old approval started the revised task")
	}
	old.Revision, old.Digest, old.RequestID = revised.Plan.Revision, revised.Plan.Digest, "fresh-review"
	next, command = m.ApplyTaskPlan(old)
	m = next.(Model)
	if command == nil || m.planResult.Error != "" {
		t.Fatal("fresh exact approval was unavailable", m.planResult.Error)
	}
	next, _ = m.Update(command())
	m = next.(Model)
	if runs != 1 {
		t.Fatal("freshly reviewed task did not execute once")
	}
	turn, err := store.ReadDialogue(m.conversation, m.dialogueTurns[0].ID)
	if err != nil || turn.State != "guided" {
		t.Fatal("durable correction outcome lost", err)
	}
}

func TestDelegatedPlanRevisionReportsToOriginalConversationWithoutChangingFocus(t *testing.T) {
	config, store := dialogueConfig(t)
	config.PreparePlan = func(context.Context, worksession.Request) (worksession.PlanContent, error) {
		return dialoguePlanContent(), nil
	}
	calls := 0
	config.RevisePlan = func(context.Context, worksession.Request) (worksession.PlanContent, error) {
		calls++
		content := dialoguePlanContent()
		content.Outcome = "A corrected original plan"
		return content, nil
	}
	config.RouteConversation = func(context.Context, workrun.ConversationRequest, ConversationInput) (workrun.ConversationDecision, error) {
		return workrun.ConversationDecision{Kind: "task"}, nil
	}
	parent := New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
	parent.running = true
	next, route := parent.SubmitBridgeUploads(BridgeUploadSubmission{RequestID: "original-delegated-plan", Context: "conversation:" + parent.conversation, Text: "Compare venues", PlanFirst: true})
	parent = routeDialogue(t, next.(Model), route)
	launch := *parent.dialogueDispatch
	childConfig := config
	childConfig.StartConversationID = launch.ChildID
	child := New(childConfig)
	next, prepare := child.BootstrapDialogue(parent.conversation, launch.TurnID)
	child = next.(Model)
	next, _ = child.Update(prepare())
	child = next.(Model)
	next, _ = parent.FinishDialogueDispatch(DialogueDispatchResult{TurnID: launch.TurnID, ChildID: launch.ChildID})
	parent = next.(Model)
	parent = parent.ConfigureDialogueHost(DialogueHost{CanStart: true, Peers: []DialoguePeer{child.dialogueTaskState().DialoguePeer}})
	parent.config.RouteConversation = revisionRoute
	parent, route = dialogueSubmit(t, parent, "correct-delegated-plan", "Keep the comparison small")
	parent = routeDialogue(t, parent, route)
	dispatch := *parent.dialogueDispatch
	control := DialogueControl{ParentID: parent.conversation, TurnID: launch.TurnID, RequestID: dispatch.TurnID, RunID: dispatch.TargetRun, Action: "revise_plan", Instruction: dispatch.Instruction}
	child, revision := child.ControlDialogue(control)
	if revision == nil || child.dialogueControlResult.Error != "" || !child.dialogueControlResult.RevisingPlan {
		t.Fatal("original owner did not admit revision", child.dialogueControlResult)
	}
	unconfirmedParent := parent
	next, _ = parent.FinishDialogueDispatch(DialogueDispatchResult{TurnID: dispatch.TurnID, ChildID: dispatch.ChildID, RevisingPlan: true})
	parent = next.(Model)
	if parent.dialogueDispatch != nil || !parent.running || parent.dialogueTurns[1].State != "editing_plan" {
		t.Fatal("parent waited for revision or changed its work")
	}
	next, _ = child.Update(revision())
	child = next.(Model)
	next, _ = unconfirmedParent.FinishDialogueDispatch(DialogueDispatchResult{TurnID: dispatch.TurnID, ChildID: dispatch.ChildID, Error: "Owner acknowledgement was lost"})
	unconfirmedParent = next.(Model)
	if unconfirmedParent.dialogueDispatch != nil || unconfirmedParent.dialogueTurns[1].State != "guided" || calls != 1 {
		t.Fatal("transport error overwrote a confirmed result or replayed work")
	}
	parent = parent.ConfigureDialogueHost(DialogueHost{CanStart: true, Peers: []DialoguePeer{child.dialogueTaskState().DialoguePeer}})
	if parent.dialogueTurns[1].State != "guided" || !parent.running || !child.background || calls != 1 {
		t.Fatal("actual correction result/focus was not preserved")
	}
	child, replay := child.ControlDialogue(control)
	if replay != nil || child.dialogueControlResult.Error != "" || calls != 1 {
		t.Fatal("control acknowledgement retry repeated model work")
	}
	turn, err := store.ReadDialogue(parent.conversation, dispatch.TurnID)
	if err != nil || turn.State != "guided" {
		t.Fatal("original conversation lost the result", err)
	}
	config.StartConversationID = parent.conversation
	restored := New(config)
	if calls != 1 || restored.dialogueTurns[1].State != "guided" || restored.dialogueRouting != "" {
		t.Fatal("restart lost the result or replayed correction")
	}
}

func TestInterruptedPlanRevisionRetainsPreviousDraftWithoutReplaying(t *testing.T) {
	config, _ := dialogueConfig(t)
	config.PreparePlan = func(context.Context, worksession.Request) (worksession.PlanContent, error) {
		return dialoguePlanContent(), nil
	}
	calls := 0
	config.RevisePlan = func(ctx context.Context, _ worksession.Request) (worksession.PlanContent, error) {
		calls++
		return worksession.PlanContent{}, ctx.Err()
	}
	config.RouteConversation = revisionRoute
	m := prepareRevisionFixture(t, config, "Compare venues")
	original := m.requests.Requests[0]
	m, route := dialogueSubmit(t, m, "orphan-revision-0001", "Keep the comparison small")
	m, revision := admitLocalRevision(t, m, route)
	m.cancel()
	_ = revision() // Provider ended/lease closed, but its native completion was lost.
	config.StartConversationID = m.conversation
	restored := New(config)
	if calls != 1 || restored.running || len(restored.queue) != 0 || restored.requests.Requests[0].Plan.Phase != "review" || restored.requests.Requests[0].Plan.Content.Outcome != original.Plan.Content.Outcome || restored.requests.Requests[0].Plan.Approval != nil || restored.dialogueTurns[0].State != "failed" {
		t.Fatal("orphaned revision replayed or lost the original reviewable draft")
	}
}

func TestPlanRevisionRechecksCapacityWithoutChangingTheRetainedReview(t *testing.T) {
	config, _ := dialogueConfig(t)
	config.PreparePlan = func(context.Context, worksession.Request) (worksession.PlanContent, error) {
		return dialoguePlanContent(), nil
	}
	config.RevisePlan = func(context.Context, worksession.Request) (worksession.PlanContent, error) {
		t.Fatal("capacity rejection called model")
		return worksession.PlanContent{}, nil
	}
	m := prepareRevisionFixture(t, config, "Compare venues")
	original := m.requests.Requests[0]
	control := m.conversationTasks()[0].ControlID
	m = m.ConfigureDialogueHost(DialogueHost{CanStart: false})
	if m.conversationTasks()[0].CanRevisePlan || !m.conversationTasks()[0].CanStop {
		t.Fatal("capacity blocked stopping or advertised a new revision")
	}
	next, command, err := m.startTaskPlanRevision(original.ID, control, m.conversation, "not-admitted-turn", "Keep it small")
	if err == nil || command != nil || next.requests.Requests[0].Plan.Digest != original.Plan.Digest || next.requests.Requests[0].Plan.Edit != nil {
		t.Fatal("capacity rejection changed the old plan or launched work")
	}
}

func TestAdditionalCorrectionsDrainOriginalRevisionBeforeLatestStarts(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run(map[bool]string{false: "replace", true: "stop while draining"}[stop], func(t *testing.T) {
			config, store := dialogueConfig(t)
			config.PreparePlan = func(context.Context, worksession.Request) (worksession.PlanContent, error) {
				return dialoguePlanContent(), nil
			}
			config.RouteConversation = revisionRoute
			entered := make(chan worksession.Request, 3)
			drain := make(chan struct{})
			config.RevisePlan = func(ctx context.Context, r worksession.Request) (worksession.PlanContent, error) {
				entered <- r
				if len(r.Plan.Edit.Prior) == 0 {
					<-ctx.Done()
					<-drain
					return worksession.PlanContent{}, ctx.Err()
				}
				content := dialoguePlanContent()
				content.Outcome = "Latest corrected comparison"
				return content, nil
			}
			m := prepareRevisionFixture(t, config, "Compare venues")
			original := m.requests.Requests[0]
			m, route := dialogueSubmit(t, m, "first-running-correction", "Keep the budget small")
			m, first := admitLocalRevision(t, m, route)
			done := make(chan tea.Msg, 1)
			go func() { done <- first() }()
			<-entered
			m, route = dialogueSubmit(t, m, "second-running-correction", "Also keep the original sources")
			m, second := admitLocalRevision(t, m, route)
			if second != nil {
				t.Fatal("replacement started before old lease released")
			}
			m, route = dialogueSubmit(t, m, "third-running-correction", "Include accessible venues")
			m, third := admitLocalRevision(t, m, route)
			if third != nil {
				t.Fatal("third correction started another concurrent call")
			}
			retained := m.requests.Requests[0]
			if len(retained.Plan.Edit.Prior) != 2 || retained.Plan.Edit.Prior[0].Instruction != "Keep the budget small" || retained.Plan.Approval != nil {
				t.Fatal("earlier instructions or review boundary lost")
			}
			select {
			case <-entered:
				t.Fatal("concurrent revision call")
			default:
			}
			if stop {
				m.interruptRun()
			}
			close(drain)
			next, latest := m.Update(<-done)
			m = next.(Model)
			if stop {
				if latest != nil || m.running || m.requests.Requests[0].State != "discarded" {
					t.Fatal("stopped handoff revived work")
				}
			} else {
				if latest == nil {
					t.Fatal("latest retained correction did not start after drain")
				}
				next, _ = m.Update(latest())
				m = next.(Model)
				final := m.requests.Requests[0]
				if final.ID != original.ID || final.Plan.Phase != "review" || final.Plan.Approval != nil || final.Plan.Content.Outcome != "Latest corrected comparison" || len(final.Plan.Edit.Prior) != 2 {
					t.Fatal("replacement not retained in same task for fresh review")
				}
				if _, _, err := store.ApproveTaskPlan(m.conversation, original.ID, retained.Plan.Revision, retained.Plan.Digest); err == nil {
					t.Fatal("pre-result approval accepted")
				}
			}
			for _, prior := range retained.Plan.Edit.Prior {
				turn, err := store.ReadDialogue(prior.ParentID, prior.TurnID)
				if err != nil || turn.State != "interrupted" {
					t.Fatal("superseded attempt outcome lost", err)
				}
			}
		})
	}
}

func TestRestartRetainsUnaccountedPlanExecutionWithoutReviewOrReplay(t *testing.T) {
	config, store := dialogueConfig(t)
	config.PreparePlan = func(context.Context, worksession.Request) (worksession.PlanContent, error) {
		return dialoguePlanContent(), nil
	}
	m := prepareRevisionFixture(t, config, "Compare venues")
	r := m.requests.Requests[0]
	j, _, err := store.ApproveTaskPlan(m.conversation, r.ID, r.Plan.Revision, r.Plan.Digest)
	if err != nil {
		t.Fatal(err)
	}
	j.Requests[0].State = "running"
	j.Requests[0].Plan.Phase = "dispatching"
	if _, err := store.SaveRequests(m.conversation, j); err != nil {
		t.Fatal(err)
	}
	turn, err := store.AdmitDialogue(m.conversation, "pause-for-recovery", "Correct the running plan", json.RawMessage(`{"scope":"same"}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"routing", "ready", "editing_plan"} {
		turn.State = phase
		if phase == "ready" {
			turn.Decision, _ = json.Marshal(map[string]string{"kind": "revise_plan", "target": r.ID, "instruction": turn.Text})
		}
		if phase == "editing_plan" {
			turn.Response = "Requesting a revised plan"
		}
		turn, err = store.UpdateDialogue(turn, turn.Revision)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, _, _, err := store.BeginRunningPlanEdit(m.conversation, r.ID, r.Plan.Revision, r.Plan.Digest, m.conversation, turn.ID, turn.Text, "unaccounted-work-run"); err != nil {
		t.Fatal(err)
	}
	config.StartConversationID = m.conversation
	config.RevisePlan = func(context.Context, worksession.Request) (worksession.PlanContent, error) {
		t.Fatal("restart replayed plan")
		return worksession.PlanContent{}, nil
	}
	restored := New(config)
	if restored.requestsIssue != "" || restored.running {
		t.Fatal("could not recover unaccounted execution", restored.requestsIssue)
	}
	saved := restored.requests.Requests[0]
	if saved.State != "interrupted" || saved.Plan.Phase != "interrupted" || saved.Plan.DrainingRun != "unaccounted-work-run" || saved.Plan.Approval != nil {
		t.Fatal("unaccounted execution became reviewable or lost identity")
	}
	if _, _, err := store.ApproveTaskPlan(m.conversation, r.ID, saved.Plan.Revision, saved.Plan.Digest); err == nil {
		t.Fatal("unaccounted execution acquired approval")
	}
}
