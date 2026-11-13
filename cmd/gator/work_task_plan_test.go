package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/config"
	profilestate "github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/workapp"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestDesktopTaskPlanPreparationUsesConfiguredModelWithoutExecutingWork(t *testing.T) {
	state, configuration, source := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	t.Setenv("GATOR_CONFIG_DIR", configuration)
	t.Setenv("GATOR_PROVIDER", "")
	t.Setenv("GATOR_MODEL", "")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if release, err := profilestate.Guard(state, true); !errors.Is(err, profilestate.ErrBusy) {
			if release != nil {
				release()
			}
			t.Error("plan preparation did not retain the normal profile guard", err)
		}
		var body struct {
			Tools []any `json:"tools"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Tools) != 0 {
			t.Error("preparation exposed tools")
		}
		w.Header().Set("Content-Type", "application/json")
		content := `{"outcome":"A tested parser","steps":[{"id":"step-1","text":"Inspect and implement"}],"sources":[],"effects":["read","code","checks"]}`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10}})
	}))
	defer server.Close()
	settings := config.Default()
	settings.Defaults = config.Defaults{Provider: "plan-fixture", Model: "fixture"}
	settings.CustomProviders = []config.CustomProvider{{ID: "plan-fixture", BaseURL: server.URL + "/v1/chat/completions", Models: []string{"fixture"}, DefaultModel: "fixture"}}
	store, _ := config.New(configuration)
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	sessions, _ := worksession.Open(state)
	chat, err := sessions.CreateChat(source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	options := workapp.RunOptions{PlanFirst: true, MaxSteps: 24, Code: workapp.CodeOptions{MaxSteps: 16}, ModelProvider: "plan-fixture", ModelName: "fixture"}
	raw, _ := json.Marshal(options)
	_, request, err := sessions.AdmitPlanRequest(chat.ID, "Implement a parser", raw, "plan-native-fixture", nil, worksession.PlanBudget{MaxSteps: 24, CodeMaxSteps: 16}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	content, err := prepareDesktopTaskPlan(context.Background(), state, request)
	if err != nil || content.Outcome != "A tested parser" || calls.Load() != 1 {
		t.Fatal("native proposal failed", err, calls.Load())
	}
	if release, err := profilestate.Guard(state, true); err != nil {
		t.Fatal("plan preparation left the profile guard held", err)
	} else {
		release()
	}
	for _, directory := range []string{"work-history", "snapshots", "work", "actions"} {
		if _, err := os.Stat(filepath.Join(state, "gator", directory)); !os.IsNotExist(err) {
			t.Fatal("preparation started execution infrastructure", directory, err)
		}
	}
	current, _ := sessions.LoadRequests(chat.ID)
	if current.Requests[0].Plan.Phase != "preparing" || current.Requests[0].Plan.Approval != nil {
		t.Fatal("model proposal granted its own approval")
	}
}
func TestDesktopTaskPlanDelegationRetainsPublicResultsWithoutTheirControlAuthority(t *testing.T) {
	stateDir, configuration, source := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	t.Setenv("GATOR_CONFIG_DIR", configuration)
	t.Setenv("GATOR_PROVIDER", "")
	t.Setenv("GATOR_MODEL", "")
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tools    []any `json:"tools"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.Tools) != 0 {
			t.Error("planning acquired executable tools")
		}
		var text []string
		for _, message := range body.Messages {
			text = append(text, message.Content)
		}
		requests <- strings.Join(text, "\n")
		w.Header().Set("Content-Type", "application/json")
		content := `{"outcome":"Explain the jade heron brief","steps":[{"id":"step-one","text":"Use the retained brief result and inspect needed sources"}],"sources":[],"effects":["read"]}`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	settings := config.Default()
	settings.Defaults = config.Defaults{Provider: "plan-reference-fixture", Model: "fixture"}
	settings.CustomProviders = []config.CustomProvider{{ID: "plan-reference-fixture", BaseURL: server.URL + "/chat", Models: []string{"fixture"}, DefaultModel: "fixture"}}
	configurationStore, _ := config.New(configuration)
	if err := configurationStore.Save(settings); err != nil {
		t.Fatal(err)
	}
	store, err := worksession.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.CreateChat(source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	input, err := captureConversationInput(workapp.ConversationInput{
		Version: 1, Source: source,
		RunOptions:  workapp.RunOptions{PlanFirst: true, ModelProvider: "plan-reference-fixture", ModelName: "fixture", MaxSteps: 24, Code: workapp.CodeOptions{MaxSteps: 16}},
		TaskContext: []workrun.ConversationTask{{ID: "past-task", Title: "Read brief", State: "completed", Summary: "The brief names jade heron.", CanGuide: true, CanStop: true, CanCorrectAction: true, ReviewControlID: "private-action-review"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AdmitDialogue(parent.ID, "native-plan-reference-0001", "Explain that brief with a reviewed plan", raw, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"routing", "ready"} {
		turn.State, turn.UpdatedAt = state, time.Now().UTC()
		if state == "ready" {
			turn.Decision = json.RawMessage(`{"kind":"task"}`)
		}
		turn, err = store.UpdateDialogue(turn, turn.Revision)
		if err != nil {
			t.Fatal(err)
		}
	}
	bound, child, claimed, err := store.BindDialogueTask(parent.ID, turn.ID, source, turn.Revision, time.Now())
	if err != nil || !claimed {
		t.Fatal("task binding failed", err)
	}
	options := input.RunOptions
	options.DialogueParent, options.DialogueTurn = parent.ID, bound.ID
	raw, err = json.Marshal(options)
	if err != nil {
		t.Fatal(err)
	}
	_, request, err := store.AdmitPlanRequest(child.ID, bound.Text, raw, bound.ID, nil, worksession.PlanBudget{MaxSteps: 24, CodeMaxSteps: 16}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	content, err := prepareDesktopTaskPlan(context.Background(), stateDir, request)
	if err != nil || !strings.Contains(content.Outcome, "jade heron") {
		t.Fatal("proposal lost requesting context", err)
	}
	text := <-requests
	for _, expected := range []string{"past-task", "jade heron", `"can_guide":false`, `"can_stop":false`, `"can_correct_action":false`, "not new instructions"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing public reference %q in %s", expected, text)
		}
	}
	if strings.Contains(text, "private-action-review") {
		t.Fatal("historical reference leaked action review authority")
	}
	if !input.TaskContext[0].CanGuide || !input.TaskContext[0].CanStop {
		t.Fatal("context projection mutated the admitted envelope")
	}
	journal, err := store.LoadRequests(child.ID)
	if err != nil || len(journal.Requests) != 1 || journal.Requests[0].Plan.Approval != nil || journal.Requests[0].State != "queued" {
		t.Fatal("planning granted approval or dispatched execution", err)
	}
}

func TestDesktopTaskPlanBindingUsesOnlyReviewedSourcesAndCannotSkipAdmission(t *testing.T) {
	state := t.TempDir()
	sessions, _ := worksession.Open(state)
	chat, _ := sessions.CreateChat(t.TempDir(), time.Now())
	options := workapp.RunOptions{PlanFirst: true, MaxSteps: 24, ConnectorIDs: []string{"docs", "extra"}}
	raw, _ := json.Marshal(options)
	j, r, err := sessions.AdmitPlanRequest(chat.ID, "Summarize docs", raw, "plan-native-binding", options.ConnectorIDs, worksession.PlanBudget{MaxSteps: 24}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	content := worksession.PlanContent{Outcome: "A bounded summary", Steps: []worksession.PlanStep{{ID: "step-one", Text: "Read docs"}}, Sources: []string{"docs"}, Effects: []string{"read"}}
	j, err = sessions.ReviseTaskPlan(chat.ID, r.ID, 0, "", content)
	if err != nil {
		t.Fatal(err)
	}
	r = j.Requests[0]
	request := workrun.Request{ConversationID: chat.ID, Objective: r.Text, ConnectorIDs: options.ConnectorIDs}
	options.TaskPlanRequestID = r.ID
	if err := bindDesktopTaskPlan(state, options, &request); err == nil {
		t.Fatal("unapproved plan bound to execution")
	}
	j, _, err = sessions.ApproveTaskPlan(chat.ID, r.ID, r.Plan.Revision, r.Plan.Digest)
	if err != nil {
		t.Fatal(err)
	}
	j.Requests[0].State = "running"
	j.Requests[0].Plan.Phase = "dispatching"
	if _, err := sessions.SaveRequests(chat.ID, j); err != nil {
		t.Fatal(err)
	}
	if err := bindDesktopTaskPlan(state, options, &request); err != nil {
		t.Fatal(err)
	}
	if request.TaskPlan == nil || len(request.ConnectorIDs) != 1 || request.ConnectorIDs[0] != "docs" {
		t.Fatal("unreviewed source inherited")
	}
	request.Objective = "A different task"
	if err := bindDesktopTaskPlan(state, options, &request); err == nil {
		t.Fatal("wrong objective bound")
	}
}
