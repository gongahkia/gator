package workapp

import (
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
