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
			// Service records a refusal as a resolved interaction before durable
			// mutation admission. Preserve that convention; no action attempt or
			// fabricated dispatch receipt is required for an unadmitted denial.
			wantReceipts := 1
			if !scenario.allow {
				wantReceipts = 0
			}
			if dispatched.Load() != scenario.calls || len(result.Outcome.Manifest.Actions) != wantReceipts || wantReceipts > 0 && result.Outcome.Manifest.Actions[0].Status != scenario.want {
				t.Fatal("exact review or uncertainty was lost")
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
