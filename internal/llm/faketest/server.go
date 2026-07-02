package faketest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

type Server struct {
	*httptest.Server
	mu        sync.Mutex
	responses []Response
	requests  []Request
}

type Response struct {
	Match  string
	Status int
	Body   string
}

type Request struct {
	Path   string
	Header http.Header
	Body   string
}

func NewServer() *Server {
	s := &Server{}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

func (s *Server) Respond(match string, status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses = append(s.responses, Response{Match: match, Status: status, Body: body})
}

func (s *Server) RespondOpenAI(match, content string) {
	s.Respond(match, http.StatusOK, mustJSON(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"content": content}},
		},
		"usage": map[string]any{
			"prompt_tokens":     11,
			"completion_tokens": 7,
		},
	}))
}

func (s *Server) RespondOllama(match, content string) {
	s.Respond(match, http.StatusOK, mustJSON(map[string]any{
		"message": map[string]any{
			"content": content,
		},
		"prompt_eval_count": 13,
		"eval_count":        5,
	}))
}

func (s *Server) RespondInvalidJSON(match string) {
	s.Respond(match, http.StatusOK, "{")
}

func (s *Server) LastRequest() Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) == 0 {
		return Request{}
	}
	return s.requests[len(s.requests)-1]
}

func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, len(s.requests))
	copy(out, s.requests)
	return out
}

func HallucinatedDigest() string {
	return `{"summary":"hallucinated","items":[{"unit_id":"missing","path":"ghost.go","relevance":100,"spans":[{"start_line":1,"end_line":1,"quote":"not in raw context"}]}]}`
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/chat/completions" && r.URL.Path != "/api/chat" {
		http.NotFound(w, r)
		return
	}
	body := readBody(r)
	s.record(Request{Path: r.URL.Path, Header: r.Header.Clone(), Body: body})
	resp := s.match(body)
	if resp.Status == 0 {
		resp = Response{Status: http.StatusInternalServerError, Body: `{"error":"no canned response"}`}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.Status)
	_, _ = w.Write([]byte(resp.Body))
}

func (s *Server) record(req Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, req)
}

func (s *Server) match(body string) Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, resp := range s.responses {
		if resp.Match == "" || strings.Contains(body, resp.Match) {
			return resp
		}
	}
	return Response{}
}

func readBody(r *http.Request) string {
	defer func() { _ = r.Body.Close() }()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}
	return string(b)
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
