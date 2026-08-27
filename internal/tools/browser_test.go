package tools

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/sandbox"
)

func TestBoundedBrowserNavigatesSnapshotsAndExtractsWithoutSubresources(t *testing.T) {
	client := &browserScriptClient{responses: []browserResponse{{
		status: http.StatusOK, contentType: "text/html",
		body: `<html><head><title>Example page</title><script>fetch("https://private.test/")</script></head>
<body><h1>Public research</h1>
<a href="/next">Next result</a><a href="http://insecure.test/">unsafe</a>
<form action="/search" method="get"><input name="q"><input type="password" name="secret"></form></body></html>`,
	}}}
	var approvals [][]string
	var events []agent.Event
	surface := HTTPTools(CommandPolicy{
		Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork},
		Approve: func(_ context.Context, argv []string) (CommandDecision, error) {
			approvals = append(approvals, append([]string(nil), argv...))
			return CommandAllowOnce, nil
		},
		OnEvent: func(event agent.Event) { events = append(events, event) },
	}, HTTPFetchOptions{
		Resolver: staticResolver{"example.test": {{IP: net.ParseIP("93.184.216.34")}}},
		Client:   client,
	})
	navigate := toolNamed(t, surface, "browser_navigate")
	snapshot := toolNamed(t, surface, "browser_snapshot")
	extract := toolNamed(t, surface, "browser_extract")

	result := executeTool(t, navigate, `{"url":"https://example.test/start"}`)
	if !strings.Contains(result, `"title":"Example page"`) || !strings.Contains(result, `"links":1`) || !strings.Contains(result, `"forms":1`) {
		t.Fatalf("navigate result = %s", result)
	}
	snapshotResult := executeTool(t, snapshot, `{}`)
	if !strings.Contains(snapshotResult, "Public research") || !strings.Contains(snapshotResult, `"ref":"link-1"`) || strings.Contains(snapshotResult, `fetch(`) || strings.Contains(snapshotResult, "insecure.test") {
		t.Fatalf("snapshot result = %s", snapshotResult)
	}
	extracted := executeTool(t, extract, `{"kind":"links","query":"next"}`)
	if !strings.Contains(extracted, `"url":"https://example.test/next"`) {
		t.Fatalf("extract result = %s", extracted)
	}
	if len(approvals) != 1 || strings.Join(approvals[0], " ") != "browser get https://example.test/start" || client.calls != 1 {
		t.Fatalf("browser approvals=%#v calls=%d", approvals, client.calls)
	}
	if len(events) != 2 || events[0].Kind != agent.EventCommandApprovalRequested || events[1].Kind != agent.EventCommandApprovalResolved {
		t.Fatalf("browser events = %#v", events)
	}
}

func TestBoundedBrowserActionsRequireFreshApprovalAndRejectSecretFields(t *testing.T) {
	client := &browserScriptClient{responses: []browserResponse{
		{status: http.StatusOK, contentType: "text/html", body: `<a href="/next">Next</a><form action="/submit" method="post"><input name="q"><input name="password" type="password"></form>`},
		{status: http.StatusOK, contentType: "text/html", body: `<title>Next page</title><p>done</p>`},
	}}
	var approvals [][]string
	surface := HTTPTools(CommandPolicy{
		Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork},
		Approve: func(_ context.Context, argv []string) (CommandDecision, error) {
			approvals = append(approvals, append([]string(nil), argv...))
			return CommandAllowOnce, nil
		},
	}, HTTPFetchOptions{
		Resolver: staticResolver{"example.test": {{IP: net.ParseIP("93.184.216.34")}}},
		Client:   client,
	})
	navigate := toolNamed(t, surface, "browser_navigate")
	act := toolNamed(t, surface, "browser_act")
	_ = executeTool(t, navigate, `{"url":"https://example.test/"}`)
	if _, err := act.Execute(context.Background(), []byte(`{"action":"submit","ref":"form-1","fields":{"password":"do-not-send"}}`)); err == nil || !strings.Contains(err.Error(), "refuses password") {
		t.Fatalf("secret-field action error = %v", err)
	}
	result := executeTool(t, act, `{"action":"click","ref":"link-1"}`)
	if !strings.Contains(result, `"title":"Next page"`) {
		t.Fatalf("click result = %s", result)
	}
	if len(approvals) != 2 || strings.Join(approvals[1], " ") != "browser get https://example.test/next" || client.calls != 2 {
		t.Fatalf("action approvals=%#v calls=%d", approvals, client.calls)
	}
}

func TestBoundedBrowserRejectsPrivateTargetsAndDoesNotFollowRedirects(t *testing.T) {
	privateClient := &browserScriptClient{responses: []browserResponse{{status: http.StatusOK, contentType: "text/html", body: "unsafe"}}}
	privateSurface := HTTPTools(CommandPolicy{
		Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork}, Approve: allowOnce,
	}, HTTPFetchOptions{
		Resolver: staticResolver{"private.test": {{IP: net.ParseIP("127.0.0.1")}}},
		Client:   privateClient,
	})
	if _, err := toolNamed(t, privateSurface, "browser_navigate").Execute(context.Background(), []byte(`{"url":"https://private.test/"}`)); err == nil || !strings.Contains(err.Error(), "private or reserved") {
		t.Fatalf("private browser target error = %v", err)
	}
	if privateClient.calls != 0 {
		t.Fatalf("private browser target reached client %d times", privateClient.calls)
	}

	redirectClient := &browserScriptClient{responses: []browserResponse{{
		status: http.StatusFound, contentType: "text/html", location: "https://private.test/", body: "redirect",
	}}}
	redirectSurface := HTTPTools(CommandPolicy{
		Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork}, Approve: allowOnce,
	}, HTTPFetchOptions{
		Resolver: staticResolver{"public.test": {{IP: net.ParseIP("93.184.216.34")}}},
		Client:   redirectClient,
	})
	result := executeTool(t, toolNamed(t, redirectSurface, "browser_navigate"), `{"url":"https://public.test/"}`)
	if !strings.Contains(result, `"redirect":"https://private.test/"`) || !strings.Contains(result, `"loaded":false`) || redirectClient.calls != 1 {
		t.Fatalf("redirect result=%s calls=%d", result, redirectClient.calls)
	}
	if _, err := toolNamed(t, redirectSurface, "browser_snapshot").Execute(context.Background(), []byte(`{}`)); err == nil || !strings.Contains(err.Error(), "no loaded document") {
		t.Fatalf("redirect unexpectedly replaced browser state: %v", err)
	}
}

func TestBoundedBrowserSubmitsApprovedPOSTWithoutLeakingFieldValuesToApproval(t *testing.T) {
	client := &browserScriptClient{responses: []browserResponse{
		{status: http.StatusOK, contentType: "text/html", body: `<form action="/submit" method="post"><input name="q"></form>`},
		{status: http.StatusOK, contentType: "text/html", body: `<title>Submitted</title><p>accepted</p>`},
	}}
	var approvals [][]string
	surface := HTTPTools(CommandPolicy{
		Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork},
		Approve: func(_ context.Context, argv []string) (CommandDecision, error) {
			approvals = append(approvals, append([]string(nil), argv...))
			return CommandAllowOnce, nil
		},
	}, HTTPFetchOptions{
		Resolver: staticResolver{"example.test": {{IP: net.ParseIP("93.184.216.34")}}},
		Client:   client,
	})
	_ = executeTool(t, toolNamed(t, surface, "browser_navigate"), `{"url":"https://example.test/"}`)
	result := executeTool(t, toolNamed(t, surface, "browser_act"), `{"action":"submit","ref":"form-1","fields":{"q":"sensitive model supplied value"}}`)
	if !strings.Contains(result, `"title":"Submitted"`) || client.lastBody != "q=sensitive+model+supplied+value" {
		t.Fatalf("POST result=%s body=%q", result, client.lastBody)
	}
	if len(approvals) != 2 || strings.Join(approvals[1], " ") != "browser post https://example.test/submit" || strings.Contains(strings.Join(approvals[1], " "), "sensitive") {
		t.Fatalf("POST approval leaked fields: %#v", approvals)
	}
}

func toolNamed(t *testing.T, surface []agent.Tool, name string) agent.Tool {
	t.Helper()
	for _, tool := range surface {
		if tool.Definition().Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}

type browserResponse struct {
	status      int
	contentType string
	body        string
	location    string
}

type browserScriptClient struct {
	responses   []browserResponse
	calls       int
	lastRequest *http.Request
	lastBody    string
}

func (c *browserScriptClient) Do(request *http.Request) (*http.Response, error) {
	c.calls++
	c.lastRequest = request.Clone(request.Context())
	if request.Body != nil {
		body, _ := io.ReadAll(request.Body)
		c.lastBody = string(body)
	}
	index := c.calls - 1
	if index >= len(c.responses) {
		return nil, errors.New("unexpected browser request")
	}
	item := c.responses[index]
	header := make(http.Header)
	header.Set("Content-Type", item.contentType)
	if item.location != "" {
		header.Set("Location", item.location)
	}
	return &http.Response{
		StatusCode: item.status, Header: header, Body: io.NopCloser(strings.NewReader(item.body)), Request: request,
	}, nil
}
