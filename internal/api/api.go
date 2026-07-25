package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/gongahkia/norbot/internal/auth"
	"github.com/gongahkia/norbot/internal/channel"
	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/engine"
	"github.com/gongahkia/norbot/internal/skill"
	"github.com/gongahkia/norbot/internal/store"
)

type Server struct {
	service  *engine.Service
	store    *store.Store
	log      *slog.Logger
	skills   *skill.Service
	channels *channel.Service
	auth     *auth.Validator
}

func New(service *engine.Service, st *store.Store, logger *slog.Logger) *Server {
	return &Server{service: service, store: st, log: logger}
}

func NewWithSkills(service *engine.Service, st *store.Store, logger *slog.Logger, skills *skill.Service) *Server {
	server := New(service, st, logger)
	server.skills = skills
	return server
}

func NewWithComponents(service *engine.Service, st *store.Store, logger *slog.Logger, skills *skill.Service, channels *channel.Service) *Server {
	server := NewWithSkills(service, st, logger, skills)
	server.channels = channels
	return server
}

func (s *Server) WithOIDC(value config.OIDC) *Server {
	if value.Issuer != "" {
		s.auth = auth.New(value)
	}
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/health/detail", s.healthDetail)
	mux.HandleFunc("GET /api/health/stream", s.healthStream)
	mux.HandleFunc("GET /api/capacity", s.capacity)
	mux.HandleFunc("POST /api/capacity/recommendations", s.recommendCapacity)
	mux.HandleFunc("POST /api/capacity/recommendations/{id}/accept", s.acceptCapacity)
	mux.HandleFunc("GET /api/providers", s.providers)
	mux.HandleFunc("GET /api/runtime", s.runtimeOptions)
	mux.HandleFunc("GET /api/skills/imports", s.skillImports)
	mux.HandleFunc("POST /api/skills/imports", s.importSkill)
	mux.HandleFunc("POST /api/skills/imports/{id}/activate", s.activateSkill)
	mux.HandleFunc("GET /api/channels/accounts", s.channelAccounts)
	mux.HandleFunc("POST /api/channels/accounts", s.createChannelAccount)
	mux.HandleFunc("POST /api/channels/accounts/{id}/pairings", s.pairChannel)
	mux.HandleFunc("DELETE /api/channels/accounts/{id}/pairings/{external}", s.unpairChannel)
	mux.HandleFunc("GET /api/channels/accounts/{id}/sessions/{external}", s.exportChannelSession)
	mux.HandleFunc("DELETE /api/channels/accounts/{id}/sessions/{external}", s.resetChannelSession)
	mux.HandleFunc("GET /api/channels/{account}/webhook", s.channelWebhook)
	mux.HandleFunc("POST /api/channels/{account}/webhook", s.channelWebhook)
	mux.HandleFunc("GET /metrics", s.metrics)
	mux.HandleFunc("GET /api/runs", s.listRuns)
	mux.HandleFunc("POST /api/runs", s.createRun)
	mux.HandleFunc("GET /api/runs/{id}", s.getRun)
	mux.HandleFunc("GET /api/runs/{id}/events", s.events)
	mux.HandleFunc("GET /api/runs/{id}/revisions", s.revisions)
	mux.HandleFunc("GET /api/runs/{id}/usage", s.usage)
	mux.HandleFunc("GET /api/runs/{id}/skills", s.runSkills)
	mux.HandleFunc("GET /api/agent/actions", s.agentActions)
	mux.HandleFunc("GET /api/agent/actions/{id}", s.agentAction)
	mux.HandleFunc("POST /api/agent/actions/{id}/decision", s.agentActionDecision)
	mux.HandleFunc("PUT /api/runs/{id}/graph", s.updateGraph)
	mux.HandleFunc("POST /api/runs/{id}/approval", s.approve)
	mux.HandleFunc("POST /api/runs/{id}/cancel", s.cancel)
	mux.HandleFunc("POST /api/runs/{id}/cleanup", s.cleanup)
	mux.HandleFunc("GET /api/runs/{id}/deployment", s.deploymentStatus)
	mux.HandleFunc("GET /api/runs/{id}/deployment/logs", s.deploymentLogs)
	mux.HandleFunc("POST /api/runs/{id}/deployment/start", s.startDeployment)
	mux.HandleFunc("POST /api/runs/{id}/deployment/stop", s.stopDeployment)
	mux.HandleFunc("DELETE /api/runs/{id}/deployment", s.deleteDeployment)
	return requestLog(s.log, otelhttp.NewHandler(s.requireOperator(mux), "norbot.http"))
}

func (s *Server) requireOperator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil || r.URL.Path == "/api/health" || strings.HasPrefix(r.URL.Path, "/api/channels/") && strings.HasSuffix(r.URL.Path, "/webhook") {
			next.ServeHTTP(w, r)
			return
		}
		value := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
		if value == "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			writeError(w, http.StatusUnauthorized, fmt.Errorf("bearer token is required"))
			return
		}
		principal, err := s.auth.Validate(r.Context(), value)
		if err != nil {
			writeError(w, http.StatusForbidden, fmt.Errorf("operator authorization failed"))
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), operatorContextKey{}, principal.Subject))
		next.ServeHTTP(w, r)
	})
}

type operatorContextKey struct{}

func (s *Server) skillImports(w http.ResponseWriter, r *http.Request) {
	if s.skills == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("skill marketplace is not configured"))
		return
	}
	values, err := s.skills.Imports(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *Server) importSkill(w http.ResponseWriter, r *http.Request) {
	if s.skills == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("skill marketplace is not configured"))
		return
	}
	var input skill.ImportInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	imported, pkg, err := s.skills.Import(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"import": imported, "package": pkg})
}

func (s *Server) activateSkill(w http.ResponseWriter, r *http.Request) {
	if s.skills == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("skill marketplace is not configured"))
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid skill import id"))
		return
	}
	value, err := s.skills.Activate(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusConflict, fmt.Errorf("skill import is not scan-approved"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) channelAccounts(w http.ResponseWriter, r *http.Request) {
	if s.channels == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("channel gateway is not configured"))
		return
	}
	values, err := s.channels.Accounts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *Server) createChannelAccount(w http.ResponseWriter, r *http.Request) {
	if s.channels == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("channel gateway is not configured"))
		return
	}
	var input domain.ChannelAccount
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	value, err := s.channels.CreateAccount(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) pairChannel(w http.ResponseWriter, r *http.Request) {
	if s.channels == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("channel gateway is not configured"))
		return
	}
	var input struct {
		ExternalID string     `json:"external_id"`
		ExpiresAt  *time.Time `json:"expires_at"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	value, err := s.channels.Pair(r.Context(), r.PathValue("id"), strings.TrimSpace(input.ExternalID), input.ExpiresAt)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) unpairChannel(w http.ResponseWriter, r *http.Request) {
	if s.channels == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("channel gateway is not configured"))
		return
	}
	err := s.channels.Unpair(r.Context(), r.PathValue("id"), r.PathValue("external"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unpaired"})
}

func (s *Server) exportChannelSession(w http.ResponseWriter, r *http.Request) {
	if s.channels == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("channel gateway is not configured"))
		return
	}
	values, err := s.channels.ExportSession(r.Context(), r.PathValue("id"), r.PathValue("external"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *Server) resetChannelSession(w http.ResponseWriter, r *http.Request) {
	if s.channels == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("channel gateway is not configured"))
		return
	}
	err := s.channels.ResetSession(r.Context(), r.PathValue("id"), r.PathValue("external"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func (s *Server) channelWebhook(w http.ResponseWriter, r *http.Request) {
	if s.channels == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("channel gateway is not configured"))
		return
	}
	s.channels.HandleWebhook(w, r)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if _, err := s.store.ListRuns(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "norbot", "time": time.Now().UTC()})
}

func (s *Server) healthDetail(w http.ResponseWriter, r *http.Request) {
	report := s.service.Health(r.Context())
	status := http.StatusOK
	if report.State == domain.HealthDown {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, report)
}

func (s *Server) healthStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unavailable"))
		return
	}
	for {
		report := s.service.Health(r.Context())
		payload, err := json.Marshal(report)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(w, "event: health\ndata: %s\n\n", payload)
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (s *Server) capacity(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.service.Capacity(r.Context()))
}

func (s *Server) recommendCapacity(w http.ResponseWriter, r *http.Request) {
	recommendation, err := s.service.RecommendCapacity(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, recommendation)
}

func (s *Server) acceptCapacity(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid recommendation id"))
		return
	}
	var input struct {
		Workers int `json:"workers"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	recommendation, err := s.service.AcceptCapacity(r.Context(), id, input.Workers)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, recommendation)
}

func (s *Server) providers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.service.ProviderOptions())
}

func (s *Server) runtimeOptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.service.RuntimeOptions())
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	s.service.Metrics().Handler(w, r)
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.store.ListRuns(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) createRun(w http.ResponseWriter, r *http.Request) {
	var input engine.CreateRunInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	run, err := s.service.CreateRun(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.store.GetRun(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) revisions(w http.ResponseWriter, r *http.Request) {
	values, err := s.store.ListRevisions(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *Server) usage(w http.ResponseWriter, r *http.Request) {
	values, err := s.store.Usage(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *Server) runSkills(w http.ResponseWriter, r *http.Request) {
	values, err := s.store.RunSkills(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *Server) agentActions(w http.ResponseWriter, r *http.Request) {
	values, err := s.service.PendingAgentActions(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}
func (s *Server) agentAction(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.AgentAction(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (s *Server) agentActionDecision(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Decision string `json:"decision"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	operator, _ := r.Context().Value(operatorContextKey{}).(string)
	if operator == "" {
		operator = "local-operator"
	}
	if s.channels != nil {
		response, action, err := s.channels.DecideAgentAction(r.Context(), r.PathValue("id"), input.Decision, operator)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"action": action, "response": response})
		return
	}
	response, action, err := s.service.DecideAgentAction(r.Context(), r.PathValue("id"), input.Decision, operator)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"action": action, "response": response})
}

func (s *Server) updateGraph(w http.ResponseWriter, r *http.Request) {
	var graph domain.Graph
	if err := decodeJSON(r, &graph); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	run, err := s.service.UpdateGraph(r.Context(), r.PathValue("id"), graph)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusConflict, fmt.Errorf("graph can only be edited while planner approval is pending"))
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) approve(w http.ResponseWriter, r *http.Request) {
	var input engine.ApprovalInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	run, err := s.service.Approve(r.Context(), r.PathValue("id"), input)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	run, err := s.service.Cancel(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) cleanup(w http.ResponseWriter, r *http.Request) {
	if err := s.service.Cleanup(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cleaned"})
}

func (s *Server) deploymentStatus(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.DeploymentStatus(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) deploymentLogs(w http.ResponseWriter, r *http.Request) {
	lines, err := strconv.Atoi(r.URL.Query().Get("lines"))
	if err != nil || lines == 0 {
		lines = 200
	}
	logs, err := s.service.DeploymentLogs(r.Context(), r.PathValue("id"), lines)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs, "lines": lines})
}

func (s *Server) startDeployment(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.StartDeployment(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) stopDeployment(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.StopDeployment(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) deleteDeployment(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.DeleteDeployment(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	after, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if raw := r.URL.Query().Get("after"); raw != "" {
		after, _ = strconv.ParseInt(raw, 10, 64)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		events, err := s.store.Events(r.Context(), r.PathValue("id"), after)
		if err != nil {
			return
		}
		for _, event := range events {
			encoded, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", event.ID, encoded)
			after = event.ID
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("request must contain one JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func requestLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("http request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(started).Milliseconds(), "remote", strings.Split(r.RemoteAddr, ":")[0])
	})
}

func Shutdown(ctx context.Context, server *http.Server) error { return server.Shutdown(ctx) }
