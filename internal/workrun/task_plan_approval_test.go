package workrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/actionlog"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/tools"
)

// These exercise real Service/Work ownership, retained decisions, plan scope
// and receipts. Models and the coding delegate are controlled test adapters;
// the external service is a private loopback fixture, never a real account.
func TestTaskPlanApprovalStillRequiresExactConnectedActionReview(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		allow  bool
		status int
		want   action.Status
		calls  int32
	}{
		{"deny", false, http.StatusNoContent, action.Denied, 0},
		{"allow", true, http.StatusNoContent, action.Executed, 1},
		{"unknown", true, http.StatusInternalServerError, action.Unknown, 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			state, _, request := claimedPlanFixture(t, []string{"read", "connected_write"}, "release")
			request.RunID = "plan-connected-" + scenario.name
			request.Mode = action.Act
			request.Contract = artifact.InspectionContract()
			request.Contract.ExternalActions = action.Approve
			var dispatched atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				dispatched.Add(1)
				w.WriteHeader(scenario.status)
			}))
			defer server.Close()
			registry, err := connector.NewRegistry([]connector.Descriptor{{Version: connector.DescriptorVersion, ID: "release", Name: "Release fixture", Kind: connector.KindHTTPWebhook, Resource: server.URL, Authentication: connector.AuthNone}})
			if err != nil {
				t.Fatal(err)
			}
			model := &scriptedModel{turns: []agent.Turn{
				{ToolCalls: []agent.ToolCall{{ID: "release-once", Name: "connector_release_publish", Arguments: json.RawMessage(`{"payload":{"version":"v1"}}`)}}},
				{Text: "The exact request has a retained receipt; inspect its actual state."},
			}}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			operation := (Service{Executor: Executor{Model: model, StateDir: state, Connectors: connector.Runtime{Registry: registry, HTTPClient: server.Client()}}}).Start(ctx, request)
			defer operation.Cancel()
			var interaction Interaction
			select {
			case interaction = <-operation.Interactions:
			case <-ctx.Done():
				t.Fatal("exact action review did not become pending")
			}
			proposal, ok := interaction.Preview.(action.Proposal)
			payload := sha256.Sum256([]byte(`{"version":"v1"}`))
			if interaction.Kind != "action" || !ok || proposal.ConnectorID != "release" || proposal.Target != server.URL || proposal.PayloadSHA256 != hex.EncodeToString(payload[:]) {
				t.Fatal("review did not bind the actual connected request")
			}
			if dispatched.Load() != 0 {
				t.Fatal("plan approval dispatched the connected mutation")
			}
			if err := operation.respondBound(interaction.ID, strings.Repeat("0", 64), true); err == nil {
				t.Fatal("mismatched exact preview accepted")
			}
			if dispatched.Load() != 0 {
				t.Fatal("mismatched preview dispatched a mutation")
			}
			digest, err := InteractionDigest(interaction)
			if err != nil {
				t.Fatal(err)
			}
			if err := operation.respondBound(interaction.ID, digest, scenario.allow); err != nil {
				t.Fatal(err)
			}
			for range operation.Events {
			}
			result := <-operation.Done
			if scenario.want == action.Executed && result.Err != nil {
				t.Fatal(result.Err)
			}
			// Refusal is a resolved domain decision, retained alongside its exact
			// interaction. A denied proposal has no fabricated provider receipt.
			if dispatched.Load() != scenario.calls || len(result.Outcome.Manifest.Actions) != 1 || result.Outcome.Manifest.Actions[0].Status != scenario.want {
				t.Fatal("exact review or uncertainty was lost")
			}
			if !scenario.allow && result.Outcome.Manifest.Actions[0].ReceiptID != "" {
				t.Fatal("denied proposal acquired a dispatch receipt")
			}
			if scenario.want == action.Unknown && result.Err == nil {
				t.Fatal("unknown action was treated as completed Work")
			}
			if err := operation.respondBound(interaction.ID, digest, true); err == nil {
				t.Fatal("consumed review accepted twice")
			}
			records, err := ReadInteractionRecords(state, request.RunID)
			if err != nil || len(records) != 1 || records[0].Status != "resolved" || records[0].Approved == nil || *records[0].Approved != scenario.allow {
				t.Fatal("the actual exact decision was not retained", err)
			}
			journal, err := actionlog.Open(state)
			if err != nil {
				t.Fatal(err)
			}
			// The manifest records the denied proposal, while the dispatch journal
			// contains only admitted external attempts. Rejection admits none.
			wantReceipts := 1
			if !scenario.allow {
				wantReceipts = 0
			}
			for range 3 {
				entries, err := journal.List(request.RunID)
				if err != nil || len(entries) != wantReceipts || wantReceipts > 0 && (entries[0].Live || entries[0].Action.Status != scenario.want) {
					t.Fatal("retained exact action receipt changed", err)
				}
			}
			if dispatched.Load() != scenario.calls {
				t.Fatal("receipt inspection replayed the mutation")
			}
		})
	}
}

func TestTaskPlanApprovalStillRequiresExactCodingCommandReview(t *testing.T) {
	for _, allow := range []bool{false, true} {
		name := "deny"
		if allow {
			name = "allow"
		}
		t.Run(name, func(t *testing.T) {
			state, _, request := claimedPlanFixture(t, []string{"read", "code", "checks", "draft"})
			request.RunID = "plan-command-" + name
			request.EnableCode = true
			var executed atomic.Int32
			argv := []string{"reviewed-fixture-command", "exact-argument"}
			model := &scriptedModel{turns: []agent.Turn{
				{ToolCalls: []agent.ToolCall{{ID: "code", Name: "delegate_agents", Arguments: json.RawMessage(`{"tasks":[{"agent":"code","task":"Perform the bounded fixture check"}]}`)}}},
				{ToolCalls: []agent.ToolCall{{ID: "report", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"brief.md","content":"The fixture command has an observed review result."}`)}}},
				{Text: "Prepared brief.md for normal artifact review."},
			}}
			service := Service{Executor: Executor{Model: model, StateDir: state, Code: func(ctx context.Context, code CodeRequest) (CodeResult, error) {
				decision, err := code.Approve(ctx, argv)
				if err != nil {
					return CodeResult{}, err
				}
				if decision != tools.CommandAllowOnce {
					return CodeResult{}, errors.New("fixture command denied")
				}
				executed.Add(1)
				return CodeResult{Summary: "One explicitly reviewed fixture command observed"}, nil
			}}}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			operation := service.Start(ctx, request)
			defer operation.Cancel()
			var interaction Interaction
			select {
			case interaction = <-operation.Interactions:
			case <-ctx.Done():
				t.Fatal("command review did not become pending")
			}
			preview, ok := interaction.Preview.([]string)
			if interaction.Kind != "command" || !ok || !slices.Equal(preview, argv) {
				t.Fatal("command review did not bind exact argv")
			}
			if executed.Load() != 0 {
				t.Fatal("plan approval admitted a coding command")
			}
			if operation.respondBound(interaction.ID, strings.Repeat("f", 64), true) == nil {
				t.Fatal("changed command preview accepted")
			}
			digest, err := InteractionDigest(interaction)
			if err != nil {
				t.Fatal(err)
			}
			if err := operation.respondBound(interaction.ID, digest, allow); err != nil {
				t.Fatal(err)
			}
			for range operation.Events {
			}
			result := <-operation.Done
			want := int32(0)
			if allow {
				want = 1
			}
			if executed.Load() != want {
				t.Fatal("exact command decision was bypassed")
			}
			if allow && (result.Err != nil || result.Outcome.Manifest.Status != artifact.Completed) {
				t.Fatal("reviewed fixture failed normal artifact verification", result.Err)
			}
			if operation.respondBound(interaction.ID, digest, true) == nil {
				t.Fatal("consumed command approval replayed")
			}
			records, err := ReadInteractionRecords(state, request.RunID)
			if err != nil || len(records) != 1 || records[0].Status != "resolved" || records[0].Approved == nil || *records[0].Approved != allow {
				t.Fatal("exact decision result not retained", err)
			}
		})
	}
}

func TestRevokedRunningPlanCannotReleasePreviouslyPendingAction(t *testing.T) {
	state, store, request := claimedPlanFixture(t, []string{"read", "connected_write"}, "release")
	request.RunID = "pausing-plan-run"
	request.Mode = action.Act
	request.Contract = artifact.InspectionContract()
	request.Contract.ExternalActions = action.Approve
	var effects atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { effects.Add(1); w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	registry, err := connector.NewRegistry([]connector.Descriptor{{Version: 1, ID: "release", Name: "Controlled", Kind: connector.KindHTTPWebhook, Resource: server.URL, Authentication: connector.AuthNone}})
	if err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{turns: []agent.Turn{{ToolCalls: []agent.ToolCall{{ID: "pending", Name: "connector_release_publish", Arguments: json.RawMessage(`{"payload":{"version":"v1"}}`)}}}, {Text: "Inspect the retained outcome."}}}
	operation := (Service{Executor: Executor{Model: model, StateDir: state, Connectors: connector.Runtime{Registry: registry, HTTPClient: server.Client()}}}).Start(context.Background(), request)
	defer func() {
		operation.Cancel()
		for range operation.Events {
		}
		<-operation.Done
	}()
	var review Interaction
	select {
	case review = <-operation.Interactions:
	case <-time.After(10 * time.Second):
		t.Fatal("review did not arrive")
	}
	turn, err := store.AdmitDialogue(request.ConversationID, "pause-running-task", "Correct that plan", json.RawMessage(`{"scope":"unchanged"}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"routing", "ready", "editing_plan"} {
		turn.State = phase
		if phase == "editing_plan" {
			turn.Response = "Requesting a revised plan for review…"
		}
		if phase == "ready" {
			turn.Decision, _ = json.Marshal(map[string]string{"kind": "revise_plan", "target": request.TaskPlan.RequestID, "instruction": turn.Text})
		}
		turn, err = store.UpdateDialogue(turn, turn.Revision)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, paused, claimed, err := store.BeginRunningPlanEdit(request.ConversationID, request.TaskPlan.RequestID, request.TaskPlan.Revision, request.TaskPlan.Digest, request.ConversationID, turn.ID, turn.Text, operation.RunID)
	if err != nil || !claimed || paused.Plan.DrainingRun != operation.RunID || paused.Plan.Approval != nil {
		t.Fatal("running dispatch was not fenced", err)
	}
	digest, _ := InteractionDigest(review)
	if err := operation.respondBound(review.ID, digest, true); err != nil {
		t.Fatal(err)
	}
	for range operation.Events {
	}
	completion := <-operation.Done
	if effects.Load() != 0 {
		t.Fatal("old exact approval escaped revoked plan")
	}
	content := *paused.Plan.Content
	if _, err := store.FinishTaskPlanEdit(request.ConversationID, paused.ID, paused.Plan.Revision, paused.Plan.Digest, turn.ID, &content, ""); err == nil {
		t.Fatal("draining run acquired a replacement review before accounting")
	}
	if _, _, err := store.ApproveTaskPlan(request.ConversationID, paused.ID, paused.Plan.Revision, paused.Plan.Digest); err == nil {
		t.Fatal("unaccounted draining run acquired execution approval")
	}
	if completion.Outcome.RevisionID == "" {
		t.Fatal("stopped run did not retain a revision")
	}
	accounted, err := store.RetainDrainedPlanRun(request.ConversationID, paused.ID, paused.Plan.Revision, paused.Plan.Digest, operation.RunID, completion.Outcome.RevisionID)
	if err != nil {
		t.Fatal("could not attach original run accounting", err)
	}
	retained := accounted.Requests[0]
	if retained.Plan.DrainingRun != "" || len(retained.Plan.Attempts) != 1 || retained.Plan.Budget != paused.Plan.Budget || retained.Text != paused.Text {
		t.Fatal("accounting lost frozen task or original budget")
	}
	remaining, limits, err := retained.Plan.RemainingExecutionBudget()
	if err != nil || remaining.MaxSteps != paused.Plan.Budget.MaxSteps-completion.Outcome.Result.Steps || limits.ModelRequests != 256-completion.Outcome.Manifest.Usage.ModelRequests {
		t.Fatal("spent work was restored", err)
	}
	if _, err := store.FinishTaskPlanEdit(request.ConversationID, retained.ID, retained.Plan.Revision, retained.Plan.Digest, turn.ID, &content, ""); err != nil {
		t.Fatal("accounted task could not prepare fresh review", err)
	}

}
