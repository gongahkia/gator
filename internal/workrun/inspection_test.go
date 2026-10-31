package workrun

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
	"github.com/gongahkia/gator/internal/executiontarget"
	"github.com/gongahkia/gator/internal/workhistory"
)

func TestOwnedInspectionAuthorizationBoundsAndLifecycle(t *testing.T) {
	state := t.TempDir()
	history, err := workhistory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := history.AcquireRun("inspection-run")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	_, err = history.Start(workhistory.Start{ID: "inspection-run", Objective: "private objective", Mode: action.Inspect, ExternalActions: artifact.InspectionContract().ExternalActions, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	live := newLiveInspection("inspection-run", "conversation-one", time.Now(), func() int { return 2 })
	control, _, err := startWorkControl(ctx, state, "inspection-run", cancel, nil, nil, nil, nil, nil, nil, live)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	actor := "inspection-" + strings.Repeat("a", 64)
	other := "inspection-" + strings.Repeat("b", 64)
	read := func(actor, id, generation string) InspectionResponse {
		t.Helper()
		result, err := InspectOwnedWork(context.Background(), state, actor, id, generation)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if len(read(actor, "", "").Targets) != 0 {
		t.Fatal("authenticated but ungranted inspection")
	}
	if err := GrantWorkInspection(ctx, state, "inspection-run", actor, 60); err != nil {
		t.Fatal(err)
	}
	page := read(actor, "", "")
	if len(page.Targets) != 1 {
		t.Fatalf("missing live target: %+v", page)
	}
	binding := page.Targets[0].Target.Binding
	if len(read(other, "", "").Targets) != 0 {
		t.Fatal("grant crossed devices")
	}
	if !read(actor, binding.ID, strings.Repeat("b", 32)).Unavailable {
		t.Fatal("stale generation accepted")
	}
	if !read(actor, "target-"+strings.Repeat("b", 32), binding.Generation).Unavailable {
		t.Fatal("wrong target accepted")
	}
	for i := 0; i < 100; i++ {
		live.record(agent.Event{Kind: agent.EventText, Text: strings.Repeat("<untrusted>", 100)})
	}
	live.record(agent.Event{Kind: agent.EventSubagent, TaskID: "inspection-run/subagent-001", Text: `{"id":"subagent-001","global_id":"inspection-run/subagent-001","parent_run":"inspection-run","role":"cli_codex","status":"running","native":{"status":"awaiting_input","thread_id":"secret-thread","questions":[{"id":"secret-question"}]}}`})
	selected := read(actor, binding.ID, binding.Generation).Targets[0]
	if len(selected.Progress) != 24 || len(selected.Progress[0]) > 512 || selected.PendingApprovals != 2 || selected.ConversationID != "conversation-one" || len(selected.Specialists) != 1 || !selected.Specialists[0].PendingQuestion {
		t.Fatalf("bad projection: %+v", selected)
	}
	data, _ := json.Marshal(selected)
	if strings.Contains(string(data), "secret-thread") || strings.Contains(string(data), "secret-question") || strings.Contains(string(data), "private objective") {
		t.Fatal("private payload leaked")
	}
	client, owner, err := connectWorkControl(ctx, state, "inspection-run")
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, owner.URL+"/inspection?actor="+actor+"&target="+binding.ID+"&generation="+binding.Generation, nil)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("missing control token accepted")
	}
	if err := GrantWorkInspection(ctx, state, "inspection-run", actor, 0); err != nil {
		t.Fatal(err)
	}
	if !read(actor, binding.ID, binding.Generation).Unavailable {
		t.Fatal("revoked grant survived")
	}
	if err := GrantWorkInspection(ctx, state, "inspection-run", actor, 60); err != nil {
		t.Fatal(err)
	}
	// A forged/stale nonce must not route to even a valid listening owner.
	path := filepath.Join(state, "gator", "work-control", "inspection-run.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var forged workControlState
	json.Unmarshal(original, &forged)
	forged.Nonce = strings.Repeat("f", 32)
	bytes, _ := json.Marshal(forged)
	if err := os.WriteFile(path, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	if !read(actor, binding.ID, binding.Generation).Unavailable {
		t.Fatal("stale nonce accepted")
	}
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	// Cancellation here simulates owner shutdown; no inspection triggers it.
	cancel()
	if !read(actor, binding.ID, binding.Generation).Unavailable {
		t.Fatal("stopped owner advertised running")
	}
}

func TestInspectionDoesNotConsumeControlsAndExpires(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	live := newLiveInspection("run", "chat", time.Now(), nil)
	mux := http.NewServeMux()
	binding, err := executiontarget.NewBinding(executiontarget.Work, "owner-instance", "")
	if err != nil {
		t.Fatal(err)
	}
	live.install(ctx, mux, func(next http.HandlerFunc) http.HandlerFunc { return next }, binding)
	actor := "inspection-" + strings.Repeat("a", 64)
	live.grants[actor] = executiontarget.Grant{Actor: executiontarget.Actor{ID: actor, Kind: executiontarget.Human, Origin: executiontarget.Remote}, Target: binding, Operation: executiontarget.Inspect, ExpiresAt: time.Now().Add(-time.Second)}
	if len(live.value.Progress) != 0 {
		t.Fatal("inspection invented output")
	}
	// Policy expiry is enforced by the owner handler, not a UI timer.
	r := httptest.NewRequest("GET", "/inspection?actor="+actor+"&target="+binding.ID+"&generation="+binding.Generation, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("expired owner grant accepted")
	}
}
