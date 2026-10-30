package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestConnectedGoalWatchReadsLocalScheduleAndMailAndDraftsForReview(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	if err := os.WriteFile(filepath.Join(source, "budget.txt"), []byte("Offsite budget: $8,000. Venue B is within budget.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "schedule.txt"), []byte("User update: offsite decision meeting is 20 October 2026 at 09:00 +08:00.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/gmail/v1/users/me/profile":
			_, _ = writer.Write([]byte(`{"historyId":"100"}`))
		case "/gmail/v1/users/me/messages":
			if request.URL.Query().Get("q") != "label:offsite newer_than:7d" {
				t.Errorf("Gmail watch query = %q", request.URL.Query().Get("q"))
			}
			_, _ = writer.Write([]byte(`{"messages":[{"id":"mail-1"}]}`))
		case "/gmail/v1/users/me/messages/mail-1":
			_, _ = writer.Write([]byte(`{"id":"mail-1","snippet":"Venue A cancelled; Venue B is available on Friday.","payload":{"headers":[{"name":"Subject","value":"Venue change"}]}}`))
		default:
			t.Errorf("unexpected connector path %s", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	settings := config.Default()
	descriptor := connector.Descriptor{Version: connector.DescriptorVersion, ID: "google", Name: "Google", Kind: connector.KindGoogle, Resource: server.URL, Authentication: connector.AuthNone}
	settings.Connectors = []connector.Descriptor{descriptor}
	configStore, err := config.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := configStore.Save(settings); err != nil {
		t.Fatal(err)
	}
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Plan offsite", "Choose a venue and prepare a reply", source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := goals.InitializePlan(record.ID, "Confirm an affordable venue", []string{"Check availability", "Prepare the reply"}); err != nil {
		t.Fatal(err)
	}
	store, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.Save(jobs.Definition{Name: "Offsite follow-up", GoalID: record.ID, GoalWatch: true, Enabled: true, Schedule: "*/5 * * * *", Timezone: "UTC", Missed: "run_once", SourcePath: source, RefreshSnapshot: true, ChangeOnly: true, Objective: "Reconcile changed offsite mail with local schedule and budget; draft a brief for review", Mode: action.Draft, Contract: artifact.DefaultContract("daily-brief.md"), ConnectorIDs: []string{"google"}, WatchSources: []jobs.WatchSource{{ConnectorID: "google", Operation: "gmail_list", Query: "label:offsite newer_than:7d"}}, MaxSteps: 9})
	if err != nil {
		t.Fatal(err)
	}
	model := &workScriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{
			{ID: "file", Name: "read_file", Arguments: json.RawMessage(`{"path":"source/budget.txt"}`)},
			{ID: "mail-list", Name: "connector_google_gmail_list", Arguments: json.RawMessage(`{"query":"label:offsite newer_than:7d","limit":10}`)},
			{ID: "schedule", Name: "read_file", Arguments: json.RawMessage(`{"path":"source/schedule.txt"}`)},
		}},
		{ToolCalls: []agent.ToolCall{{ID: "mail-detail", Name: "connector_google_gmail_get", Arguments: json.RawMessage(`{"resource_id":"mail-1"}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "draft", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"daily-brief.md","content":"# Offsite update\n\nVenue A cancelled; Venue B is available Friday (Gmail mail-1). The decision meeting is on 20 October (user update in schedule.txt:1). Venue B is within the $8,000 local budget (budget.txt:1). Review the venue reply before sending.\n"}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "artifact_status", Arguments: json.RawMessage(`{}`)}}},
		{Text: `{"attention":true,"summary":"Venue A cancelled; review the cited brief and draft a reply for Venue B.","evidence":["google/gmail_list mail-1","source/schedule.txt:1","source/budget.txt:1"]}`},
	}}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		t.Fatal(err)
	}
	service := workrun.Service{Executor: workrun.Executor{Model: model, StateDir: state, Connectors: connector.Runtime{Registry: registry, HTTPClient: server.Client()}}}
	attempt, err := store.Begin(definition, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	observedSources := map[string]bool{}
	result := executeJobContextWith(context.Background(), definition, attempt, state, false, func(ctx context.Context, current jobs.Definition, stateDir string, runIDs ...string) (jobResult, error) {
		return runJobProcessWith(ctx, current, stateDir, func(ctx context.Context, _, _, _ string, request workrun.Request) (workrun.Outcome, error) {
			if !strings.Contains(request.System, "changed-item metadata") || !strings.Contains(request.System, "Gator checks reference integrity") {
				t.Error("connected watch change metadata was not supplied to Work")
			}
			outcome, err := service.Execute(ctx, request)
			for _, source := range outcome.Manifest.ConnectedSources {
				observedSources[source.Operation] = true
			}
			return outcome, err
		}, runIDs...)
	})
	if result.Status != "completed" || !result.WatchAttention || len(result.WatchChanges) < 2 {
		t.Fatalf("connected goal result = %#v", result)
	}
	for _, operation := range []string{"gmail_list", "gmail_get"} {
		if !observedSources[operation] {
			t.Fatalf("Work did not retain %s as connected evidence: %#v", operation, observedSources)
		}
	}
	contents, err := os.ReadFile(filepath.Join(result.BundlePath, "output", "daily-brief.md"))
	if err != nil || !strings.Contains(string(contents), "Venue A cancelled") || !strings.Contains(string(contents), "$8,000") {
		t.Fatalf("connected brief = %q, %v", contents, err)
	}
	if len(model.requests) == 0 || hasDefinition(model.requests[0].Tools, "connector_google_gmail_send") {
		t.Fatal("unattended connected watch exposed Gmail send")
	}
}
