// Package appserver exposes Gator's versioned local RPC protocol over a
// deliberately narrow loopback HTTP interface. It keeps the RPC server as the
// single execution authority rather than creating a second agent control path.
package appserver

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	internalrpc "github.com/gongahkia/gator/internal/rpc"
	protocol "github.com/gongahkia/gator/rpc"
)

const (
	maxRequestBytes        = 1024 * 1024
	defaultHistory         = 256
	defaultSubscriberQueue = 64
	defaultStreams         = 1024
	defaultStreamRetention = 15 * time.Minute
	heartbeatInterval      = 15 * time.Second
)

var (
	errDuplicateRequestID = errors.New("request id has already been submitted to this app server")
	errStreamCapacity     = errors.New("app server event-stream capacity is exhausted; retry later")
)

// Config supplies the RPC dependencies and local transport constraints. Token
// is a high-entropy printable bearer token owned by the local developer; the
// server never logs it or sends it to an agent.
type Config struct {
	RPC             internalrpc.Config
	Token           []byte
	Version         string
	MaxStreams      int
	MaxHistory      int
	SubscriberQueue int
}

// Server bridges HTTP requests to one in-process JSONL RPC server and stores a
// bounded replay window per request ID for reconnecting SSE clients.
type Server struct {
	backend *internalrpc.Server
	input   *io.PipeWriter
	hub     *messageHub
	token   []byte
	version string

	write     sync.Mutex
	mu        sync.Mutex
	started   bool
	stopped   bool
	cancel    context.CancelFunc
	done      chan struct{}
	serveErr  error
	closeOnce sync.Once
}

// New constructs an inactive bridge. Call Start before serving HTTP. RPC Input
// and Output are deliberately owned by this bridge so all transports observe
// the exact same protocol messages.
func New(config Config) (*Server, error) {
	if config.RPC.Input != nil || config.RPC.Output != nil {
		return nil, errors.New("app server owns RPC input and output")
	}
	token, err := validToken(config.Token)
	if err != nil {
		return nil, err
	}
	if config.MaxStreams <= 0 {
		config.MaxStreams = defaultStreams
	}
	if config.MaxHistory <= 0 {
		config.MaxHistory = defaultHistory
	}
	if config.SubscriberQueue <= 0 {
		config.SubscriberQueue = defaultSubscriberQueue
	}
	if config.Version == "" {
		config.Version = "dev"
	}
	reader, writer := io.Pipe()
	hub := newMessageHub(config.MaxStreams, config.MaxHistory, config.SubscriberQueue)
	config.RPC.Input = reader
	config.RPC.Output = hub
	backend, err := internalrpc.New(config.RPC)
	if err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return nil, err
	}
	return &Server{backend: backend, input: writer, hub: hub, token: token, version: config.Version, done: make(chan struct{})}, nil
}

func validToken(value []byte) ([]byte, error) {
	value = bytes.TrimSpace(value)
	if len(value) < 32 || len(value) > 4096 {
		return nil, errors.New("app server token must contain 32-4096 printable bytes")
	}
	for _, character := range value {
		if character < 0x21 || character > 0x7e {
			return nil, errors.New("app server token must contain 32-4096 printable bytes")
		}
	}
	return append([]byte(nil), value...), nil
}

// Start begins the private RPC loop once. It is safe to call Handler before
// Start, but all operational routes report not-ready until Start succeeds.
func (s *Server) Start(parent context.Context) error {
	if s == nil || s.backend == nil || s.input == nil {
		return errors.New("app server is not initialized")
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("app server is already started")
	}
	ctx, cancel := context.WithCancel(parent)
	s.started, s.cancel = true, cancel
	s.mu.Unlock()
	go func() {
		err := s.backend.Serve(ctx)
		s.mu.Lock()
		s.serveErr, s.stopped = err, true
		s.mu.Unlock()
		close(s.done)
	}()
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
	return nil
}

// Close stops request ingress, cancels active runs, and releases waiting SSE
// handlers through their request contexts. It is safe to call more than once.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	var closeErr error
	s.closeOnce.Do(func() {
		s.mu.Lock()
		cancel := s.cancel
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if s.input != nil {
			closeErr = s.input.Close()
		}
	})
	return closeErr
}

// Wait reports the RPC loop's terminal error after the server has stopped.
func (s *Server) Wait() error {
	if s == nil {
		return errors.New("app server is not initialized")
	}
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.serveErr
}

// Handler returns the HTTP handler for this local app-server instance.
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serveHTTP) }

func (s *Server) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Cache-Control", "no-store")
	if request.Header.Get("Origin") != "" {
		writeError(writer, http.StatusForbidden, "cross-origin requests are not accepted")
		return
	}
	switch {
	case request.URL.Path == "/healthz" || request.URL.Path == "/readyz":
		s.health(writer, request)
	case request.URL.Path == "/openapi.json":
		if !s.authorized(request) {
			writeUnauthorized(writer)
			return
		}
		s.openAPI(writer, request)
	case request.URL.Path == "/v1/rpc":
		if !s.authorized(request) {
			writeUnauthorized(writer)
			return
		}
		s.submitHTTP(writer, request)
	case strings.HasPrefix(request.URL.Path, "/v1/events/"):
		if !s.authorized(request) {
			writeUnauthorized(writer)
			return
		}
		s.eventsHTTP(writer, request)
	default:
		writeError(writer, http.StatusNotFound, "route not found")
	}
}

func (s *Server) health(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.mu.Lock()
	ready := s.started && !s.stopped && s.serveErr == nil
	s.mu.Unlock()
	if !ready {
		writeError(writer, http.StatusServiceUnavailable, "app server is not ready")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ready": true, "version": s.version, "protocol_version": protocol.Version})
}

func (s *Server) openAPI(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"openapi": "3.1.0",
		"info":    map[string]string{"title": "Gator local app server", "version": s.version},
		"paths": map[string]any{
			"/healthz":        map[string]any{"get": map[string]string{"summary": "Read local readiness without authentication"}},
			"/v1/rpc":         map[string]any{"post": map[string]any{"summary": "Submit one Gator RPC v1 request", "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{}}}}},
			"/v1/events/{id}": map[string]any{"get": map[string]any{"summary": "Replay and stream correlated RPC messages as SSE", "parameters": []map[string]any{{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}}, {"name": "Last-Event-ID", "in": "header", "required": false, "schema": map[string]string{"type": "integer"}}}}},
		},
	})
}

func (s *Server) submitHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(writer, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}
	var message protocol.Request
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxRequestBytes))
	if err := decoder.Decode(&message); err != nil {
		writeError(writer, http.StatusBadRequest, "request must be valid JSON")
		return
	}
	if err := ensureEOF(decoder); err != nil {
		writeError(writer, http.StatusBadRequest, "request must contain one JSON object")
		return
	}
	if !protocol.ValidID(message.ID) {
		writeError(writer, http.StatusBadRequest, "request id must contain 1-128 letters, digits, '.', '_', or '-'")
		return
	}
	if err := s.submit(message); err != nil {
		switch {
		case errors.Is(err, errDuplicateRequestID):
			writeError(writer, http.StatusConflict, err.Error())
		case errors.Is(err, errStreamCapacity):
			writeError(writer, http.StatusTooManyRequests, err.Error())
		default:
			writeError(writer, http.StatusServiceUnavailable, err.Error())
		}
		return
	}
	events := "/v1/events/" + url.PathEscape(message.ID)
	writer.Header().Set("Location", events)
	writeJSON(writer, http.StatusAccepted, map[string]any{"id": message.ID, "events": events})
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func (s *Server) submit(message protocol.Request) error {
	s.mu.Lock()
	ready := s.started && !s.stopped && s.serveErr == nil
	s.mu.Unlock()
	if !ready {
		return errors.New("app server is not ready")
	}
	if err := s.hub.reserve(message.ID); err != nil {
		return err
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode RPC request: %w", err)
	}
	if len(payload) > maxRequestBytes {
		return errors.New("request exceeds 1 MiB")
	}
	s.write.Lock()
	defer s.write.Unlock()
	if _, err := s.input.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("submit RPC request: %w", err)
	}
	return nil
}

func (s *Server) eventsHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id, err := url.PathUnescape(strings.TrimPrefix(request.URL.Path, "/v1/events/"))
	if err != nil || !protocol.ValidID(id) {
		writeError(writer, http.StatusBadRequest, "event stream id is invalid")
		return
	}
	after, err := eventCursor(request.Header.Get("Last-Event-ID"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "Last-Event-ID must be a non-negative integer")
		return
	}
	history, subscription, ok := s.hub.subscribe(id, after)
	if !ok {
		writeError(writer, http.StatusNotFound, "event stream was not found")
		return
	}
	defer subscription.cancel()
	flusher, ok := writer.(http.Flusher)
	if !ok {
		writeError(writer, http.StatusInternalServerError, "streaming response is unavailable")
		return
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Connection", "keep-alive")
	writer.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(writer, ": gator event stream\n\n")
	flusher.Flush()
	for _, item := range history {
		if err := writeSSE(writer, item); err != nil {
			return
		}
	}
	flusher.Flush()
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case item := <-subscription.messages:
			if err := writeSSE(writer, item); err != nil {
				return
			}
			flusher.Flush()
		case <-subscription.dropped:
			_, _ = io.WriteString(writer, "event: overflow\ndata: {\"error\":\"event subscriber overflow; reconnect with Last-Event-ID\"}\n\n")
			flusher.Flush()
			return
		case <-heartbeat.C:
			_, _ = io.WriteString(writer, ": heartbeat\n\n")
			flusher.Flush()
		case <-request.Context().Done():
			return
		}
	}
}

func eventCursor(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	return strconv.ParseUint(value, 10, 64)
}

func writeSSE(writer io.Writer, item storedMessage) error {
	payload, err := json.Marshal(item.Message)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "id: %d\nevent: message\ndata: %s\n\n", item.Sequence, payload)
	return err
}

func (s *Server) authorized(request *http.Request) bool {
	value := request.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	provided := []byte(strings.TrimPrefix(value, prefix))
	if len(provided) != len(s.token) {
		return false
	}
	return subtle.ConstantTimeCompare(provided, s.token) == 1
}

func writeUnauthorized(writer http.ResponseWriter) {
	writer.Header().Set("WWW-Authenticate", `Bearer realm="gator-local"`)
	writeError(writer, http.StatusUnauthorized, "a valid bearer token is required")
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]any{"error": message})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

type storedMessage struct {
	Sequence uint64
	Message  protocol.Message
}

type subscription struct {
	messages chan storedMessage
	dropped  chan struct{}
	cancel   func()
}

type messageHub struct {
	mu              sync.Mutex
	streams         map[string]*messageStream
	maxStreams      int
	maxHistory      int
	subscriberQueue int
	retention       time.Duration
	now             func() time.Time
}

type messageStream struct {
	next         uint64
	history      []storedMessage
	subscribers  map[uint64]*subscription
	nextSub      uint64
	terminal     bool
	lastActivity time.Time
}

func newMessageHub(maxStreams, maxHistory, subscriberQueue int) *messageHub {
	return newMessageHubWithClock(maxStreams, maxHistory, subscriberQueue, defaultStreamRetention, time.Now)
}

func newMessageHubWithClock(maxStreams, maxHistory, subscriberQueue int, retention time.Duration, now func() time.Time) *messageHub {
	return &messageHub{
		streams: make(map[string]*messageStream), maxStreams: maxStreams, maxHistory: maxHistory, subscriberQueue: subscriberQueue,
		retention: retention, now: now,
	}
}

func (h *messageHub) Write(value []byte) (int, error) {
	var message protocol.Message
	if err := json.Unmarshal(value, &message); err == nil && protocol.ValidID(message.ID) {
		h.publish(message)
	}
	return len(value), nil
}

func (h *messageHub) reserve(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pruneLocked(h.now())
	if _, found := h.streams[id]; found {
		return errDuplicateRequestID
	}
	if len(h.streams) >= h.maxStreams {
		return errStreamCapacity
	}
	h.streams[id] = &messageStream{subscribers: make(map[uint64]*subscription), lastActivity: h.now()}
	return nil
}

func (h *messageHub) publish(message protocol.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	stream, found := h.streams[message.ID]
	if !found {
		return
	}
	stream.lastActivity = h.now()
	stream.next++
	item := storedMessage{Sequence: stream.next, Message: message}
	if len(stream.history) == h.maxHistory {
		copy(stream.history, stream.history[1:])
		stream.history[len(stream.history)-1] = item
	} else {
		stream.history = append(stream.history, item)
	}
	for id, subscriber := range stream.subscribers {
		select {
		case subscriber.messages <- item:
		default:
			delete(stream.subscribers, id)
			close(subscriber.dropped)
		}
	}
	if terminalMessage(message) {
		stream.terminal = true
	}
}

func (h *messageHub) subscribe(id string, after uint64) ([]storedMessage, *subscription, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	stream, found := h.streams[id]
	if !found {
		return nil, nil, false
	}
	stream.lastActivity = h.now()
	history := make([]storedMessage, 0, len(stream.history))
	for _, item := range stream.history {
		if item.Sequence > after {
			history = append(history, item)
		}
	}
	stream.nextSub++
	subscriberID := stream.nextSub
	subscription := &subscription{messages: make(chan storedMessage, h.subscriberQueue), dropped: make(chan struct{})}
	stream.subscribers[subscriberID] = subscription
	subscription.cancel = func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if current := stream.subscribers[subscriberID]; current == subscription {
			delete(stream.subscribers, subscriberID)
			stream.lastActivity = h.now()
		}
	}
	return history, subscription, true
}

func (h *messageHub) pruneLocked(now time.Time) {
	if h.retention <= 0 {
		return
	}
	for id, stream := range h.streams {
		if stream.terminal && len(stream.subscribers) == 0 && now.Sub(stream.lastActivity) >= h.retention {
			delete(h.streams, id)
		}
	}
}

func terminalMessage(message protocol.Message) bool {
	if message.Type == "error" {
		return true
	}
	if message.Type != "response" {
		return false
	}
	if result, ok := message.Result.(map[string]any); ok {
		if accepted, found := result["accepted"].(bool); found && accepted {
			return false
		}
	}
	return true
}
