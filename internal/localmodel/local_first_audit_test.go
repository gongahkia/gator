package localmodel

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type localAuditTransport struct{ hosts []string }

func (t *localAuditTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.hosts = append(t.hosts, r.URL.Host)
	if len(t.hosts) == 1 {
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://off-host.invalid/api/version"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"version":"fixture"}`)), Request: r}, nil
}

// No socket or DNS operation occurs: the fake transport records redirect URLs.
func TestLocalFirstAuditLoopbackClientFollowsAnOffHostRedirect(t *testing.T) {
	c, err := NewClient("http://127.0.0.1:11434")
	if err != nil {
		t.Fatal(err)
	}
	transport := &localAuditTransport{}
	testClient := *c.client
	testClient.Transport = transport
	c.client = &testClient
	v, err := c.Version(context.Background())
	if err != nil || v != "fixture" || len(transport.hosts) != 2 || transport.hosts[1] != "off-host.invalid" {
		t.Fatal("redirect reproduction changed; re-evaluate LF-03", err)
	}
	t.Log("LF-03 reproduced without network: initial loopback validation does not constrain redirected requests")
}
