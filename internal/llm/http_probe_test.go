package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeHTTPAppliesHeadersAndReturnsBody(t *testing.T) {
	var gotHeader, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-test")
		gotBody = readRequestBody(t, r)
		mustWriteResponse(t, w, `{"ok":true}`)
	}))
	defer srv.Close()

	body, err := EndpointHealthChecker{}.probeHTTP(context.Background(), http.MethodPost, srv.URL, strings.NewReader("payload"), map[string]string{"x-test": "value"})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if string(body) != `{"ok":true}` || gotHeader != "value" || gotBody != "payload" {
		t.Fatalf("body=%q header=%q request=%q", body, gotHeader, gotBody)
	}
}

func TestProbeHTTPReturnsStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}))
	defer srv.Close()

	_, err := EndpointHealthChecker{}.probeHTTP(context.Background(), http.MethodGet, srv.URL, nil, nil)
	var se *statusError
	if !errors.As(err, &se) || se.StatusCode != http.StatusTeapot || !strings.Contains(se.Body, "nope") {
		t.Fatalf("err = %#v", err)
	}
}
