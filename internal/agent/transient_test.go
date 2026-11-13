package agent

import (
	"errors"
	"fmt"
	"testing"
)

func TestHTTPStatusErrorMarksOnlyRetryableStatuses(t *testing.T) {
	permanent := HTTPStatusError(400, errString("HTTP 400: invalid_request"))
	if IsTransient(permanent) {
		t.Fatalf("400 marked transient: %v", permanent)
	}
	tooMany := HTTPStatusError(429, errString("HTTP 429: rate limit reached"))
	if !IsTransient(tooMany) || tooMany.Error() != "HTTP 429: rate limit reached" {
		t.Fatalf("429 = %v", tooMany)
	}
	unavailable := HTTPStatusError(503, errString("HTTP 503"))
	if !IsTransient(unavailable) {
		t.Fatalf("503 not transient: %v", unavailable)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestProviderStatusSurvivesErrorWrappingWithoutLosingDiagnostics(t *testing.T) {
	diagnostic := errors.New("original diagnostic")
	for _, code := range []int{400, 401, 402, 403, 404, 429, 503} {
		wrapped := fmt.Errorf("turn failed: %w", HTTPStatusError(code, diagnostic))
		var status ProviderHTTPError
		if !errors.As(wrapped, &status) || status.Status != code || !errors.Is(wrapped, diagnostic) {
			t.Fatal(code, wrapped)
		}
	}
	if HTTPStatusError(401, nil) != nil {
		t.Fatal("nil failure was invented")
	}
}
