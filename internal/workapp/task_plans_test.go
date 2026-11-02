package workapp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/worksession"
)

func TestTaskPlanAdmissionCannotBypassReviewOrReplayInterruptedWork(t *testing.T) {
	config, s := requestConfig(t, func(_, _, _ string, _ RunOptions) RunResult { t.Fatal("unapproved work ran"); return RunResult{} })
	m := New(config)
	opts := m.submissionOptions()
	opts.PlanFirst = true
	raw, _ := json.Marshal(retainedRunOptions(opts))
	j, r, err := s.AdmitPlanRequest(m.conversation, "Implement", raw, "plan-test-1", nil, worksession.PlanBudget{MaxSteps: 24}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	m.requests = j
	m.nextRequest = r.ID
	if release, err := m.admitRun("Implement", opts); err == nil {
		release()
		t.Fatal("unreviewed preparation dispatched")
	}
	content := worksession.PlanContent{Outcome: "Implement a checked patch", Steps: []worksession.PlanStep{{ID: "step-one", Text: "Implement and verify"}}, Effects: []string{"read", "code", "checks"}}
	j, err = s.ReviseTaskPlan(m.conversation, r.ID, 0, "", content)
	if err != nil {
		t.Fatal(err)
	}
	r = j.Requests[0]
	m.requests = j
	if release, err := m.admitRun("Implement", opts); err == nil {
		release()
		t.Fatal("review alone dispatched work")
	}
	j, _, err = s.ApproveTaskPlan(m.conversation, r.ID, r.Plan.Revision, r.Plan.Digest)
	if err != nil {
		t.Fatal(err)
	}
	m.requests = j
	changed := opts
	changed.MaxSteps++
	if release, err := m.admitRun("Implement", changed); err == nil {
		release()
		t.Fatal("different frozen options dispatched")
	}
	release, err := m.admitRun("Implement", opts)
	if err != nil {
		t.Fatal("approved request not claimed", err)
	}
	if m.requests.Requests[0].State != "running" || m.requests.Requests[0].Plan.Phase != "dispatching" {
		t.Fatal("claim was not durable")
	}
	release()
	// Reopening never inserts an interrupted plan into the generic replay queue.
	config.StartConversationID = m.conversation
	restored := New(config)
	if len(restored.queue) != 0 {
		t.Fatal("interrupted plan entered retry queue")
	}
	j, err = s.LoadRequests(m.conversation)
	if err != nil {
		t.Fatal(err)
	}
	if j.Requests[0].State != "interrupted" || j.Requests[0].Plan.Phase != "interrupted" || j.Requests[0].Plan.Approval == nil {
		t.Fatal("uncertainty or original approval evidence lost")
	}
	restored.requests = j
	restored.nextRequest = r.ID
	if release, err := restored.admitRun("Implement", opts); err == nil {
		release()
		t.Fatal("interrupted input replayed")
	}
}

func TestQueueChangesCannotConsumePendingPlanReview(t *testing.T) {
	config, s := requestConfig(t, func(_, _, _ string, _ RunOptions) RunResult { return RunResult{} })
	m := New(config)
	opts := m.submissionOptions()
	opts.PlanFirst = true
	raw, _ := json.Marshal(retainedRunOptions(opts))
	j, r, err := s.AdmitPlanRequest(m.conversation, "Reviewed task", raw, "plan-queue-1", nil, worksession.PlanBudget{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	m.requests = j
	m.queueSequence++
	if err := m.persistQueue([]queuedRun{{id: m.queueSequence, prompt: "A separate queued message", options: m.submissionOptions()}}); err != nil {
		t.Fatal(err)
	}
	j, err = s.LoadRequests(m.conversation)
	if err != nil || len(j.Requests) != 2 {
		t.Fatal("queue update removed plan", err)
	}
	for _, record := range j.Requests {
		if record.ID == r.ID && record.State != "queued" {
			t.Fatal("queue edit consumed review")
		}
	}
	if err := m.persistQueue([]queuedRun{{id: 1, requestID: r.ID, prompt: "Changed reviewed task", options: opts}}); err == nil {
		t.Fatal("generic queue editor changed plan task")
	}
}

func TestTaskPlanOwnerPreparesPausesEditsAndStartsOnlyReviewedFrozenTask(t *testing.T) {
	preparations, runs := 0, 0
	var actual RunOptions
	config, store := requestConfig(t, func(_, _, prompt string, options RunOptions) RunResult {
		runs++
		actual = options
		if prompt != "Implement parser" {
			t.Fatal("task redirected")
		}
		return RunResult{WorkID: "run-planned", RevisionID: "rev-planned", FinalText: "Prepared for review"}
	})
	config.PreparePlan = func(_ context.Context, r worksession.Request) (worksession.PlanContent, error) {
		preparations++
		return worksession.PlanContent{Outcome: "A checked parser", Steps: []worksession.PlanStep{{ID: "step-one", Text: "Implement and verify"}}, Effects: []string{"read", "code", "checks"}}, nil
	}
	m := New(config)
	owner := m.conversation
	next, cmd := m.SubmitBridgeUploads(BridgeUploadSubmission{RequestID: "plan-ui-123", Context: "conversation:" + owner, Text: "Implement parser", PlanFirst: true})
	m = next.(Model)
	if !m.running || cmd == nil || runs != 0 || m.planPreparing == "" || len(m.messages) != 0 {
		t.Fatal("preparation ran implementation or duplicated transcript")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.running || runs != 0 || preparations != 1 || m.requests.Requests[0].Plan.Phase != "review" {
		t.Fatal("preparation did not pause")
	}
	r := m.requests.Requests[0]
	action := TaskPlanAction{ConversationID: owner, TaskID: r.ID, Revision: r.Plan.Revision, Digest: r.Plan.Digest, Action: "inspect", RequestID: "inspect-1"}
	next, _ = m.ApplyTaskPlan(action)
	m = next.(Model)
	if m.planResult.Review == nil || m.planResult.Review.Task != "Implement parser" {
		t.Fatal("exact review unavailable")
	}
	edited := *m.planResult.Review.Content
	edited.Outcome = "A bounded, checked parser"
	action.Action = "edit"
	action.Content = &edited
	next, _ = m.ApplyTaskPlan(action)
	m = next.(Model)
	if m.planResult.Error != "" || m.requests.Requests[0].Plan.Revision != 2 {
		t.Fatal("edit not retained", m.planResult.Error)
	}
	action.Action = "approve"
	action.Content = nil
	next, _ = m.ApplyTaskPlan(action)
	m = next.(Model)
	if m.planResult.Error == "" || runs != 0 {
		t.Fatal("stale approval started work")
	}
	r = m.requests.Requests[0]
	action.Revision = r.Plan.Revision
	action.Digest = r.Plan.Digest
	m.options.MaxSteps = 99 // changes to future requests cannot rewrite this task.
	next, cmd = m.ApplyTaskPlan(action)
	m = next.(Model)
	if m.planResult.Error != "" || !m.running || cmd == nil {
		t.Fatal("current approval did not claim work", m.planResult.Error)
	}
	if m.bridgeGuidanceID() != "" {
		t.Fatal("planned task advertised unreviewed steering")
	}
	_, duplicate := m.ApplyTaskPlan(action)
	if duplicate != nil {
		t.Fatal("concurrent approve produced a second command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if runs != 1 || actual.MaxSteps == 99 || actual.TaskPlanRequestID != r.ID || !actual.PlanFirst {
		t.Fatal("frozen task or single dispatch lost", runs, actual.MaxSteps)
	}
	next, duplicate = m.ApplyTaskPlan(action)
	m = next.(Model)
	if duplicate != nil || m.planResult.Error != "" || runs != 1 {
		t.Fatal("completed approval replayed")
	}
	journal, err := store.LoadRequests(owner)
	if err != nil || journal.Requests[0].State != "completed" || journal.Requests[0].Plan.Phase != "finished" {
		t.Fatal("completion receipt lost", err)
	}
}
func TestTaskPlanOwnerCancellationAndStalePreparationCannotRun(t *testing.T) {
	config, _ := requestConfig(t, func(_, _, _ string, _ RunOptions) RunResult {
		t.Fatal("cancelled preparation ran work")
		return RunResult{}
	})
	config.PreparePlan = func(context.Context, worksession.Request) (worksession.PlanContent, error) {
		return worksession.PlanContent{Outcome: "Not to be accepted", Steps: []worksession.PlanStep{{ID: "step-one", Text: "Proposed"}}, Effects: []string{"read"}}, nil
	}
	m := New(config)
	next, cmd := m.SubmitBridgeUploads(BridgeUploadSubmission{RequestID: "plan-cancel", Context: "conversation:" + m.conversation, Text: "Prepare a task", PlanFirst: true})
	m = next.(Model)
	m.cancel()
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.running || m.requests.Requests[0].State != "interrupted" || m.requests.Requests[0].Plan.Content != nil || len(m.queue) != 0 {
		t.Fatal("cancelled proposal retained authority")
	}
	next, _ = m.Update(taskPlanPrepared{ID: "old-owner", Content: worksession.PlanContent{Outcome: "Wrong task"}})
	after := next.(Model)
	if after.requests.Requests[0].Plan.Content != nil {
		t.Fatal("stale preparation overwrote current task")
	}
}

func TestTaskPlanQueuedPreparationFreezesOptionsAndPausesFollowingWork(t *testing.T) {
	runs, preparations := 0, 0
	config, store := requestConfig(t, func(_, _, _ string, options RunOptions) RunResult {
		runs++
		if runs == 2 && (!options.PlanFirst || options.MaxSteps != 12) {
			t.Fatal("review lost frozen options")
		}
		return RunResult{WorkID: "fixture-work", FinalText: "Actual result"}
	})
	config.PreparePlan = func(_ context.Context, r worksession.Request) (worksession.PlanContent, error) {
		preparations++
		var frozen RunOptions
		if json.Unmarshal(r.Options, &frozen) != nil || frozen.MaxSteps != 12 || !frozen.PlanFirst {
			t.Fatal("queued options changed")
		}
		return worksession.PlanContent{Outcome: "A reviewed follow-up", Steps: []worksession.PlanStep{{ID: "first", Text: "Read and explain"}}, Effects: []string{"read"}}, nil
	}
	m := New(config)
	next, first := m.SubmitText("First task")
	m = next.(Model)
	m.options.MaxSteps = 12
	request := BridgeUploadSubmission{RequestID: "queued-plan", Context: "conversation:" + m.conversation, Text: "Plan the follow-up", PlanFirst: true}
	next, _ = m.SubmitBridgeUploads(request)
	m = next.(Model)
	if len(m.queue) != 1 || !m.queue[0].options.PlanFirst || m.options.PlanFirst {
		t.Fatal("per-request plan flag escaped its admission")
	}
	m.options.MaxSteps = 88
	next, _ = m.SubmitText("Do not run before review")
	m = next.(Model)
	next, prepare := m.Update(first())
	m = next.(Model)
	if runs != 1 || preparations != 0 || prepare == nil || m.planPreparing == "" {
		t.Fatal("queued plan was skipped or auto-executed")
	}
	next, auto := m.Update(prepare())
	m = next.(Model)
	if auto != nil || m.running || preparations != 1 || runs != 1 || len(m.queue) != 1 {
		t.Fatal("plan review did not pause subsequent work")
	}
	var planned worksession.Request
	journal, err := store.LoadRequests(m.conversation)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range journal.Requests {
		if r.Plan != nil {
			planned = r
		}
	}
	if planned.Plan == nil || planned.Plan.Budget.MaxSteps != 12 || planned.Plan.Phase != "review" {
		t.Fatal("queued review lost")
	}
	next, _ = m.SubmitBridgeUploads(request)
	m = next.(Model)
	if m.bridgeUploadResult.Error != "" || preparations != 1 || runs != 1 || len(m.requests.Requests) != 3 {
		t.Fatal("transport retry duplicated admission")
	}
	next, execute := m.ApplyTaskPlan(TaskPlanAction{ConversationID: m.conversation, TaskID: planned.ID, Revision: planned.Plan.Revision, Digest: planned.Plan.Digest, Action: "approve", RequestID: "queued-approve"})
	m = next.(Model)
	if execute == nil || m.planResult.Error != "" {
		t.Fatal("queued task could not start after exact review", m.planResult.Error)
	}
	next, _ = m.Update(execute())
	m = next.(Model)
	if runs != 2 || preparations != 1 {
		t.Fatal("reviewed queued task did not execute exactly once")
	}
}

func TestTaskPlanRestartRetainsReviewWithoutReplayAndRejectsWrongOwner(t *testing.T) {
	runs := 0
	config, store := requestConfig(t, func(_, _, _ string, _ RunOptions) RunResult { runs++; return RunResult{FinalText: "Done"} })
	config.PreparePlan = func(context.Context, worksession.Request) (worksession.PlanContent, error) {
		return worksession.PlanContent{Outcome: "Read only", Steps: []worksession.PlanStep{{ID: "one", Text: "Read the input"}}, Effects: []string{"read"}}, nil
	}
	m := New(config)
	next, prepare := m.SubmitBridgeUploads(BridgeUploadSubmission{RequestID: "restart-plan", Context: "conversation:" + m.conversation, Text: "Review this task", PlanFirst: true})
	m = next.(Model)
	next, _ = m.Update(prepare())
	m = next.(Model)
	r := m.requests.Requests[0]
	config.StartConversationID = m.conversation
	restored := New(config)
	if len(restored.queue) != 0 || restored.running || runs != 0 || restored.requests.Requests[0].Plan.Digest != r.Plan.Digest {
		t.Fatal("restart replayed or changed review")
	}
	action := TaskPlanAction{ConversationID: m.conversation, TaskID: r.ID, Revision: r.Plan.Revision, Digest: r.Plan.Digest, RequestID: "owner-approve", Action: "approve"}
	wrong, err := store.CreateChat(config.CurrentFolder, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	wrongAction := action
	wrongAction.ConversationID = wrong.ID
	next, command := restored.ApplyTaskPlan(wrongAction)
	if command != nil || next.(Model).planResult.Error == "" || runs != 0 {
		t.Fatal("cross-chat plan authority accepted")
	}
	_, _, err = store.ApproveTaskPlan(m.conversation, r.ID, r.Plan.Revision, r.Plan.Digest)
	if err != nil {
		t.Fatal(err)
	}
	restored = New(config)
	if restored.running || len(restored.queue) != 0 || runs != 0 {
		t.Fatal("retained approval auto-started")
	}
	// An approved but undispatched record needs a fresh explicit start.
	action.Action = "start"
	next, command = restored.ApplyTaskPlan(action)
	restored = next.(Model)
	if command == nil || restored.planResult.Error != "" {
		t.Fatal("explicit retained start unavailable", restored.planResult.Error)
	}
	_, _ = restored.Update(command())
	if runs != 1 {
		t.Fatal("explicit start did not execute once")
	}
}
