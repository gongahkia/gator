package agent

import "testing"

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
