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
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
)

func TestGoalAccessChecksSelectedReadsWithoutChangingWatchHistory(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_STATE_DIR", state)
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.URL.String())
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/gmail/v1/users/me/messages":
			if request.URL.Query().Get("maxResults") != "1" || request.URL.Query().Get("q") != "label:offsite" {
				t.Errorf("Gmail probe query = %s", request.URL.String())
			}
			_, _ = writer.Write([]byte(`{"messages":[{"id":"mail-1"}]}`))
		case "/gmail/v1/users/me/messages/mail-1":
			_, _ = writer.Write([]byte(`{"id":"mail-1","snippet":"private mail content"}`))
		default:
			t.Errorf("unexpected probe path %s", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	settings := config.Default()
	settings.Connectors = []connector.Descriptor{{Version: connector.DescriptorVersion, ID: "google", Name: "Google", Kind: connector.KindGoogle, Resource: server.URL, Authentication: connector.AuthNone}}
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
	record, err := goals.Add("Plan offsite", "", source)
	if err != nil {
		t.Fatal(err)
	}
	jobStore, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := jobStore.Save(jobs.Definition{
		Name: "Offsite watch", GoalID: record.ID, Enabled: true, GoalWatch: true,
		Schedule: "*/5 * * * *", Timezone: "UTC", Missed: "run_once", SourcePath: source,
		RefreshSnapshot: true, ChangeOnly: true, Objective: "Watch offsite changes",
		Mode: action.Inspect, Contract: artifact.InspectionContract(), ConnectorIDs: []string{"google"},
		WatchSources: []jobs.WatchSource{{ConnectorID: "google", Operation: "gmail_list", Query: "label:offsite"}, {ConnectorID: "google", Operation: "calendar_events", Days: 7}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := probeGoalAccess(context.Background(), state, record.ID, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "list and one message detail reachable") || !strings.Contains(text, "Calendar support was removed") || strings.Contains(text, "private mail content") || strings.Contains(text, "label:offsite") {
		t.Fatalf("goal access output = %q", text)
	}
	if len(requests) != 2 {
		t.Fatalf("probe made %d requests: %#v", len(requests), requests)
	}
	history, err := jobStore.History(definition.ID, 0)
	if err != nil || len(history) != 0 {
		t.Fatalf("probe changed watch history: %#v, %v", history, err)
	}
}

func TestGoalAccessDoesNotProbeRevokedReadOrExposeServiceBody(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte(`{"private":"secret response"}`))
	}))
	defer server.Close()
	registry, err := connector.NewRegistry([]connector.Descriptor{{Version: connector.DescriptorVersion, ID: "metrics", Name: "Metrics", Kind: connector.KindHTTPJSON, Resource: server.URL, Authentication: connector.AuthNone}})
	if err != nil {
		t.Fatal(err)
	}
	runtime := connector.Runtime{Registry: registry, HTTPClient: server.Client()}
	permissions := connector.PermissionSet{{ConnectorID: "metrics", Operation: "fetch", Read: connector.PermissionDeny}}
	status := probeGoalWatchSource(context.Background(), runtime, registry, permissions, mustGoalAccessCredentials(t), jobs.WatchSource{ConnectorID: "metrics", Operation: "fetch"}, time.Now())
	if calls != 0 || !strings.Contains(status, "permission is missing") {
		t.Fatalf("revoked probe = %q, calls=%d", status, calls)
	}
	status = probeGoalWatchSource(context.Background(), runtime, registry, nil, mustGoalAccessCredentials(t), jobs.WatchSource{ConnectorID: "metrics", Operation: "fetch"}, time.Now())
	if calls != 1 || !strings.Contains(status, "forbidden") || strings.Contains(status, "secret response") {
		t.Fatalf("failed probe = %q, calls=%d", status, calls)
	}
}

func mustGoalAccessCredentials(t *testing.T) auth.Store {
	t.Helper()
	store, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}
