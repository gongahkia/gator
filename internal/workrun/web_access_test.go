package workrun

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/tools"
)

func accessCall(t *testing.T, surface []agent.Tool, name string, args string) (agent.ToolResult, error) {
	t.Helper()
	for _, tool := range surface {
		if tool.Definition().Name == name {
			return tool.Execute(context.Background(), json.RawMessage(args))
		}
	}
	t.Fatalf("missing tool %s", name)
	return agent.ToolResult{}, nil
}

func TestPublicResearchWaitsForBoundForegroundReviewAndRetainsFetchedEvidence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	state, source := t.TempDir(), t.TempDir()
	http := &searchFixture{}
	model := &scriptedModel{turns: []agent.Turn{
		dynamicCall("web_access", map[string]any{"action": "request", "scope": "public", "reason": "Discover primary sources beyond the initially selected sites."}),
		dynamicCall("web_search", map[string]any{"query": "venue deadlines"}),
		dynamicCall("http_fetch", map[string]any{"url": "https://other.invalid/secret"}),
		{Text: "Read the newly discovered primary page."},
	}}
	service := Service{Executor: Executor{Model: model, StateDir: state, HTTP: tools.HTTPFetchOptions{Client: http, Resolver: researchResolver{}, BraveSearchAPIKey: "fixture-secret"}}}
	operation := service.Start(ctx, Request{SourcePath: source, Objective: "Research primary sources", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 5, ForegroundApp: true, DisableBrowser: true})
	defer operation.Cancel()
	var interaction Interaction
	select {
	case interaction = <-operation.Interactions:
	case <-ctx.Done():
		t.Fatal("review did not arrive")
	}
	if http.calls != 0 || interaction.Kind != "web_access" {
		t.Fatal("network ran before scope review", http.calls, interaction.Kind)
	}
	review, ok := interaction.Preview.(WebAccessReview)
	if !ok || review.Scope != "public" || review.MaxRequests != 32 || review.MaxSearches != 8 || review.SearchProvider != "brave" || !strings.Contains(review.Lifetime, "provider charges") {
		t.Fatal("incomplete host review", review)
	}
	if err := operation.respondBound(interaction.ID, strings.Repeat("0", 64), true); err == nil {
		t.Fatal("wrong review digest approved scope")
	}
	digest, _ := InteractionDigest(interaction)
	if err := operation.respondBound(interaction.ID, digest, true); err != nil {
		t.Fatal(err)
	}
	var completed Completion
	select {
	case completed = <-operation.Done:
	case <-ctx.Done():
		t.Fatal("research did not finish")
	}
	if completed.Err != nil {
		t.Fatal(completed.Err)
	}
	out := completed.Outcome
	if http.calls != 2 || strings.Contains(http.query, "site:") || len(out.Manifest.Evidence) != 1 || out.Manifest.Evidence[0].Locator != "https://other.invalid/secret" {
		t.Fatal("public discovery/evidence failed", http.calls, http.query, out.Manifest.Evidence)
	}
	if r := out.Manifest.Research; r == nil || !r.PublicWeb || r.Requests != 2 || r.Searches != 1 || r.ScopeReviews != 1 {
		t.Fatal("missing shared usage", r)
	}
	if len(out.Manifest.Actions) != 0 {
		t.Fatal("web consent became external-action authority")
	}
	record, err := ReadInteractionRecord(state, operation.RunID, interaction.ID)
	if err != nil || record.Status != "resolved" || record.Approved == nil || !*record.Approved {
		t.Fatal("review evidence not retained", record, err)
	}
	// An explicit continuation can retain old evidence but never its grant.
	nextModel := &scriptedModel{turns: []agent.Turn{dynamicCall("http_fetch", map[string]any{"url": "https://other.invalid/new"}), {Text: "New web access needs review."}}}
	second, err := (Executor{Model: nextModel, StateDir: state, HTTP: service.Executor.HTTP}).Execute(ctx, Request{SourcePath: source, ConversationID: out.ConversationID, ParentRevisionID: out.RevisionID, Objective: "Check again without a new grant", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 3, DisableBrowser: true, ForegroundApp: true, ApproveWebAccess: func(context.Context, WebAccessReview) (bool, error) {
		t.Error("direct read unexpectedly acquired consent")
		return false, nil
	}})
	if err != nil || http.calls != 2 || second.Manifest.Research == nil || second.Manifest.Research.PublicWeb || second.Manifest.Research.Requests != 0 {
		t.Fatal("retained grant authorized a later task", err, second.Manifest.Research)
	}
}

func TestWebScopeDenialInvalidRequestsAndRevocation(t *testing.T) {
	reviews := 0
	http := &searchFixture{}
	r := Request{ForegroundApp: true, ApproveWebAccess: func(context.Context, WebAccessReview) (bool, error) { reviews++; return false, nil }}
	surface, _, scope, err := newWebResearch(&evidenceCatalog{}, r, tools.HTTPFetchOptions{Client: http, Resolver: researchResolver{}, BraveSearchAPIKey: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []string{`{"action":"request","scope":"public","reason":"Research","max_requests":1000}`, `{"action":"request","scope":"sites","origins":["https://127.0.0.1"],"reason":"Research"}`, `{"action":"request","scope":"sites","origins":["https://public.example/path"],"reason":"Research"}`} {
		if _, err := accessCall(t, surface, "web_access", args); err == nil {
			t.Fatal("invalid grant accepted", args)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := accessCall(t, surface, "web_access", `{"action":"request","scope":"public","reason":"Research"}`); err == nil {
			t.Fatal("denied grant accepted")
		}
	}
	if reviews != 1 {
		t.Fatal("denied scope prompted again", reviews)
	}
	for _, name := range []string{"http_fetch", "web_search"} {
		args := `{"url":"https://other.invalid/secret"}`
		if name == "web_search" {
			args = `{"query":"research"}`
		}
		if _, err := accessCall(t, surface, name, args); err == nil {
			t.Fatal("denied scope reached a read", name)
		}
	}
	if http.calls != 0 || scope.evidence().Requests != 0 {
		t.Fatal("denial dispatched or consumed network capacity")
	}
	// A narrower grant remains reviewable after a public-web denial.
	scope.request.ApproveWebAccess = func(context.Context, WebAccessReview) (bool, error) { reviews++; return true, nil }
	if _, err := accessCall(t, surface, "web_access", `{"action":"request","scope":"sites","origins":["https://research.invalid"],"reason":"Read only this site"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := scope.approve(context.Background(), []string{"http_fetch", "https://research.invalid/fact"}); err != nil {
		t.Fatal(err)
	}
	if _, err := scope.approve(context.Background(), []string{"http_fetch", "https://other.invalid/fact"}); err == nil {
		t.Fatal("site grant allowed another origin")
	}
	if _, err := accessCall(t, surface, "web_access", `{"action":"revoke"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := scope.approve(context.Background(), []string{"http_fetch", "https://research.invalid/fact"}); err == nil {
		t.Fatal("revoked grant still admitted reads")
	}
}

func TestWebScopeSharedBudgetAcrossConcurrentCallers(t *testing.T) {
	_, _, scope, err := newWebResearch(&evidenceCatalog{}, Request{WebOrigins: []string{"https://research.invalid"}}, tools.HTTPFetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = scope.approve(context.Background(), []string{"web_search", "research"})
		}()
	}
	wg.Wait()
	if e := scope.evidence(); e.Searches != researchSearchLimit || e.Requests != researchSearchLimit {
		t.Fatal("search limit reset across callers", e)
	}
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = scope.approve(context.Background(), []string{"http_fetch", "https://research.invalid/fact"})
		}()
	}
	wg.Wait()
	if e := scope.evidence(); e.Requests != researchRequestLimit || e.Searches != researchSearchLimit {
		t.Fatal("shared total limit reset", e)
	}
	if _, err := scope.approve(context.Background(), []string{"http_fetch", "https://research.invalid/fact"}); err == nil {
		t.Fatal("exhausted budget admitted another read")
	}
}

func TestRevocationDuringReviewExpiresApproval(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	r := Request{ForegroundApp: true, ApproveWebAccess: func(context.Context, WebAccessReview) (bool, error) { close(started); <-release; return true, nil }}
	surface, _, scope, err := newWebResearch(&evidenceCatalog{}, r, tools.HTTPFetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		_, err := accessCall(t, surface, "web_access", `{"action":"request","scope":"public","reason":"Investigate"}`)
		finished <- err
	}()
	<-started
	if _, err := accessCall(t, surface, "web_access", `{"action":"revoke"}`); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-finished; err == nil || !strings.Contains(err.Error(), "approval expired") {
		t.Fatal("late approval reinstated revoked access", err)
	}
	if public, _ := scope.scope(); public {
		t.Fatal("expired approval widened scope")
	}
}

func TestBackgroundResearchCannotRequestExpansion(t *testing.T) {
	called := false
	r := Request{ApproveWebAccess: func(context.Context, WebAccessReview) (bool, error) { called = true; return true, nil }}
	surface, _, scope, err := newWebResearch(&evidenceCatalog{}, r, tools.HTTPFetchOptions{})
	if err != nil || len(surface) != 0 || scope != nil || called {
		t.Fatal("background task acquired new research surface", err)
	}
}

func TestManagerAndDynamicResearcherShareSearchCapacity(t *testing.T) {
	http := &searchFixture{}
	calls := []agent.ToolCall{}
	for i := 0; i < 9; i++ {
		calls = append(calls, agent.ToolCall{ID: strings.Repeat("q", i+1), Name: "web_search", Arguments: json.RawMessage(`{"query":"primary venue evidence"}`)})
	}
	child := &scriptedModel{turns: []agent.Turn{{ToolCalls: calls}, {Text: "Shared search budget reached; no factual claim made from snippets."}}}
	manager := &scriptedModel{turns: []agent.Turn{
		dynamicCall("web_access", map[string]any{"action": "request", "scope": "public", "reason": "Research primary sites"}),
		dynamicCall("create_agent", map[string]any{"name": "public_researcher", "base": "connected_researcher", "description": "Research primary sources", "instructions": "Search for primary evidence, then read useful pages.", "task": "Investigate the venue"}),
		dynamicCall("await_agent", map[string]any{"id": "subagent-001", "timeout_seconds": 5}),
		dynamicCall("web_search", map[string]any{"query": "another primary source"}),
		{Text: "No additional search capacity remains."},
	}}
	out, err := (Executor{Model: manager, RoleModels: map[string]agent.Model{"connected_researcher": child}, StateDir: t.TempDir(), HTTP: tools.HTTPFetchOptions{Client: http, Resolver: researchResolver{}, BraveSearchAPIKey: "fixture"}}).Execute(context.Background(), Request{SourcePath: t.TempDir(), Objective: "Use a research specialist", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 6, ForegroundApp: true, ApproveWebAccess: func(context.Context, WebAccessReview) (bool, error) { return true, nil }, DisableBrowser: true})
	if err != nil || http.calls != 8 || len(out.Manifest.Subagents) != 1 || out.Manifest.Research.Searches != 8 || out.Manifest.Research.Requests != 8 {
		t.Fatal("worker reset shared research budget", http.calls, out.Manifest.Research, err)
	}
	for _, tool := range child.requests[0].Tools {
		if tool.Name == "web_access" {
			t.Fatal("worker gained authority to request new grants")
		}
	}
	if !strings.Contains(manager.requests[len(manager.requests)-1].Messages[len(manager.requests[len(manager.requests)-1].Messages)-1].Content, "budget exhausted") {
		t.Fatal("manager did not observe child's exhausted shared budget")
	}
}

type privateResearchResolver struct{}

func (privateResearchResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("10.1.2.3")}}, nil
}

func TestPublicGrantDoesNotBypassPrivateDNSOrCancelledReview(t *testing.T) {
	http := &searchFixture{}
	r := Request{ForegroundApp: true, ApproveWebAccess: func(context.Context, WebAccessReview) (bool, error) { return true, nil }}
	surface, _, _, err := newWebResearch(&evidenceCatalog{}, r, tools.HTTPFetchOptions{Client: http, Resolver: privateResearchResolver{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accessCall(t, surface, "web_access", `{"action":"request","scope":"public","reason":"Investigate"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := accessCall(t, surface, "http_fetch", `{"url":"https://internal.example/secret"}`); err == nil || !strings.Contains(err.Error(), "private or reserved") {
		t.Fatal("public consent bypassed DNS protections", err)
	}
	if http.calls != 0 {
		t.Fatal("private address reached transport")
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.ApproveWebAccess = func(context.Context, WebAccessReview) (bool, error) { cancel(); return true, nil }
	surface, _, scope, err := newWebResearch(&evidenceCatalog{}, r, tools.HTTPFetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = surface[len(surface)-1].Execute(ctx, json.RawMessage(`{"action":"request","scope":"public","reason":"Investigate"}`))
	if err != context.Canceled || scope.evidence().PublicWeb {
		t.Fatal("cancelled review granted access", err)
	}
}

func TestWebExpansionChecksReviewedPlanAgainAfterConsent(t *testing.T) {
	_, store, request := claimedPlanFixture(t, []string{"read"})
	request.ForegroundApp = true
	request.ApproveWebAccess = func(context.Context, WebAccessReview) (bool, error) {
		journal, err := store.LoadRequests(request.ConversationID)
		if err != nil {
			return false, err
		}
		journal.Requests[0].Plan.Phase = "cancelled"
		_, err = store.SaveRequests(request.ConversationID, journal)
		return true, err
	}
	surface, _, scope, err := newWebResearch(&evidenceCatalog{}, request, tools.HTTPFetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accessCall(t, surface, "web_access", `{"action":"request","scope":"public","reason":"Research the reviewed task"}`); err == nil {
		t.Fatal("stale reviewed plan gained new scope")
	}
	if scope.evidence().PublicWeb {
		t.Fatal("consent bypassed invalidated task plan")
	}
}
