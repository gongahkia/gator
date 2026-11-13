package workapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/worksession"
)

func TestPlanSetupOutcomeIsRetainedButLateCancelledOutcomeCannotReviveTask(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "setup", true: "cancelled"}[cancel], func(t *testing.T) {
			config, store := requestConfig(t, func(_, _, _ string, _ RunOptions) RunResult { t.Fatal("setup ran Work"); return RunResult{} })
			config.PreparePlan = func(_ context.Context, r worksession.Request) (worksession.PlanContent, error) {
				c, err := store.Load(r.Plan.Binding.ConversationID)
				if err != nil {
					t.Fatal(err)
				}
				b, err := store.PlanningPreparationBoundary(c.ID, r.ID, r.Text, c.SourcePath, r.Options)
				if err != nil {
					t.Fatal(err)
				}
				return worksession.PlanContent{}, &worksession.PreparationError{Issue: "Selected source is no longer configured", Recovery: worksession.RequestRecovery{Preparation: &b, Kind: "source_access", RetrySafe: true, Needs: []worksession.TaskAccessNeed{{ConnectorID: "reports", Kind: "unconfigured", Operation: "configure"}}, ScopeDigest: strings.Repeat("a", 64), ProcessingDigest: strings.Repeat("b", 64), Summary: "First proposal needs setup", Repair: "Review required source"}}
			}
			m := New(config)
			next, command := m.SubmitBridgeUploads(BridgeUploadSubmission{RequestID: "setup-plan-request", Context: "conversation:" + m.conversation, Text: "Read the original report", PlanFirst: true})
			m = next.(Model)
			if command == nil {
				t.Fatal("no initial preparation")
			}
			result := command()
			j, err := store.LoadRequests(m.conversation)
			if err != nil {
				t.Fatal(err)
			}
			original := j.Requests[0]
			if cancel {
				next, _ = m.ApplyTaskPlan(TaskPlanAction{ConversationID: m.conversation, TaskID: original.ID, Revision: original.Plan.Revision, Digest: original.Plan.Digest, Action: "cancel", RequestID: "cancel-original-setup"})
				m = next.(Model)
				if m.planResult.Error != "" {
					t.Fatal(m.planResult.Error)
				}
			}
			next, _ = m.Update(result)
			m = next.(Model)
			j, err = store.LoadRequests(m.conversation)
			if err != nil {
				t.Fatal(err)
			}
			actual := j.Requests[0]
			if actual.ID != original.ID || actual.Text != original.Text || string(actual.Options) != string(original.Options) || len(actual.Plan.Attempts) > 0 || actual.Plan.Approval != nil {
				t.Fatal("setup changed task scope or acquired authority")
			}
			if cancel {
				if actual.State != "discarded" || actual.Recovery != nil || len(m.requestRecoveryViews()) > 0 {
					t.Fatal("late setup outcome revived cancelled plan", actual.State)
				}
			} else {
				if actual.State != "failed" || actual.Recovery == nil || actual.Recovery.Preparation == nil || len(m.requestRecoveryViews()) != 1 {
					raw, _ := json.Marshal(actual)
					t.Fatal("lost actionable setup outcome", string(raw))
				}
			}
		})
	}
}
