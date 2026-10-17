package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestCalendarWatchPaginatesSelectedWindowAndRejectsOverflow(t *testing.T) {
	oversized, repeatPage, duplicateSecond := false, false, false
	secondPageSummary := "Planning meeting"
	windowStart, windowEnd := "", ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		query := request.URL.Query()
		if query.Get("maxResults") != "100" || query.Get("singleEvents") != "true" || query.Get("timeMin") == "" || query.Get("timeMax") == "" {
			t.Errorf("unexpected Calendar query: %v", query)
		}
		if windowStart == "" {
			windowStart, windowEnd = query.Get("timeMin"), query.Get("timeMax")
		} else if query.Get("timeMin") != windowStart || query.Get("timeMax") != windowEnd {
			t.Errorf("Calendar window changed between pages: %v", query)
		}
		page := 1
		if token := query.Get("pageToken"); token != "" {
			var err error
			page, err = strconv.Atoi(token)
			if err != nil {
				t.Error(err)
			}
		}
		count := 1
		if page == 1 || oversized {
			count = 100
		}
		events := make([]string, 0, count)
		for index := range count {
			summary := "Planning meeting"
			if page == 2 && index == 0 {
				summary = secondPageSummary
			}
			id := (page-1)*100 + index
			if duplicateSecond && page == 2 && index == 0 {
				id = 0
			}
			events = append(events, fmt.Sprintf(`{"id":"event-%03d","summary":%q}`, id, summary))
		}
		more := ""
		if page == 1 || oversized || repeatPage {
			next := strconv.Itoa(page + 1)
			if repeatPage && page > 1 {
				next = "2"
			}
			more = fmt.Sprintf(`,"nextPageToken":%q`, next)
		}
		_, _ = fmt.Fprintf(writer, `{"items":[%s]%s}`, strings.Join(events, ","), more)
	}))
	defer server.Close()
	descriptor := connector.Descriptor{Version: connector.DescriptorVersion, ID: "google", Name: "Google", Kind: connector.KindGoogle, Resource: server.URL, Authentication: connector.AuthNone}
	registry, err := connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := connector.Runtime{Registry: registry, HTTPClient: server.Client()}
	source := jobs.WatchSource{ConnectorID: "google", Operation: "calendar_events", Days: 14}
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	firstHash := sha256.New()
	items, err := hashCalendarWatch(context.Background(), firstHash, runtime, source, now)
	if err != nil || len(items) != 101 || items[0].ID != "event-000" || items[100].ID != "event-100" {
		t.Fatalf("two-page Calendar watch = %d items, %v", len(items), err)
	}
	secondPageSummary = "Planning meeting moved"
	secondHash := sha256.New()
	if _, err := hashCalendarWatch(context.Background(), secondHash, runtime, source, now); err != nil || string(firstHash.Sum(nil)) == string(secondHash.Sum(nil)) {
		t.Fatalf("second-page event change was missed: %v", err)
	}
	oversized = true
	if _, err := hashCalendarWatch(context.Background(), sha256.New(), runtime, source, now); err == nil || !strings.Contains(err.Error(), "more than 500") {
		t.Fatalf("overlarge Calendar window was accepted: %v", err)
	}
	oversized, repeatPage = false, true
	if _, err := hashCalendarWatch(context.Background(), sha256.New(), runtime, source, now); err == nil || !strings.Contains(err.Error(), "invalid pagination") {
		t.Fatalf("repeated Calendar page was accepted: %v", err)
	}
	repeatPage, duplicateSecond = false, true
	if _, err := hashCalendarWatch(context.Background(), sha256.New(), runtime, source, now); err == nil || !strings.Contains(err.Error(), "duplicate event ID") {
		t.Fatalf("duplicate Calendar event was accepted: %v", err)
	}
}

func TestGmailWatchPaginatesAndBoundsSelectedMessages(t *testing.T) {
	oversized := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		page := 1
		if token := request.URL.Query().Get("pageToken"); token != "" {
			var err error
			page, err = strconv.Atoi(token)
			if err != nil {
				t.Error(err)
			}
		}
		count := 1
		if page == 1 || oversized {
			count = 100
		}
		messages := make([]string, 0, count)
		for index := range count {
			messages = append(messages, fmt.Sprintf(`{"id":"mail-%03d"}`, (page-1)*100+index))
		}
		more := ""
		if page == 1 || oversized {
			more = fmt.Sprintf(`,"nextPageToken":%q`, strconv.Itoa(page+1))
		}
		_, _ = fmt.Fprintf(writer, `{"messages":[%s]%s}`, strings.Join(messages, ","), more)
	}))
	defer server.Close()
	descriptor := connector.Descriptor{Version: connector.DescriptorVersion, ID: "google", Name: "Google", Kind: connector.KindGoogle, Resource: server.URL, Authentication: connector.AuthNone}
	registry, err := connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := connector.Runtime{Registry: registry, HTTPClient: server.Client()}
	ids, err := listGmailWatchIDs(context.Background(), runtime, jobs.WatchSource{ConnectorID: "google", Operation: "gmail_list", Query: "label:offsite"})
	if err != nil || len(ids) != 101 || ids[0] != "mail-000" || ids[100] != "mail-100" {
		t.Fatalf("two-page Gmail list = %d IDs, %v", len(ids), err)
	}
	oversized = true
	if _, err := listGmailWatchIDs(context.Background(), runtime, jobs.WatchSource{ConnectorID: "google", Operation: "gmail_list", Query: "label:offsite"}); err == nil || !strings.Contains(err.Error(), "more than 500") {
		t.Fatalf("overlarge Gmail list was accepted: %v", err)
	}
}

func TestGmailHistoryPaginatesAndDoesNotHideProviderErrors(t *testing.T) {
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if fail {
			writer.WriteHeader(http.StatusTooManyRequests)
			_, _ = writer.Write([]byte(`{"error":{"code":429}}`))
			return
		}
		if request.URL.Query().Get("startHistoryId") != "100" {
			t.Errorf("history cursor = %q", request.URL.Query().Get("startHistoryId"))
		}
		if request.URL.Query().Get("pageToken") == "second" {
			_, _ = writer.Write([]byte(`{"historyId":"102","history":[{"id":"102","labelsAdded":[{"message":{"id":"mail-2"}}]}]}`))
		} else {
			_, _ = writer.Write([]byte(`{"historyId":"102","history":[{"id":"101","messagesAdded":[{"message":{"id":"mail-1"}}]}],"nextPageToken":"second"}`))
		}
	}))
	defer server.Close()
	descriptor := connector.Descriptor{Version: connector.DescriptorVersion, ID: "google", Name: "Google", Kind: connector.KindGoogle, Resource: server.URL, Authentication: connector.AuthNone}
	registry, err := connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := connector.Runtime{Registry: registry, HTTPClient: server.Client()}
	changed, cursor, fullScan, err := gmailHistoryChanges(context.Background(), runtime, "google", "100")
	if err != nil || fullScan || cursor != "102" || !changed["mail-1"] || !changed["mail-2"] || len(changed) != 2 {
		t.Fatalf("history pages = %#v, %q, full=%t, err=%v", changed, cursor, fullScan, err)
	}
	fail = true
	if _, _, fullScan, err := gmailHistoryChanges(context.Background(), runtime, "google", "100"); err == nil || fullScan {
		t.Fatalf("HTTP 429 was hidden as a full scan: %v, full=%t", err, fullScan)
	}
}

func TestGmailWatchDoesNotReuseWrongScopeOrUnpinnedCursor(t *testing.T) {
	profileReads, historyReads, detailReads := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/gmail/v1/users/me/profile":
			profileReads++
			_, _ = writer.Write([]byte(`{"historyId":"100"}`))
		case "/gmail/v1/users/me/history":
			historyReads++
			_, _ = writer.Write([]byte(`{"historyId":"101"}`))
		case "/gmail/v1/users/me/messages":
			_, _ = writer.Write([]byte(`{"messages":[{"id":"mail-1"}]}`))
		case "/gmail/v1/users/me/messages/mail-1":
			detailReads++
			_, _ = writer.Write([]byte(`{"id":"mail-1","snippet":"Current account"}`))
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	descriptor := connector.Descriptor{Version: connector.DescriptorVersion, ID: "google", Name: "Google", Kind: connector.KindGoogle, Resource: server.URL, Authentication: connector.AuthNone}
	registry, err := connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := connector.Runtime{Registry: registry, HTTPClient: server.Client()}
	source := jobs.WatchSource{ConnectorID: "google", Operation: "gmail_list", Query: "label:offsite"}
	badPrior := &jobs.Attempt{WatchItems: []jobs.WatchItem{{Source: "google/gmail_list", ID: "mail-1", SHA256: strings.Repeat("a", 64)}}, WatchCursors: []jobs.WatchCursor{{Source: "google/gmail_list", ScopeSHA256: strings.Repeat("b", 64), HistoryID: "99", FullSyncAt: time.Now().UTC()}}}
	items, cursor, err := hashGmailWatch(context.Background(), runtime, registry, nil, source, time.Now(), badPrior, true)
	if err != nil || profileReads != 1 || historyReads != 0 || detailReads != 1 || len(items) != 1 || items[0].SHA256 == strings.Repeat("a", 64) || cursor.ScopeSHA256 == badPrior.WatchCursors[0].ScopeSHA256 {
		t.Fatalf("wrong-scope cursor was reused: items=%#v cursor=%#v err=%v counts=%d/%d/%d", items, cursor, err, profileReads, historyReads, detailReads)
	}
	prior := &jobs.Attempt{WatchItems: items, WatchCursors: []jobs.WatchCursor{cursor}}
	if _, _, err := hashGmailWatch(context.Background(), runtime, registry, nil, source, time.Now(), prior, false); err != nil || profileReads != 2 || historyReads != 0 || detailReads != 2 {
		t.Fatalf("unpinned cursor was reused: err=%v counts=%d/%d/%d", err, profileReads, historyReads, detailReads)
	}
}

func TestGmailWatchCursorSurvivesRestartAndRescansAfterExpiry(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	snippet := "Venue available"
	changed, expired, selectedVisible := false, false, true
	profileID := "100"
	profileReads, listReads, detailReads, historyReads := 0, 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/gmail/v1/users/me/profile":
			profileReads++
			_, _ = fmt.Fprintf(writer, `{"historyId":%q}`, profileID)
		case "/gmail/v1/users/me/history":
			historyReads++
			if expired {
				writer.WriteHeader(http.StatusNotFound)
				_, _ = writer.Write([]byte(`{"error":{"code":404}}`))
				return
			}
			if request.URL.Query().Get("startHistoryId") == "100" && !changed {
				_, _ = writer.Write([]byte(`{"historyId":"101"}`))
			} else if request.URL.Query().Get("startHistoryId") == "101" && changed {
				_, _ = writer.Write([]byte(`{"historyId":"102","history":[{"id":"102","messages":[{"id":"mail-1"}]}]}`))
			} else if request.URL.Query().Get("startHistoryId") == "200" && !changed {
				_, _ = writer.Write([]byte(`{"historyId":"201"}`))
			} else {
				t.Errorf("unexpected Gmail history cursor %q", request.URL.Query().Get("startHistoryId"))
				writer.WriteHeader(http.StatusBadRequest)
			}
		case "/gmail/v1/users/me/messages":
			listReads++
			if request.URL.Query().Get("q") != "label:offsite" {
				t.Errorf("watch query = %q", request.URL.Query().Get("q"))
			}
			if selectedVisible {
				_, _ = writer.Write([]byte(`{"messages":[{"id":"mail-1"}]}`))
			} else {
				_, _ = writer.Write([]byte(`{}`))
			}
		case "/gmail/v1/users/me/messages/mail-1":
			detailReads++
			_, _ = fmt.Fprintf(writer, `{"id":"mail-1","snippet":%q}`, snippet)
		default:
			t.Errorf("unexpected Gmail path %s", request.URL.Path)
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
	goalRecord, err := goals.Add("Plan offsite", "Watch selected mail", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.Save(jobs.Definition{Name: "Offsite mail", GoalID: goalRecord.ID, GoalWatch: true, Enabled: true, Schedule: "*/5 * * * *", Timezone: "UTC", Missed: "run_once", SourcePath: goalRecord.SourcePath, RefreshSnapshot: true, ChangeOnly: true, Objective: "Watch selected offsite mail", Mode: action.Inspect, Contract: artifact.InspectionContract(), ConnectorIDs: []string{"google"}, WatchSources: []jobs.WatchSource{{ConnectorID: "google", Operation: "gmail_list", Query: "label:offsite"}}, MaxSteps: 4})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(5 * time.Minute).Add(5 * time.Minute)
	modelCalls := 0
	execute := func(context.Context, jobs.Definition, string, ...string) (jobResult, error) {
		modelCalls++
		return jobResult{Status: "completed", Summary: "Checked offsite", ResultSHA256: fmt.Sprint(modelCalls)}, nil
	}
	runDueJobsContextWith(context.Background(), store, state, false, now, execute)
	if profileReads != 1 || listReads != 1 || detailReads != 1 || historyReads != 0 || modelCalls != 1 {
		t.Fatalf("initial scan profile=%d list=%d detail=%d history=%d model=%d", profileReads, listReads, detailReads, historyReads, modelCalls)
	}
	store, err = jobs.Open(state) // restart should use the retained cursor
	if err != nil {
		t.Fatal(err)
	}
	runDueJobsContextWith(context.Background(), store, state, false, now.Add(5*time.Minute), execute)
	if profileReads != 1 || listReads != 2 || detailReads != 1 || historyReads != 1 || modelCalls != 1 {
		t.Fatalf("quiet poll profile=%d list=%d detail=%d history=%d model=%d", profileReads, listReads, detailReads, historyReads, modelCalls)
	}
	history, err := store.History(definition.ID, 0)
	if err != nil || len(history) != 2 || history[0].Status != "unchanged" || len(history[0].WatchItems) < 2 || len(history[0].WatchCursors) != 1 || history[0].WatchCursors[0].HistoryID != "101" {
		t.Fatalf("quiet cursor was not retained: %#v, %v", history, err)
	}
	changed, snippet = true, "Venue cancelled"
	runDueJobsContextWith(context.Background(), store, state, false, now.Add(10*time.Minute), execute)
	if detailReads != 2 || historyReads != 2 || modelCalls != 2 {
		t.Fatalf("changed poll detail=%d history=%d model=%d", detailReads, historyReads, modelCalls)
	}
	expired, profileID = true, "200"
	runDueJobsContextWith(context.Background(), store, state, false, now.Add(15*time.Minute), execute)
	if profileReads != 2 || detailReads != 3 || historyReads != 3 || modelCalls != 2 {
		t.Fatalf("expired cursor recovery profile=%d detail=%d history=%d model=%d", profileReads, detailReads, historyReads, modelCalls)
	}
	history, err = store.History(definition.ID, 0)
	if err != nil || len(history) != 4 || history[0].Status != "unchanged" || len(history[0].WatchCursors) != 1 || history[0].WatchCursors[0].HistoryID != "200" {
		t.Fatalf("full sync cursor was not retained: %#v, %v", history, err)
	}
	// A time-relative or label-based query can lose a match without a new
	// mailbox history event. Listing the selected query on every poll catches it.
	expired, changed, selectedVisible = false, false, false
	runDueJobsContextWith(context.Background(), store, state, false, now.Add(20*time.Minute), execute)
	if historyReads != 4 || listReads != 5 || detailReads != 3 || modelCalls != 3 {
		t.Fatalf("query membership change history=%d list=%d detail=%d model=%d", historyReads, listReads, detailReads, modelCalls)
	}
	history, err = store.History(definition.ID, 0)
	if err != nil || len(history) != 5 || history[0].Status != "completed" || len(history[0].WatchChanges) != 1 || history[0].WatchChanges[0].Kind != "removed" {
		t.Fatalf("removed selected message was missed: %#v, %v", history, err)
	}
}

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
	execute := func(_ context.Context, current jobs.Definition, _ string, _ ...string) (jobResult, error) {
		calls++
		request := jobWorkRequest(current)
		if calls == 1 && !strings.Contains(request.System, `"kind":"added"`) || calls == 2 && !strings.Contains(request.System, `"kind":"updated"`) {
			t.Errorf("model was not told which watched item changed: %s", request.System)
		}
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
	if len(history[0].WatchChanges) != 1 || history[0].WatchChanges[0].Source != "planning/fetch" || history[0].WatchChanges[0].Kind != "updated" {
		t.Fatalf("item-level change was not retained: %#v", history[0].WatchChanges)
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
		case "/gmail/v1/users/me/profile":
			_, _ = writer.Write([]byte(`{"historyId":"100"}`))
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
	if _, err := connectedGoalWatchHash(context.Background(), definition, state, now.Add(4*time.Minute)); err == nil {
		t.Fatalf("repeated Gmail page looked complete: %v", err)
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
