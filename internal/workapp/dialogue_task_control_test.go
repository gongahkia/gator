package workapp

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

func dialoguePlanContent() worksession.PlanContent {
	return worksession.PlanContent{Outcome: "A reviewed venue comparison", Steps: []worksession.PlanStep{{ID: "compare", Text: "Compare venues within the selected scope"}}, Effects: []string{"read"}}
}

func TestDialogueStopsExactLocalPlanDuringPreparationAndReview(t *testing.T) {
	for _, phase := range []string{"preparing", "review", "approved"} {
		t.Run(phase, func(t *testing.T) {
			config, store := dialogueConfig(t)
			config.Run = func(string, string, string, RunOptions) RunResult { t.Fatal("cancelled plan ran"); return RunResult{} }
			config.PreparePlan = func(context.Context, worksession.Request) (worksession.PlanContent, error) {
				return dialoguePlanContent(), nil
			}
			config.RouteConversation = func(_ context.Context, request workrun.ConversationRequest, _ ConversationInput) (workrun.ConversationDecision, error) {
				if len(request.Tasks) != 1 || !request.Tasks[0].CanStop || request.Tasks[0].CanGuide {
					t.Fatalf("plan did not expose safe controls: %+v", request.Tasks)
				}
				return workrun.ConversationDecision{Kind: "stop", Target: request.Tasks[0].ID}, nil
			}
			m := New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
			next, prepare := m.SubmitBridgeUploads(BridgeUploadSubmission{RequestID: "local-plan-admission", Context: "conversation:" + m.conversation, Text: "Compare venues", PlanFirst: true})
			m = next.(Model)
			if prepare == nil {
				t.Fatal("plan preparation unavailable")
			}
			if phase != "preparing" {
				next, _ = m.Update(prepare())
				m = next.(Model)
			}
			r := m.requests.Requests[0]
			if phase == "approved" {
				j, _, err := store.ApproveTaskPlan(m.conversation, r.ID, r.Plan.Revision, r.Plan.Digest)
				if err != nil {
					t.Fatal(err)
				}
				m.requests = j
			}
			if !m.conversationActive() {
				t.Fatal("pending plan made ordinary conversation unavailable")
			}
			m, route := dialogueSubmit(t, m, "cancel-local-plan-0001", "Cancel that plan")
			m = routeDialogue(t, m, route)
			if m.dialogueTurns[0].State != "stopped" {
				t.Fatal("cancellation was not acknowledged", m.dialogueTurns[0])
			}
			j, err := store.LoadRequests(m.conversation)
			if err != nil || len(j.Requests) != 1 || j.Requests[0].State != "discarded" || j.Requests[0].Plan.Approval != nil {
				t.Fatal("cancellation was not retained or retained approval", err)
			}
			var before, after bytes.Buffer
			if json.Compact(&before, r.Options) != nil || json.Compact(&after, j.Requests[0].Options) != nil || !bytes.Equal(before.Bytes(), after.Bytes()) {
				t.Fatal("cancellation changed the frozen task options")
			}
			if phase == "preparing" {
				next, _ = m.Update(prepare())
				m = next.(Model)
				if m.running || m.requests.Requests[0].Plan.Content != nil {
					t.Fatal("late proposal revived cancelled preparation")
				}
			}
			m, retry := dialogueSubmit(t, m, "cancel-local-plan-0001", "Cancel that plan")
			if retry != nil || len(m.dialogueTurns) != 1 {
				t.Fatal("admission retry replayed cancellation")
			}
			config.StartConversationID = m.conversation
			restored := New(config)
			if restored.running || len(restored.queue) != 0 || restored.requests.Requests[0].State != "discarded" {
				t.Fatal("restart revived cancelled plan")
			}
		})
	}
}

func TestDialogueRejectsPlanRevisionChangedDuringRouting(t *testing.T) {
	for _, refreshOwner := range []bool{false, true} {
		t.Run(map[bool]string{false: "persisted revision", true: "owner revision"}[refreshOwner], func(t *testing.T) {
			config, store := dialogueConfig(t)
			config.PreparePlan = func(context.Context, worksession.Request) (worksession.PlanContent, error) {
				return dialoguePlanContent(), nil
			}
			config.RouteConversation = func(_ context.Context, request workrun.ConversationRequest, _ ConversationInput) (workrun.ConversationDecision, error) {
				return workrun.ConversationDecision{Kind: "stop", Target: request.Tasks[0].ID}, nil
			}
			m := New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
			next, prepare := m.SubmitBridgeUploads(BridgeUploadSubmission{RequestID: "changing-plan-admission", Context: "conversation:" + m.conversation, Text: "Compare venues", PlanFirst: true})
			m = next.(Model)
			next, _ = m.Update(prepare())
			m = next.(Model)
			m, route := dialogueSubmit(t, m, "cancel-changing-plan-01", "Cancel that plan")
			routed := route()
			r := m.requests.Requests[0]
			content := dialoguePlanContent()
			content.Outcome = "An edited venue comparison"
			j, err := store.ReviseTaskPlan(m.conversation, r.ID, r.Plan.Revision, r.Plan.Digest, content)
			if err != nil {
				t.Fatal(err)
			}
			if refreshOwner {
				m.requests = j
			}
			next, _ = m.Update(routed)
			m = next.(Model)
			if m.dialogueTurns[0].State != "failed" {
				t.Fatal("stale cancellation was accepted")
			}
			j, err = store.LoadRequests(m.conversation)
			if err != nil || j.Requests[0].State != "queued" || j.Requests[0].Plan.Content.Outcome != content.Outcome {
				t.Fatal("stale control changed the revised plan", err)
			}
		})
	}
}

func TestDialogueStopsDelegatedPlanThroughItsActualOwner(t *testing.T) {
	for _, phase := range []string{"preparing", "review"} {
		t.Run(phase, func(t *testing.T) {
			config, store := dialogueConfig(t)
			config.Run = func(string, string, string, RunOptions) RunResult {
				t.Fatal("cancelled delegated plan ran")
				return RunResult{}
			}
			config.PreparePlan = func(context.Context, worksession.Request) (worksession.PlanContent, error) {
				return dialoguePlanContent(), nil
			}
			config.RouteConversation = func(context.Context, workrun.ConversationRequest, ConversationInput) (workrun.ConversationDecision, error) {
				return workrun.ConversationDecision{Kind: "task"}, nil
			}
			parent := New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
			parent.running = true
			next, route := parent.SubmitBridgeUploads(BridgeUploadSubmission{RequestID: "delegate-plan-000001", Context: "conversation:" + parent.conversation, Text: "Compare venues", PlanFirst: true})
			parent = routeDialogue(t, next.(Model), route)
			dispatch := *parent.dialogueDispatch
			childConfig := config
			childConfig.StartConversationID = dispatch.ChildID
			child := New(childConfig)
			next, prepare := child.BootstrapDialogue(parent.conversation, dispatch.TurnID)
			child = next.(Model)
			if child.dialogueBootstrapResult.Error != "" || prepare == nil {
				t.Fatal("delegated preparation unavailable", child.dialogueBootstrapResult)
			}
			if phase == "review" {
				next, _ = child.Update(prepare())
				child = next.(Model)
			}
			next, _ = parent.FinishDialogueDispatch(DialogueDispatchResult{TurnID: dispatch.TurnID, ChildID: dispatch.ChildID})
			parent = next.(Model)
			state := child.dialogueTaskState()
			if state == nil || !state.CanStop || state.CanGuide || state.RunID == "" {
				t.Fatal("delegated plan controls missing", state)
			}
			parent = parent.ConfigureDialogueHost(DialogueHost{CanStart: true, Peers: []DialoguePeer{state.DialoguePeer}})
			parent.config.RouteConversation = func(_ context.Context, request workrun.ConversationRequest, _ ConversationInput) (workrun.ConversationDecision, error) {
				return workrun.ConversationDecision{Kind: "stop", Target: state.ConversationID}, nil
			}
			parent, route = dialogueSubmit(t, parent, "stop-delegate-plan-01", "Stop the venue plan")
			parent = routeDialogue(t, parent, route)
			stop := parent.dialogueDispatch
			if stop == nil || stop.TargetRun != state.RunID {
				t.Fatal("lost exact target", stop)
			}
			control := DialogueControl{ParentID: parent.conversation, TurnID: state.TurnID, RequestID: stop.TurnID, RunID: stop.TargetRun, Action: "stop"}
			child, _ = child.ControlDialogue(control)
			if child.dialogueControlResult.Error != "" {
				t.Fatal(child.dialogueControlResult.Error)
			}
			child, _ = child.ControlDialogue(control)
			if child.dialogueControlResult.Error != "" {
				t.Fatal("same-control retry lost its receipt")
			}
			changed := control
			changed.Instruction = "different request"
			child, _ = child.ControlDialogue(changed)
			if child.dialogueControlResult.Error == "" {
				t.Fatal("reused control ID changed its meaning")
			}
			if phase == "preparing" {
				next, _ = child.Update(prepare())
				child = next.(Model)
			}
			if state := child.dialogueTaskState(); state.State != "stopped" || state.CanStop || state.CanGuide {
				t.Fatal("cancelled plan retained control", state)
			}
			next, _ = parent.FinishDialogueDispatch(DialogueDispatchResult{TurnID: stop.TurnID, ChildID: stop.ChildID})
			parent = next.(Model)
			if !parent.running || parent.dialogueTurns[len(parent.dialogueTurns)-1].State != "stopped" {
				t.Fatal("control stopped the parent or lost its receipt")
			}
			journal, err := store.LoadRequests(child.conversation)
			var frozen RunOptions
			if err != nil || len(journal.Requests) != 1 || json.Unmarshal(journal.Requests[0].Options, &frozen) != nil || frozen.DialogueParent != parent.conversation || journal.Requests[0].State != "discarded" {
				t.Fatal("original delegated identity lost", err)
			}
		})
	}
}
