package tools

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/sandbox"
)

func TestHTTPFetchRequiresNetworkApprovalAndReturnsBoundedText(t *testing.T) {
	if tools := HTTPTools(CommandPolicy{Sandbox: sandbox.Policy{Network: sandbox.DenyNetwork}}, HTTPFetchOptions{}); len(tools) != 0 {
		t.Fatalf("network-denied tool surface = %#v", tools)
	}
	client := &fakeHTTPClient{status: http.StatusOK, contentType: "text/plain; charset=utf-8", body: "abcdef"}
	var approvals [][]string
	var events []agent.Event
	memory := NewCommandMemory(nil)
	tools := HTTPTools(CommandPolicy{
		Sandbox:    sandbox.Policy{Network: sandbox.AllowNetwork},
		Remembered: memory,
		Approve: func(_ context.Context, argv []string) (CommandDecision, error) {
			approvals = append(approvals, append([]string(nil), argv...))
			return CommandAllowAlways, nil
		},
		OnEvent: func(event agent.Event) { events = append(events, event) },
	}, HTTPFetchOptions{Resolver: staticResolver{"example.test": {{IP: net.ParseIP("93.184.216.34")}}}, Client: client, MaxResponseBytes: 4})
	if len(tools) != 5 || tools[0].Definition().Name != "http_fetch" || tools[1].Definition().Name != "browser_navigate" {
		t.Fatalf("network-allowed tool surface = %#v", tools)
	}
	first := executeTool(t, tools[0], `{"url":"https://example.test/research?q=gator"}`)
	if !strings.Contains(first, `"body":"abcd"`) || !strings.Contains(first, `"truncated":true`) || !strings.Contains(first, `"status":200`) {
		t.Fatalf("HTTP fetch result = %s", first)
	}
	if len(approvals) != 1 || strings.Join(approvals[0], " ") != "http_fetch https://example.test/research?q=gator" {
		t.Fatalf("HTTP fetch approval = %#v", approvals)
	}
	if len(events) != 2 || events[0].Kind != agent.EventCommandApprovalRequested || events[1].Kind != agent.EventCommandApprovalResolved {
		t.Fatalf("HTTP fetch approval events = %#v", events)
	}
	_ = executeTool(t, tools[0], `{"url":"https://example.test/research?q=gator"}`)
	if len(approvals) != 1 || client.calls != 2 {
		t.Fatalf("remembered URL approval = %#v, calls = %d", approvals, client.calls)
	}
	if memory.Allows([]string{"http_fetch", "https://example.test/research?q=gator"}) {
		t.Fatal("HTTP allow-always decision leaked into the command allowlist")
	}
}

func TestWebSearchRequiresConfiguredKeyAndReturnsBoundedResults(t *testing.T) {
	if tools := HTTPTools(CommandPolicy{Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork}}, HTTPFetchOptions{}); len(tools) != 5 || tools[0].Definition().Name != "http_fetch" {
		t.Fatalf("web search appeared without a configured key: %#v", tools)
	}
	if tools := HTTPTools(CommandPolicy{Sandbox: sandbox.Policy{Network: sandbox.DenyNetwork}}, HTTPFetchOptions{BraveSearchAPIKey: "do-not-leak"}); len(tools) != 0 {
		t.Fatalf("network-denied web search surface = %#v", tools)
	}
	const token = "do-not-leak"
	client := &fakeHTTPClient{contentType: "application/json; charset=utf-8", body: `{"web":{"results":[{"title":"One","url":"https://one.example/","description":"first result"},{"title":"Two","url":"https://two.example/","description":"second result"},{"title":"Three","url":"https://three.example/"}]}}`}
	var approvals [][]string
	var events []agent.Event
	tools := HTTPTools(CommandPolicy{
		Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork},
		Approve: func(_ context.Context, argv []string) (CommandDecision, error) {
			approvals = append(approvals, append([]string(nil), argv...))
			return CommandAllowAlways, nil
		},
		OnEvent: func(event agent.Event) { events = append(events, event) },
	}, HTTPFetchOptions{
		BraveSearchAPIKey: token,
		Resolver:          staticResolver{"api.search.brave.com": {{IP: net.ParseIP("93.184.216.34")}}},
		Client:            client,
	})
	if len(tools) != 6 || tools[1].Definition().Name != "web_search" {
		t.Fatalf("web search tool surface = %#v", tools)
	}
	first := executeTool(t, tools[1], `{"query":"Gator web research","count":2}`)
	if !strings.Contains(first, `"query":"Gator web research"`) || !strings.Contains(first, `"title":"One"`) || !strings.Contains(first, `"title":"Two"`) || strings.Contains(first, `"title":"Three"`) || strings.Contains(first, token) {
		t.Fatalf("web search result = %s", first)
	}
	if len(approvals) != 1 || strings.Join(approvals[0], " ") != "web_search Gator web research" {
		t.Fatalf("web search approval = %#v", approvals)
	}
	if client.lastRequest == nil || client.lastRequest.URL.Host != "api.search.brave.com" || client.lastRequest.URL.Query().Get("q") != "Gator web research" || client.lastRequest.URL.Query().Get("count") != "2" || client.lastRequest.Header.Get("X-Subscription-Token") != token {
		t.Fatalf("web search request = %#v", client.lastRequest)
	}
	for _, event := range events {
		if strings.Contains(event.Text, token) || strings.Contains(strings.Join(event.Argv, " "), token) {
			t.Fatalf("web search token leaked into event: %#v", event)
		}
	}
	_ = executeTool(t, tools[1], `{"query":"Gator web research","count":1}`)
	if len(approvals) != 1 || client.calls != 2 {
		t.Fatalf("remembered web search approval = %#v, calls=%d", approvals, client.calls)
	}
}

func TestHTTPFetchRejectsPrivateMixedAndNonTextTargetsBeforeConnection(t *testing.T) {
	tests := []struct {
		name     string
		resolver HostResolver
		client   *fakeHTTPClient
		want     string
	}{
		{
			name:     "private address",
			resolver: staticResolver{"private.test": {{IP: net.ParseIP("127.0.0.1")}}},
			client:   &fakeHTTPClient{body: "should not fetch"},
			want:     "private or reserved",
		},
		{
			name: "mixed DNS response",
			resolver: staticResolver{"mixed.test": {
				{IP: net.ParseIP("93.184.216.34")},
				{IP: net.ParseIP("10.0.0.8")},
			}},
			client: &fakeHTTPClient{body: "should not fetch"},
			want:   "private or reserved",
		},
		{
			name:     "binary response",
			resolver: staticResolver{"binary.test": {{IP: net.ParseIP("93.184.216.34")}}},
			client:   &fakeHTTPClient{contentType: "application/octet-stream", body: "bytes"},
			want:     "non-text",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			approvals := 0
			tools := HTTPTools(CommandPolicy{
				Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork},
				Approve: func(context.Context, []string) (CommandDecision, error) {
					approvals++
					return CommandAllowOnce, nil
				},
			}, HTTPFetchOptions{Resolver: test.resolver, Client: test.client})
			_, err := tools[0].Execute(context.Background(), []byte(`{"url":"https://`+strings.Split(test.name, " ")[0]+`.test/"}`))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("HTTP fetch error = %v, want %q", err, test.want)
			}
			if test.name != "binary response" && (approvals != 1 || test.client.calls != 0) {
				t.Fatalf("unsafe target reached the client: approvals=%d calls=%d", approvals, test.client.calls)
			}
		})
	}
}

func TestHTTPFetchReportsRedirectWithoutFollowingIt(t *testing.T) {
	client := &fakeHTTPClient{status: http.StatusFound, contentType: "text/html", body: "redirecting", location: "https://other.example/"}
	tools := HTTPTools(CommandPolicy{
		Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork},
		Approve: allowOnce,
	}, HTTPFetchOptions{Resolver: staticResolver{"redirect.test": {{IP: net.ParseIP("93.184.216.34")}}}, Client: client})
	result := executeTool(t, tools[0], `{"url":"https://redirect.test/start"}`)
	if !strings.Contains(result, `"status":302`) || !strings.Contains(result, `"redirect":"https://other.example/"`) || client.calls != 1 {
		t.Fatalf("redirect result = %s, calls = %d", result, client.calls)
	}
}

func TestNormalizeHTTPURLAndPublicAddressValidation(t *testing.T) {
	for _, value := range []string{"http://example.com", "https://localhost/", "https://example.com:8443/", "https://user@example.com/"} {
		if _, _, err := normalizeHTTPURL(value); err == nil {
			t.Fatalf("unsafe URL accepted: %q", value)
		}
	}
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "100.64.0.1", "192.0.2.1", "2001:db8::1"} {
		if isPublicHTTPAddress(net.ParseIP(address)) {
			t.Fatalf("private or reserved address accepted: %s", address)
		}
	}
	if !isPublicHTTPAddress(net.ParseIP("93.184.216.34")) {
		t.Fatal("public address rejected")
	}
}

type staticResolver map[string][]net.IPAddr

func (r staticResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	addresses, found := r[host]
	if !found {
		return nil, &net.DNSError{Err: "not found", Name: host}
	}
	return append([]net.IPAddr(nil), addresses...), nil
}

type fakeHTTPClient struct {
	status      int
	contentType string
	body        string
	location    string
	calls       int
	lastRequest *http.Request
}

func (c *fakeHTTPClient) Do(request *http.Request) (*http.Response, error) {
	c.calls++
	c.lastRequest = request.Clone(request.Context())
	status := c.status
	if status == 0 {
		status = http.StatusOK
	}
	header := make(http.Header)
	if c.contentType != "" {
		header.Set("Content-Type", c.contentType)
	}
	if c.location != "" {
		header.Set("Location", c.location)
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(c.body)), Request: request}, nil
}
