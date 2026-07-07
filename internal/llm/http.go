package llm

import (
	"net/http"
	"time"
)

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func timeoutOrDefault(timeouts []time.Duration, fallback time.Duration) time.Duration {
	if len(timeouts) > 0 && timeouts[0] > 0 {
		return timeouts[0]
	}
	return fallback
}
