package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/model/openai"
	"github.com/gongahkia/gator/internal/workrun"
)

// Explicit opt-in. A synthetic foreground owner consents; native Electron
// review is tested separately. Search and model spend use the same durable
// cumulative allowance, including failed attempts and unused search reserve.
func TestLivePublicWebResearch(t *testing.T) {
	path := os.Getenv("GATOR_LIVE_WEB_RESEARCH_REPORT")
	if path == "" {
		t.Skip("opt-in bounded public-web research acceptance")
	}
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Fatal("model credential unavailable")
	}
	options := webResearchOptionsFromEnvironment()
	provider, err := options.SearchStatus()
	if err != nil || provider == "" {
		t.Fatal("configure one host search provider for the live discovery acceptance")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	report := struct {
		Milliseconds  int64
		Reviews       []workrun.WebAccessReview
		Tools, Errors []string
		Answer, Error string
		Evidence      []artifact.Evidence
		Research      *artifact.ResearchEvidence
		Checks        map[string]bool
		Spend         projectEvalSpend
	}{Checks: map[string]bool{}}
	defer func() {
		data, _ := os.ReadFile(filepath.Join(filepath.Dir(path), "budget.json"))
		_ = json.Unmarshal(data, &report.Spend)
		e := json.NewEncoder(f)
		e.SetIndent("", "  ")
		if err := e.Encode(report); err != nil {
			t.Error(err)
		}
	}()
	if err := reserveProjectSearchCalls(filepath.Dir(path), 8); err != nil {
		t.Fatal(err)
	}
	check := func(name string, ok bool) {
		t.Helper()
		report.Checks[name] = ok
		if !ok {
			t.Error(name)
		}
	}
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	backend := projectEvalModel{openai.Responses{APIKey: os.Getenv("OPENAI_API_KEY"), Model: "gpt-5.6", BaseURL: "https://us.api.openai.com/v1/responses", Client: &http.Client{Transport: projectEvalTransport{directory: filepath.Dir(path)}, Timeout: 60 * time.Second}}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var events sync.Mutex
	start := time.Now()
	out, err := (workrun.Executor{Model: backend, StateDir: state, HTTP: options}).Execute(ctx, workrun.Request{
		SourcePath: t.TempDir(), Objective: "Research this question from official documentation: does Go's net/http Client follow redirects by default, and how can a caller stop redirect following? No websites are preselected. First request public web access for this task, then use web search to find the relevant official Go documentation yourself and read useful primary pages. Return a concise sourced answer based on the pages you actually read. Keep the research bounded, use no accounts, do not change files or send anything.",
		Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 9, ForegroundApp: true, DisableBrowser: true, DisableDelegation: true, DisableChatHistory: true, Limits: agent.Limits{ModelRequests: 10, Tokens: 120000, WallSeconds: 150},
		ApproveWebAccess: func(_ context.Context, review workrun.WebAccessReview) (bool, error) {
			report.Reviews = append(report.Reviews, review)
			return review.Scope == "public" && review.MaxRequests == 32 && review.MaxSearches == 8, nil
		},
		OnEvent: func(e agent.Event) {
			events.Lock()
			defer events.Unlock()
			if e.Kind == agent.EventToolCalled && e.ToolCall != nil {
				report.Tools = append(report.Tools, e.ToolCall.Name)
			}
			if e.ToolError != "" {
				report.Errors = append(report.Errors, e.ToolError)
			}
		},
	})
	report.Milliseconds = time.Since(start).Milliseconds()
	report.Answer = out.Result.FinalText
	report.Evidence = out.Manifest.Evidence
	report.Research = out.Manifest.Research
	if err != nil {
		report.Error = err.Error()
		t.Error(err)
	}
	check("foreground public scope reviewed once", len(report.Reviews) == 1 && report.Reviews[0].Scope == "public")
	check("real search and page fetch used", strings.Contains(strings.Join(report.Tools, ","), "web_search") && strings.Contains(strings.Join(report.Tools, ","), "http_fetch"))
	official := false
	for _, e := range report.Evidence {
		u, _ := url.Parse(e.Locator)
		if u != nil && (u.Host == "go.dev" || u.Host == "pkg.go.dev") && e.SnapshotPath != "" {
			official = true
		}
	}
	check("previously unselected official page retained", official)
	check("historical scope and shared usage retained", report.Research != nil && report.Research.PublicWeb && report.Research.Searches >= 1 && report.Research.Searches <= 8 && report.Research.Requests <= 32)
	check("answer identifies redirect control", strings.Contains(report.Answer, "CheckRedirect") && strings.Contains(report.Answer, "ErrUseLastResponse"))
	check("answer cites retrieved evidence", strings.Contains(report.Answer, "gator-evidence:") || strings.Contains(report.Answer, "https://pkg.go.dev/") || strings.Contains(report.Answer, "https://go.dev/"))
	check("no external action recorded", len(out.Manifest.Actions) == 0)
	check("no tool errors", len(report.Errors) == 0)
}
