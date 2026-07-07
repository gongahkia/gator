package llm

import (
	"context"
	"io"
	"net/http"
	"time"
)

const healthHTTPTimeout = 10 * time.Second

func (c EndpointHealthChecker) probeHTTP(ctx context.Context, method, url string, body io.Reader, headers map[string]string) ([]byte, error) {
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: healthHTTPTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &statusError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return respBody, nil
}
