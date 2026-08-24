// Package reviewweb serves the local browser review surface for one retained
// Gator worktree. It is intentionally separate from appserver: appserver
// rejects browser origins for RPC, while this package accepts only its own
// authenticated, same-origin browser session.
package reviewweb

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/review"
)

const (
	cookieName       = "gator_review_session"
	sessionLifetime  = 15 * time.Minute
	maximumBodyBytes = 64 * 1024
)

// Config identifies one already-retained Gator run. StatePath is never sent
// to the browser; the server uses it only to locate the private worktree and
// journal on the local filesystem.
type Config struct {
	StatePath string
	Now       func() time.Time
}

// Server exposes a small, authenticated review API and same-origin HTML page.
// Its one-time bootstrap credential is exchanged immediately for an HttpOnly
// cookie, so routine review requests never carry a secret in a URL.
type Server struct {
	statePath string
	session   journal.Session
	now       func() time.Time

	mu              sync.Mutex
	bootstrapSecret []byte
	bootstrapUsed   bool
	cookie          []byte
	cookieExpiry    time.Time
}

// New validates the retained run before binding a browser endpoint.
func New(config Config) (*Server, error) {
	if strings.TrimSpace(config.StatePath) == "" {
		return nil, errors.New("review run record path is required")
	}
	session, err := journal.LoadSession(config.StatePath)
	if err != nil {
		return nil, fmt.Errorf("open retained review run: %w", err)
	}
	if strings.TrimSpace(session.WorktreePath) == "" {
		return nil, errors.New("retained review run has no worktree")
	}
	bootstrap, err := randomSecret()
	if err != nil {
		return nil, err
	}
	cookie, err := randomSecret()
	if err != nil {
		return nil, err
	}
	clock := config.Now
	if clock == nil {
		clock = time.Now
	}
	return &Server{statePath: config.StatePath, session: session, now: clock, bootstrapSecret: bootstrap, cookie: cookie}, nil
}

// BootstrapURL returns the intentionally short-lived, one-use local URL. It
// must only be printed to the local developer who started this server.
func (s *Server) BootstrapURL(address string) (string, error) {
	if s == nil {
		return "", errors.New("review server is not initialized")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || strings.TrimSpace(port) == "" {
		return "", errors.New("review server address must be a loopback IP and port")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bootstrapUsed {
		return "", errors.New("review bootstrap URL has already been used")
	}
	value := base64.RawURLEncoding.EncodeToString(s.bootstrapSecret)
	return "http://" + address + "/review/?access=" + url.QueryEscape(value), nil
}

// Handler serves only loopback peers. Callers should additionally bind their
// listener to a literal loopback address; both checks are deliberate defense
// in depth for source-bearing review data.
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serveHTTP) }

func (s *Server) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	securityHeaders(writer)
	if !loopbackPeer(request.RemoteAddr) {
		writeError(writer, http.StatusForbidden, "review is available only to loopback clients")
		return
	}
	switch request.URL.Path {
	case "/review/":
		if request.Method != http.MethodGet {
			writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if request.URL.Query().Get("access") != "" {
			s.bootstrap(writer, request)
			return
		}
		if !s.authorized(request, false) {
			writeError(writer, http.StatusUnauthorized, "review session is required")
			return
		}
		s.page(writer)
	case "/review/api/snapshot":
		if request.Method != http.MethodGet || !s.authorized(request, false) {
			writeError(writer, http.StatusUnauthorized, "review session is required")
			return
		}
		s.snapshot(writer)
	case "/review/api/file":
		if request.Method != http.MethodGet || !s.authorized(request, false) {
			writeError(writer, http.StatusUnauthorized, "review session is required")
			return
		}
		s.filePatch(writer, request)
	case "/review/api/stage":
		if request.Method != http.MethodPost || !s.authorized(request, true) {
			writeError(writer, http.StatusUnauthorized, "same-origin review session is required")
			return
		}
		s.stage(writer, request)
	case "/review/api/feedback":
		if request.Method != http.MethodPost || !s.authorized(request, true) {
			writeError(writer, http.StatusUnauthorized, "same-origin review session is required")
			return
		}
		s.feedback(writer, request)
	default:
		writeError(writer, http.StatusNotFound, "route not found")
	}
}

func (s *Server) bootstrap(writer http.ResponseWriter, request *http.Request) {
	if request.Header.Get("Origin") != "" {
		writeError(writer, http.StatusForbidden, "cross-origin review bootstrap is not accepted")
		return
	}
	provided, err := base64.RawURLEncoding.DecodeString(request.URL.Query().Get("access"))
	if err != nil {
		writeError(writer, http.StatusUnauthorized, "invalid review bootstrap credential")
		return
	}
	s.mu.Lock()
	valid := !s.bootstrapUsed && subtle.ConstantTimeCompare(provided, s.bootstrapSecret) == 1
	if valid {
		s.bootstrapUsed = true
		s.cookieExpiry = s.now().Add(sessionLifetime)
	}
	expires := s.cookieExpiry
	cookie := append([]byte(nil), s.cookie...)
	s.mu.Unlock()
	if !valid {
		writeError(writer, http.StatusUnauthorized, "review bootstrap credential has expired or was already used")
		return
	}
	http.SetCookie(writer, &http.Cookie{
		Name: cookieName, Value: base64.RawURLEncoding.EncodeToString(cookie), Path: "/review/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: int(sessionLifetime.Seconds()),
	})
	writer.Header().Set("Location", "/review/")
	writer.WriteHeader(http.StatusSeeOther)
}

func (s *Server) authorized(request *http.Request, requireOrigin bool) bool {
	if !sameOrigin(request, requireOrigin) {
		return false
	}
	cookie, err := request.Cookie(cookieName)
	if err != nil {
		return false
	}
	provided, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.cookieExpiry.IsZero() && s.now().Before(s.cookieExpiry) && subtle.ConstantTimeCompare(provided, s.cookie) == 1
}

func (s *Server) snapshot(writer http.ResponseWriter) {
	snapshot, err := s.loadSnapshot()
	if err != nil {
		writeError(writer, http.StatusConflict, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, snapshot)
}

func (s *Server) filePatch(writer http.ResponseWriter, request *http.Request) {
	snapshot, err := s.loadSnapshot()
	if err != nil {
		writeError(writer, http.StatusConflict, err.Error())
		return
	}
	fileID := strings.TrimSpace(request.URL.Query().Get("id"))
	scope := review.Scope(strings.TrimSpace(request.URL.Query().Get("scope")))
	if scope != review.All && scope != review.Staged && scope != review.Unstaged {
		scope = review.All
	}
	for _, file := range snapshot.ChangeSet(scope).Files {
		if file.ID == fileID {
			writeJSON(writer, http.StatusOK, map[string]string{"id": file.ID, "patch": file.Patch})
			return
		}
	}
	writeError(writer, http.StatusNotFound, "review file is no longer present")
}

type stageRequest struct {
	Scope  review.Scope `json:"scope"`
	FileID string       `json:"file_id"`
	HunkID string       `json:"hunk_id,omitempty"`
	Whole  bool         `json:"whole"`
}

func (s *Server) stage(writer http.ResponseWriter, request *http.Request) {
	var value stageRequest
	if err := decodeJSON(request, &value); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if value.Scope != review.Staged && value.Scope != review.Unstaged {
		writeError(writer, http.StatusBadRequest, "stage scope must be staged or unstaged")
		return
	}
	snapshot, err := s.loadSnapshot()
	if err != nil {
		writeError(writer, http.StatusConflict, err.Error())
		return
	}
	file, hunk, found := findSelection(snapshot.ChangeSet(value.Scope), value.FileID, value.HunkID)
	if !found {
		writeError(writer, http.StatusConflict, "review selection is no longer present; refresh first")
		return
	}
	switch {
	case value.Whole && value.Scope == review.Unstaged:
		err = review.StageFile(request.Context(), s.session.WorktreePath, file.Path)
	case value.Whole && value.Scope == review.Staged:
		err = review.UnstageFile(request.Context(), s.session.WorktreePath, file.Path)
	case !value.Whole && value.HunkID == "":
		writeError(writer, http.StatusBadRequest, "hunk id is required unless whole-file is selected")
		return
	case !value.Whole && value.Scope == review.Unstaged:
		err = review.StageHunk(request.Context(), s.session.WorktreePath, hunk.ID)
	case !value.Whole && value.Scope == review.Staged:
		err = review.UnstageHunk(request.Context(), s.session.WorktreePath, hunk.ID)
	}
	if err != nil {
		writeError(writer, http.StatusConflict, err.Error())
		return
	}
	updated, err := s.loadSnapshot()
	if err != nil {
		writeError(writer, http.StatusConflict, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, updated)
}

type feedbackRequest struct {
	Scope       review.Scope `json:"scope"`
	FileID      string       `json:"file_id"`
	HunkID      string       `json:"hunk_id"`
	Start       int          `json:"start"`
	End         int          `json:"end"`
	Instruction string       `json:"instruction"`
}

func (s *Server) feedback(writer http.ResponseWriter, request *http.Request) {
	var value feedbackRequest
	if err := decodeJSON(request, &value); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if value.Scope != review.All && value.Scope != review.Staged && value.Scope != review.Unstaged {
		writeError(writer, http.StatusBadRequest, "review scope is invalid")
		return
	}
	snapshot, err := s.loadSnapshot()
	if err != nil {
		writeError(writer, http.StatusConflict, err.Error())
		return
	}
	file, hunk, found := findSelection(snapshot.ChangeSet(value.Scope), value.FileID, value.HunkID)
	if !found {
		writeError(writer, http.StatusConflict, "review selection is no longer present; refresh first")
		return
	}
	selection, err := selectionFromLines(hunk, value.Start, value.End)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	feedback, err := journal.SaveReviewFeedback(s.statePath, journal.ReviewFeedback{
		File: file.Path, HunkID: hunk.ID, Side: selection.side, StartLine: selection.startLine, EndLine: selection.endLine,
		Before: selection.before, After: selection.after, Instruction: value.Instruction,
	})
	if err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{"feedback": feedback, "follow_up": journal.ReviewFollowUp(feedback)})
}

type selectedRange struct {
	side      string
	startLine int
	endLine   int
	before    []string
	after     []string
}

func selectionFromLines(hunk review.Hunk, start, end int) (selectedRange, error) {
	if start < 0 || end < start || end >= len(hunk.Lines) {
		return selectedRange{}, errors.New("selected review line range is invalid")
	}
	value := selectedRange{}
	oldStart, oldEnd, newStart, newEnd := 0, 0, 0, 0
	for _, line := range hunk.Lines[start : end+1] {
		if line.Kind == "meta" {
			return selectedRange{}, errors.New("selected review range cannot include diff metadata")
		}
		text := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(line.Text, "+"), "-"), " ")
		switch line.Kind {
		case "deletion":
			value.before = append(value.before, text)
			oldStart, oldEnd = extendRange(oldStart, oldEnd, line.OldLine)
		case "addition":
			value.after = append(value.after, text)
			newStart, newEnd = extendRange(newStart, newEnd, line.NewLine)
		case "context":
			value.before, value.after = append(value.before, text), append(value.after, text)
			oldStart, oldEnd = extendRange(oldStart, oldEnd, line.OldLine)
			newStart, newEnd = extendRange(newStart, newEnd, line.NewLine)
		}
	}
	switch {
	case oldStart > 0 && newStart > 0:
		value.side, value.startLine, value.endLine = "both", min(oldStart, newStart), max(oldEnd, newEnd)
	case oldStart > 0:
		value.side, value.startLine, value.endLine = "old", oldStart, oldEnd
	case newStart > 0:
		value.side, value.startLine, value.endLine = "new", newStart, newEnd
	default:
		return selectedRange{}, errors.New("selected review range has no source lines")
	}
	return value, nil
}

func extendRange(start, end, line int) (int, int) {
	if line <= 0 {
		return start, end
	}
	if start == 0 || line < start {
		start = line
	}
	if line > end {
		end = line
	}
	return start, end
}

func findSelection(set review.ChangeSet, fileID, hunkID string) (review.File, review.Hunk, bool) {
	for _, file := range set.Files {
		if file.ID != fileID {
			continue
		}
		if hunkID == "" {
			return file, review.Hunk{}, true
		}
		for _, hunk := range file.Hunks {
			if hunk.ID == hunkID {
				return file, hunk, true
			}
		}
	}
	return review.File{}, review.Hunk{}, false
}

func (s *Server) loadSnapshot() (review.Snapshot, error) {
	snapshot, err := review.Load(context.Background(), s.session.WorktreePath, s.session.BaseCommit)
	if err != nil {
		return review.Snapshot{}, fmt.Errorf("load retained review: %w", err)
	}
	return snapshot, nil
}

func randomSecret() ([]byte, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return nil, fmt.Errorf("generate review session secret: %w", err)
	}
	return value, nil
}

func loopbackPeer(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func sameOrigin(request *http.Request, require bool) bool {
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if require && origin == "" {
		return false
	}
	if origin != "" && origin != "http://"+request.Host {
		return false
	}
	if strings.EqualFold(request.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return false
	}
	return true
}

func securityHeaders(writer http.ResponseWriter) {
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
}

func (s *Server) page(writer http.ResponseWriter) {
	nonce, err := randomSecret()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	nonceValue := base64.RawURLEncoding.EncodeToString(nonce)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'; connect-src 'self'; img-src 'none'; style-src 'nonce-"+nonceValue+"'; script-src 'nonce-"+nonceValue+"'")
	_, _ = io.WriteString(writer, strings.ReplaceAll(strings.ReplaceAll(reviewPage, "{{nonce}}", nonceValue), "{{title}}", "Gator review"))
}

func decodeJSON(request *http.Request, destination any) error {
	if !strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
		return errors.New("content type must be application/json")
	}
	decoder := json.NewDecoder(io.LimitReader(request.Body, maximumBodyBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return errors.New("invalid JSON request")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("JSON request must contain one value")
	}
	return nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}
