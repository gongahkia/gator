package llm

import (
	"context"
	"errors"
	"net"
	"net/url"
	"time"
)

var retryBackoffs = []time.Duration{250 * time.Millisecond, time.Second}

type retryClient struct {
	inner   Client
	timeout time.Duration
}

func NewRetryClient(inner Client, timeout time.Duration) Client {
	return &retryClient{inner: inner, timeout: timeout}
}

func (c *retryClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= len(retryBackoffs); attempt++ {
		attemptCtx := ctx
		cancel := func() {}
		if c.timeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, c.timeout)
		}
		resp, err := c.inner.Chat(attemptCtx, req)
		cancel()
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if ctx.Err() != nil || attempt == len(retryBackoffs) || !retryable(err) {
			return nil, err
		}
		timer := time.NewTimer(retryBackoffs[attempt])
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func retryable(err error) bool {
	var se *statusError
	if errors.As(err, &se) {
		return se.StatusCode == 429 || se.StatusCode >= 500
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
