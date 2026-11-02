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
