package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"sync"
)

type httpStatusError struct{ Code int }

func (e *httpStatusError) Error() string { return fmt.Sprintf("MCP server returned HTTP %d", e.Code) }

type httpClient struct {
	endpoint string
	client   *http.Client
	access   string
	gate     chan struct{}
	done     chan struct{}
	once     sync.Once
	cancel   context.CancelFunc
	lifetime context.Context
	mu       sync.Mutex
	version  string
	next     int64
	session  string
	headers  map[string][]headerBinding
}

func newHTTPClient(endpoint string) *httpClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &httpClient{endpoint: endpoint, client: &http.Client{Timeout: callTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, gate: make(chan struct{}, 1), done: make(chan struct{}), lifetime: ctx, cancel: cancel, version: modernVersion}
}
func newAuthorizedHTTPClient(endpoint, access string) *httpClient {
	c := newHTTPClient(endpoint)
	c.access = access
	return c
}
func (c *httpClient) setProtocol(version string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.version = version
	c.session = ""
}
func (c *httpClient) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	if err := acquire(ctx, c.gate, c.done); err != nil {
		return nil, err
	}
	defer func() { <-c.gate }()
	c.mu.Lock()
	version := c.version
	c.mu.Unlock()
	c.next++
	id := c.next
	raw, err := c.exchange(ctx, method, params, &id, version)
	if err == nil {
		err = normalizeResult(raw, version)
	}
	return raw, err
}
func (c *httpClient) Notify(ctx context.Context, method string, params any) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	if err := acquire(ctx, c.gate, c.done); err != nil {
		return err
	}
	defer func() { <-c.gate }()
	c.mu.Lock()
	version := c.version
	c.mu.Unlock()
	_, err := c.exchange(ctx, method, params, nil, version)
	return err
}
func (c *httpClient) exchange(ctx context.Context, method string, params any, id *int64, version string) (json.RawMessage, error) {
	raw, err := marshalRPC(method, params, id, version)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(c.lifetime, cancel)
	defer stop()
	if c.lifetime.Err() != nil {
		cancel()
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("MCP endpoint cannot form a valid HTTP request")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", version)
	if c.access != "" {
		request.Header.Set("Authorization", "Bearer "+c.access)
	}
	c.mu.Lock()
	session := c.session
	c.mu.Unlock()
	if version == protocolVersion && session != "" {
		request.Header.Set("Mcp-Session-Id", session)
	}
	if version == modernVersion {
		request.Header.Set("Mcp-Method", method)
		if method == "tools/call" {
			var wire struct {
				Params struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"params"`
			}
			if json.Unmarshal(raw, &wire) != nil {
				return nil, errors.New("invalid MCP call parameters")
			}
			p := wire.Params
			request.Header.Set("Mcp-Name", encodeHeader(p.Name))
			c.mu.Lock()
			bindings := c.headers[p.Name]
			c.mu.Unlock()
			if err := applyHeaders(request.Header, bindings, p.Arguments); err != nil {
				return nil, err
			}
		}
	}
	response, err := c.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("MCP HTTP request cancelled; delivery may be unknown: %w", ctx.Err())
		}
		return nil, errors.New("MCP HTTP request failed; delivery may be unknown")
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		if response.StatusCode == http.StatusUnauthorized {
			return nil, errors.New("MCP authentication is missing or expired; authenticate the trusted server again")
		}
		if response.StatusCode == 400 || response.StatusCode == 404 {
			if id == nil {
				return nil, &httpStatusError{Code: response.StatusCode}
			}
			// Modern validation errors may legitimately omit the request ID.
			body, readErr := io.ReadAll(io.LimitReader(response.Body, maxWire+1))
			if readErr != nil || len(body) > maxWire {
				return nil, errors.New("MCP HTTP error response is unreadable or oversized")
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal(body, &fields) == nil && fields["error"] != nil && (len(fields["id"]) == 0 || string(fields["id"]) == "null") {
				fields["id"] = json.RawMessage(strconv.FormatInt(*id, 10))
				body, _ = json.Marshal(fields)
			}
			_, rpcErr := parseRPCResponse(body, *id)
			var rpc *rpcError
			if errors.As(rpcErr, &rpc) {
				return nil, rpc
			}
		}
		return nil, &httpStatusError{Code: response.StatusCode}
	}
	if id == nil {
		if response.StatusCode != http.StatusAccepted {
			return nil, errors.New("MCP notification was not acknowledged with HTTP 202")
		}
		return nil, nil
	}
	if version == protocolVersion {
		if session := response.Header.Get("Mcp-Session-Id"); session != "" {
			if len(session) > 1024 {
				return nil, errors.New("MCP session identity exceeds its bound")
			}
			c.mu.Lock()
			c.session = session
			c.mu.Unlock()
		}
	}
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil {
		return nil, errors.New("MCP response has no supported content type")
	}
	switch media {
	case "text/event-stream":
		return readSSEResponse(ctx, response.Body, *id)
	case "application/json":
		return readJSONResponse(response.Body, *id)
	default:
		return nil, errors.New("MCP response has an unsupported content type")
	}
}
func (c *httpClient) Close() error {
	c.once.Do(func() { close(c.done); c.cancel(); c.client.CloseIdleConnections() })
	return nil
}
