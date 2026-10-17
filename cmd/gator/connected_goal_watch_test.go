package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
)

func TestConnectedJSONGoalWatchSkipsUnchangedPollAfterRestart(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	value := "available"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"venue":"` + value + `"}`))
	}))
	defer server.Close()
	if err := connectorCommandWithIO([]string{"add", "planning", "--url", server.URL}, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Plan offsite", "Decide on a venue", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := goalCommand([]string{"watch", record.ID, "--connector", "planning"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := store.List()
	if err != nil || len(definitions) != 1 || definitions[0].Schedule != "*/5 * * * *" || len(definitions[0].WatchSources) != 1 {
		t.Fatalf("connected watch = %#v, %v", definitions, err)
	}
	now := time.Now().UTC().Truncate(5 * time.Minute).Add(5 * time.Minute)
	calls := 0
	execute := func(_ context.Context, _ jobs.Definition, _ string, _ ...string) (jobResult, error) {
		calls++
		return jobResult{Status: "completed", Summary: "Checked venue", ResultSHA256: "result", WatchAttention: false}, nil
	}
	runDueJobsContextWith(context.Background(), store, state, false, now, execute)
	if calls != 1 {
		t.Fatalf("first poll called model %d times", calls)
	}
	store, err = jobs.Open(state) // supervisor restart
	if err != nil {
		t.Fatal(err)
	}
	runDueJobsContextWith(context.Background(), store, state, false, now.Add(5*time.Minute), execute)
	if calls != 1 {
		t.Fatalf("unchanged remote source called model %d times", calls)
	}
	value = "cancelled"
	runDueJobsContextWith(context.Background(), store, state, false, now.Add(10*time.Minute), execute)
	if calls != 2 {
		t.Fatalf("changed remote source called model %d times", calls)
	}
	history, err := store.History(definitions[0].ID, 0)
	if err != nil || len(history) != 3 || history[0].Status != "completed" || history[1].Status != "unchanged" || history[1].InputSHA256 == "" {
		t.Fatalf("durable connected watch history = %#v, %v", history, err)
	}
}

func TestConnectedGoogleWatchHashesMessageAndEventContent(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	snippet, summary := "Venue available", "Planning meeting"
	moreMail := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/gmail/v1/users/me/messages":
			if moreMail {
				_, _ = writer.Write([]byte(`{"messages":[{"id":"mail-1"}],"nextPageToken":"more"}`))
				return
			}
			_, _ = writer.Write([]byte(`{"messages":[{"id":"mail-1"}]}`))
		case "/gmail/v1/users/me/messages/mail-1":
			_, _ = writer.Write([]byte(`{"id":"mail-1","snippet":"` + snippet + `","payload":{"headers":[{"name":"Subject","value":"Venue"}]}}`))
		case "/calendar/v3/calendars/primary/events":
			_, _ = writer.Write([]byte(`{"items":[{"id":"event-1","summary":"` + summary + `"}]}`))
		default:
			t.Errorf("unexpected connector path %s", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	settings := config.Default()
	settings.Connectors = []connector.Descriptor{{Version: connector.DescriptorVersion, ID: "google-test", Name: "Google test", Kind: connector.KindGoogle, Resource: server.URL, Authentication: connector.AuthNone}}
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
	record, err := goals.Add("Plan offsite", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	definition := jobs.Definition{GoalID: record.ID, GoalWatch: true, ChangeOnly: true, RefreshSnapshot: true, SourcePath: record.SourcePath, ConnectorIDs: []string{"google-test"}, WatchSources: []jobs.WatchSource{{ConnectorID: "google-test", Operation: "gmail_list", Query: "subject:Venue newer_than:7d"}, {ConnectorID: "google-test", Operation: "calendar_events", Days: 14}}, Mode: action.Inspect, Contract: artifact.InspectionContract()}
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	first, err := connectedGoalWatchHash(context.Background(), definition, state, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := connectedGoalWatchHash(context.Background(), definition, state, now.Add(time.Minute))
	if err != nil || second != first {
		t.Fatalf("unchanged Google results changed hash: %s %s %v", first, second, err)
	}
	snippet = "Venue cancelled"
	changed, err := connectedGoalWatchHash(context.Background(), definition, state, now.Add(2*time.Minute))
	if err != nil || changed == first {
		t.Fatalf("changed Gmail message was missed: %s %s %v", first, changed, err)
	}
	summary = "Planning meeting moved"
	changedAgain, err := connectedGoalWatchHash(context.Background(), definition, state, now.Add(3*time.Minute))
	if err != nil || changedAgain == changed {
		t.Fatalf("changed Calendar event was missed: %s %s %v", changed, changedAgain, err)
	}
	moreMail = true
	if _, err := connectedGoalWatchHash(context.Background(), definition, state, now.Add(4*time.Minute)); err == nil || !strings.Contains(err.Error(), "more than 100") {
		t.Fatalf("truncated Gmail poll looked complete: %v", err)
	}
	moreMail = false
	settings.ConnectorPermissions = []connector.PermissionRule{{ConnectorID: "google-test", Operation: "gmail_get", Read: connector.PermissionDeny}}
	if err := configStore.Save(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := connectedGoalWatchHash(context.Background(), definition, state, now.Add(5*time.Minute)); err == nil || !strings.Contains(err.Error(), "cannot be read unattended") {
		t.Fatalf("revoked Gmail read permission did not stop watch: %v", err)
	}
}
