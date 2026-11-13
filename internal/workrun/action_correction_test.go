package workrun

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/connector"
)

func correctionOperation() *Operation {
	return &Operation{steering: make(chan string, 1), stopped: make(chan struct{}), pending: map[int]pendingInteraction{1: {digest: "exact", interaction: Interaction{ID: 1, Kind: "action"}, response: make(chan bool, 1)}}}
}

func TestActionCorrectionRejectsChangedAuthorityWithoutRetiringReview(t *testing.T) {
	for _, variant := range []string{"digest", "missing", "kind", "multiple", "planned", "stopped", "full", "nil queue", "invalid"} {
		t.Run(variant, func(t *testing.T) {
			operation := correctionOperation()
			id, digest, instruction := 1, "exact", "Use the corrected subject"
			switch variant {
			case "digest":
				digest = "old"
			case "missing":
				id = 2
			case "kind":
				p := operation.pending[1]
				p.interaction.Kind = "command"
				operation.pending[1] = p
			case "multiple":
				operation.pending[2] = pendingInteraction{}
			case "planned":
				operation.planBound = true
			case "stopped":
				stopped := make(chan struct{})
				close(stopped)
				operation.stopped = stopped
			case "full":
				operation.steering <- "prior guidance"
			case "nil queue":
				operation.steering = nil
			case "invalid":
				instruction = "\x00"
			}
			pending := operation.pending[1]
			if err := operation.CorrectAction(id, digest, instruction); err == nil {
				t.Fatal("unsafe correction accepted")
			}
			if _, ok := operation.pending[1]; !ok || len(pending.response) != 0 {
				t.Fatal("failed correction retired original review")
			}
		})
	}
}

func TestActionCorrectionAndApprovalHaveOneWinner(t *testing.T) {
	for index := 0; index < 100; index++ {
		operation := correctionOperation()
		pending := operation.pending[1]
		start := make(chan struct{})
		result := make(chan error, 2)
		go func() { <-start; result <- operation.CorrectAction(1, "exact", "Correct it") }()
		go func() { <-start; result <- operation.respondBound(1, "exact", true) }()
		close(start)
		first, second := <-result, <-result
		if (first == nil) == (second == nil) {
			t.Fatal("approval and correction did not have exactly one winner")
		}
		approved := <-pending.response
		if approved && len(operation.steering) != 0 || !approved && len(operation.steering) != 1 {
			t.Fatal("correction and approval were partially applied")
		}
	}
}

func TestServiceActionCorrectionRetainsRunAndRequiresFreshExactApproval(t *testing.T) {
	var effects atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		effects.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"corrected-send"}`)
	}))
	defer server.Close()
	registry, err := connector.NewRegistry([]connector.Descriptor{{Version: 1, ID: "google", Name: "Local test", Kind: connector.KindGoogle, Resource: server.URL, Authentication: connector.AuthNone}})
	if err != nil {
		t.Fatal(err)
	}
	send := func(id, subject string) agent.Turn {
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: id, Name: "connector_google_gmail_send", Arguments: json.RawMessage(fmt.Sprintf(`{"payload":{"from":"me@example.com","to":"venue@example.com","subject":%q,"body":"Exact body"}}`, subject))}}}
	}
	model := &scriptedModel{turns: []agent.Turn{send("original", "Original subject"), send("replacement", "Corrected subject"), {Text: "Inspect the retained send receipt."}}}
	state := t.TempDir()
	contract := artifact.InspectionContract()
	contract.ExternalActions = action.Approve
	operation := (Service{Executor: Executor{Model: model, StateDir: state, Connectors: connector.Runtime{Registry: registry, HTTPClient: server.Client()}}}).Start(context.Background(), Request{RunID: "correction-original-run", SourcePath: t.TempDir(), Objective: "Send the venue update", Mode: action.Act, Contract: contract, ConnectorIDs: []string{"google"}, DisableBrowser: true, DisableDelegation: true, MaxSteps: 5})
	defer operation.Cancel()
	nextReview := func() Interaction {
		t.Helper()
		select {
		case review := <-operation.Interactions:
			return review
		case result := <-operation.Done:
			t.Fatalf("ended before review: %v", result.Err)
		case <-time.After(10 * time.Second):
			t.Fatal("review timed out")
		}
		return Interaction{}
	}
	first := nextReview()
	digest, _ := InteractionDigest(first)
	rollback, err := operation.FreezeForReview()
	if err != nil {
		t.Fatal(err)
	}
	if operation.CanCorrectAction(first.ID, digest) {
		t.Fatal("fenced action still advertised corrections")
	}
	if err := operation.respondBound(first.ID, digest, true); err == nil {
		t.Fatal("scope fence released an old approval")
	}
	if err := operation.Steer("new instruction"); err == nil {
		t.Fatal("guidance bypassed scope fence")
	}
	if effects.Load() != 0 {
		t.Fatal("fenced review dispatched an effect")
	}
	rollback()
	laterRollback, err := operation.FreezeForReview()
	if err != nil {
		t.Fatal(err)
	}
	rollback()
	if err := operation.respondBound(first.ID, digest, true); err == nil {
		t.Fatal("replayed rollback removed a later scope fence")
	}
	laterRollback()
	if !operation.CanCorrectAction(first.ID, digest) || effects.Load() != 0 {
		t.Fatal("action not held for review")
	}
	if err := operation.CorrectAction(first.ID, digest, "Use Corrected subject instead. Preserve the recipient and body."); err != nil {
		t.Fatal(err)
	}
	replacement := nextReview()
	freshDigest, _ := InteractionDigest(replacement)
	if replacement.ID == first.ID || freshDigest == digest || effects.Load() != 0 || operation.RunID != "correction-original-run" {
		t.Fatal("replacement lost exact review or task identity")
	}
	if err := operation.respondBound(first.ID, digest, true); err == nil {
		t.Fatal("withdrawn review executed")
	}
	if err := operation.respondBound(replacement.ID, digest, true); err == nil {
		t.Fatal("old preview approved replacement")
	}
	if effects.Load() != 0 {
		t.Fatal("sent without fresh review")
	}
	if err := operation.respondBound(replacement.ID, freshDigest, true); err != nil {
		t.Fatal(err)
	}
	for range operation.Events {
	}
	result := <-operation.Done
	if result.Err != nil || effects.Load() != 1 {
		t.Fatalf("fresh approval outcome: effects=%d err=%v", effects.Load(), result.Err)
	}
	if len(result.Outcome.Manifest.Actions) != 2 || result.Outcome.Manifest.Actions[0].Status != action.Denied || result.Outcome.Manifest.Actions[1].Status != action.Executed {
		t.Fatalf("original and replacement receipts: %+v", result.Outcome.Manifest.Actions)
	}
	if len(model.requests) != 3 {
		t.Fatal("unexpected execution restart")
	}
	data, _ := json.Marshal(model.requests[1].Messages)
	if !strings.Contains(string(data), "Use Corrected subject instead") {
		t.Fatal("correction did not reach original model execution")
	}
	records, err := ReadInteractionRecords(state, operation.RunID)
	decisions := map[int]bool{}
	for _, record := range records {
		if record.Approved != nil {
			decisions[record.Interaction.ID] = *record.Approved
		}
	}
	if err != nil || len(records) != 2 || len(decisions) != 2 || decisions[first.ID] || !decisions[replacement.ID] {
		t.Fatalf("durable exact decisions: %+v %v", records, err)
	}
}

func TestConversationActionCorrectionIsIntentWithoutReviewAuthority(t *testing.T) {
	request := ConversationRequest{Text: "Change the subject", Tasks: []ConversationTask{{ID: "original", Title: "Send update", State: "review", CanCorrectAction: true, ReviewControlID: "private-review-epoch"}}}
	backend := &conversationModel{turn: agent.Turn{Text: `{"kind":"revise_action","target":"original","instruction":"Use Corrected subject"}`}}
	if _, err := RouteConversation(context.Background(), backend, request); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(backend.request.Messages[0].Content, "private-review-epoch") {
		t.Fatal("review authority leaked to router")
	}
	for _, variant := range []string{"files", "plan", "capability", "acknowledgement", "target"} {
		copy := request
		copy.Tasks = append([]ConversationTask(nil), request.Tasks...)
		decision := ConversationDecision{Kind: "revise_action", Target: "original", Instruction: "Use Corrected subject"}
		switch variant {
		case "files":
			copy.HasFiles = true
		case "plan":
			copy.PlanFirst = true
		case "capability":
			copy.Tasks[0].CanCorrectAction = false
		case "acknowledgement":
			decision.Text = "Done"
		case "target":
			decision.Target = "other"
		}
		if decision.Validate(copy) == nil {
			t.Fatalf("accepted %s", variant)
		}
	}
}
