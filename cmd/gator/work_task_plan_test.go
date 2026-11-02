package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
