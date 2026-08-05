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
	"github.com/gongahkia/norbot/internal/observability"
	"github.com/gongahkia/norbot/internal/skill"
	"github.com/gongahkia/norbot/internal/store"
	"github.com/gongahkia/norbot/internal/web"
)

type Server struct {
	service  *engine.Service
	store    *store.Store
	log      *slog.Logger
	skills   *skill.Service
	channels *channel.Service
	auth     *auth.Validator
	oidc     config.OIDC
	security config.Security
	limiter  *clientLimiter
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
	s.oidc = value
	if value.Issuer != "" {
		s.auth = auth.New(value)
	}
	return s
}

func (s *Server) WithSecurity(value config.Security) *Server {
	s.security = value
	s.limiter = newClientLimiter(value.HTTP)
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/config", s.authConfig)
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
	mux.HandleFunc("DELETE /api/channels/accounts/{id}", s.deleteChannelAccount)
	mux.HandleFunc("POST /api/channels/accounts/{id}/messages", s.queueChannelMessage)
	mux.HandleFunc("GET /api/channels/accounts/{id}/messages/{external}", s.channelMessages)
	mux.HandleFunc("GET /api/channels/accounts/{id}/sessions/{external}", s.exportChannelSession)
	mux.HandleFunc("DELETE /api/channels/accounts/{id}/sessions/{external}", s.resetChannelSession)
	mux.HandleFunc("GET /api/channels/{account}/webhook", s.channelWebhook)
	mux.HandleFunc("POST /api/channels/{account}/webhook", s.channelWebhook)
	mux.HandleFunc("GET /metrics", s.metrics)
	mux.HandleFunc("GET /api/operations/outbox/dead", s.deadOutbox)
	mux.HandleFunc("POST /api/operations/outbox/{id}/replay", s.replayOutbox)
	mux.HandleFunc("GET /api/events/stream", s.runEventsStream)
	mux.HandleFunc("GET /api/runs", s.listRuns)
	mux.HandleFunc("POST /api/runs", s.createRun)
	mux.HandleFunc("GET /api/apps", s.listApps)
	mux.HandleFunc("GET /api/runs/{id}", s.getRun)
	mux.HandleFunc("GET /api/runs/{id}/agent-policy", s.agentPolicy)
	mux.HandleFunc("GET /api/runs/{id}/agent-policy-history", s.agentPolicyHistory)
	mux.HandleFunc("PUT /api/runs/{id}/agent-policy", s.restrictAgentPolicy)
	mux.HandleFunc("GET /api/runs/{id}/architecture", s.architecture)
	mux.HandleFunc("PUT /api/runs/{id}/architecture", s.updateArchitecture)
	mux.HandleFunc("GET /api/runs/{id}/acceptance", s.acceptance)
	mux.HandleFunc("PUT /api/runs/{id}/acceptance", s.updateAcceptance)
	mux.HandleFunc("GET /api/runs/{id}/verification-commands", s.verificationCommands)
	mux.HandleFunc("POST /api/runs/{id}/change-runs", s.createChangeRun)
	mux.HandleFunc("GET /api/runs/{id}/events", s.events)
	mux.HandleFunc("GET /api/runs/{id}/trace", s.trace)
	mux.HandleFunc("GET /api/runs/{id}/trace/export", s.traceExport)
	mux.HandleFunc("GET /api/runs/{id}/trace/{event_id}/raw", s.traceRaw)
	mux.HandleFunc("GET /api/runs/{id}/agent-turns", s.agentTurns)
	mux.HandleFunc("GET /api/runs/{id}/sandboxes", s.sandboxExecutions)
	mux.HandleFunc("GET /api/runs/{id}/event-history", s.eventHistory)
	mux.HandleFunc("GET /api/runs/{id}/revisions", s.revisions)
	mux.HandleFunc("GET /api/runs/{id}/planner-revisions", s.plannerRevisions)
	mux.HandleFunc("GET /api/runs/{id}/usage", s.usage)
	mux.HandleFunc("GET /api/runs/{id}/skills", s.runSkills)
	mux.HandleFunc("GET /api/agent/actions", s.agentActions)
	mux.HandleFunc("GET /api/agent/turns/{id}", s.agentTurn)
	mux.HandleFunc("GET /api/agent/actions/{id}", s.agentAction)
	mux.HandleFunc("POST /api/agent/actions/{id}/decision", s.agentActionDecision)
	mux.HandleFunc("PUT /api/runs/{id}/graph", s.updateGraph)
	mux.HandleFunc("POST /api/runs/{id}/approval", s.approve)
	mux.HandleFunc("POST /api/runs/{id}/cancel", s.cancel)
	mux.HandleFunc("POST /api/runs/{id}/cleanup", s.cleanup)
	mux.HandleFunc("DELETE /api/runs/{id}", s.deleteRun)
	mux.HandleFunc("GET /api/runs/{id}/deployment", s.deploymentStatus)
	mux.HandleFunc("GET /api/runs/{id}/deployment/logs", s.deploymentLogs)
	mux.HandleFunc("POST /api/runs/{id}/deployment/start", s.startDeployment)
	mux.HandleFunc("POST /api/runs/{id}/deployment/stop", s.stopDeployment)
	mux.HandleFunc("DELETE /api/runs/{id}/deployment", s.deleteDeployment)
	mux.Handle("/", web.Handler())
	if s.limiter == nil {
		s.limiter = newClientLimiter(s.security.HTTP)
	}
	handler := s.protectMetrics(mux)
	handler = s.rateLimit(handler)
	handler = s.requireOperator(handler)
	handler = s.securityHeaders(handler)
	return requestLog(s.log, otelhttp.NewHandler(handler, "norbot.http"))
}

// WebhookHandler intentionally exposes only signed channel webhook routes.
func (s *Server) WebhookHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/channels/{account}/webhook", s.channelWebhook)
	mux.HandleFunc("POST /api/channels/{account}/webhook", s.channelWebhook)
	return requestLog(s.log, mux)
}

func (s *Server) requireOperator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.URL.Path != "/metrics" && !strings.HasPrefix(r.URL.Path, "/api/")) || s.auth == nil || r.URL.Path == "/api/health" || r.URL.Path == "/api/auth/config" || strings.HasPrefix(r.URL.Path, "/api/channels/") && strings.HasSuffix(r.URL.Path, "/webhook") {
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

func (s *Server) authConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": s.auth != nil,
		"issuer":  s.oidc.Issuer, "client_id": s.oidc.ClientID,
		"scopes": s.oidc.Scopes, "audience": s.oidc.Audience,
	})
}

type operatorContextKey struct{}

func operatorFromRequest(r *http.Request) string {
	operator, _ := r.Context().Value(operatorContextKey{}).(string)
	if operator == "" {
		return "local-operator"
	}
	return operator
}

func (s *Server) skillImports(w http.ResponseWriter, r *http.Request) {
	if s.skills == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("skill marketplace is unavailable"))
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
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("skill marketplace is unavailable"))
		return
	}
	var input skill.ImportInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.skills.ImportAll(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) activateSkill(w http.ResponseWriter, r *http.Request) {
	if s.skills == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("skill marketplace is unavailable"))
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid skill import id"))
		return
	}
	value, err := s.skills.Activate(r.Context(), id, operatorFromRequest(r))
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
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("channel gateway is unavailable"))
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
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("channel gateway is unavailable"))
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

func (s *Server) deleteChannelAccount(w http.ResponseWriter, r *http.Request) {
	if s.channels == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("channel gateway is unavailable"))
		return
	}
	if err := s.channels.DeleteAccount(r.Context(), r.PathValue("id")); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err)
	} else {
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

func (s *Server) queueChannelMessage(w http.ResponseWriter, r *http.Request) {
	if s.channels == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("channel gateway is unavailable"))
		return
	}
	var input struct {
		ExternalID string `json:"external_id"`
		Text       string `json:"text"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	value, err := s.channels.QueueOutbound(r.Context(), r.PathValue("id"), strings.TrimSpace(input.ExternalID), input.Text)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusAccepted, value)
}
func (s *Server) channelMessages(w http.ResponseWriter, r *http.Request) {
	if s.channels == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("channel gateway is unavailable"))
		return
	}
	values, err := s.channels.Messages(r.Context(), r.PathValue("id"), r.PathValue("external"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *Server) exportChannelSession(w http.ResponseWriter, r *http.Request) {
	if s.channels == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("channel gateway is unavailable"))
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
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("channel gateway is unavailable"))
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
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("channel gateway is unavailable"))
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
	gauges, err := s.store.OperationalGauges(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	s.service.Metrics().HandlerWithGauges(w, r, gauges)
}

func (s *Server) deadOutbox(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.DeadOutbox(r.Context(), queryLimit(r, 50))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) replayOutbox(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid outbox event id"))
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := s.store.ReplayDeadOutbox(r.Context(), id, operatorFromRequest(r), input.Reason)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	filter, err := runFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	page, err := s.store.ListRunsFilteredPage(r.Context(), r.URL.Query().Get("cursor"), filter, queryLimit(r, 50))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func runFilter(r *http.Request) (store.RunFilter, error) {
	filter := store.RunFilter{Status: r.URL.Query().Get("status"), AppID: r.URL.Query().Get("app_id"), Search: r.URL.Query().Get("search")}
	for key, target := range map[string]**time.Time{"created_after": &filter.CreatedAfter, "created_before": &filter.CreatedBefore} {
		value := strings.TrimSpace(r.URL.Query().Get(key))
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return store.RunFilter{}, fmt.Errorf("%s must be RFC3339", key)
		}
		*target = &parsed
	}
	return filter, nil
}

func (s *Server) listApps(w http.ResponseWriter, r *http.Request) {
	filter, err := appFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	page, err := s.store.ListAppsFilteredPage(r.Context(), r.URL.Query().Get("cursor"), filter, queryLimit(r, 50))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func appFilter(r *http.Request) (store.AppFilter, error) {
	filter := store.AppFilter{Status: r.URL.Query().Get("status"), Search: r.URL.Query().Get("search")}
	for key, target := range map[string]**time.Time{"updated_after": &filter.UpdatedAfter, "updated_before": &filter.UpdatedBefore} {
		value := strings.TrimSpace(r.URL.Query().Get(key))
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return store.AppFilter{}, fmt.Errorf("%s must be RFC3339", key)
		}
		*target = &parsed
	}
	return filter, nil
}

func queryLimit(r *http.Request, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || value < 1 || value > 100 {
		return fallback
	}
	return value
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

func (s *Server) agentPolicy(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.AgentPolicy(r.Context(), r.PathValue("id"))
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

func (s *Server) agentPolicyHistory(w http.ResponseWriter, r *http.Request) {
	values, err := s.store.RunAgentPolicyHistory(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *Server) restrictAgentPolicy(w http.ResponseWriter, r *http.Request) {
	var input domain.RunAgentPolicy
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	value, err := s.service.RestrictAgentPolicy(r.Context(), r.PathValue("id"), input, operatorFromRequest(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) architecture(w http.ResponseWriter, r *http.Request) {
	run, err := s.store.GetRun(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, run.Architecture)
}

func (s *Server) updateArchitecture(w http.ResponseWriter, r *http.Request) {
	var architecture domain.Architecture
	if err := decodeJSON(r, &architecture); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	run, err := s.service.UpdateArchitecture(r.Context(), r.PathValue("id"), architecture)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusConflict, fmt.Errorf("architecture can only be edited while planner approval is pending"))
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) acceptance(w http.ResponseWriter, r *http.Request) {
	run, err := s.store.GetRun(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, run.Architecture.Acceptance)
}

func (s *Server) updateAcceptance(w http.ResponseWriter, r *http.Request) {
	var acceptance domain.AcceptanceContract
	if err := decodeJSON(r, &acceptance); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	run, err := s.service.UpdateAcceptance(r.Context(), r.PathValue("id"), acceptance)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusConflict, fmt.Errorf("acceptance can only be edited while planner approval is pending"))
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) verificationCommands(w http.ResponseWriter, r *http.Request) {
	commands, err := s.service.VerificationCommands(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, commands)
}

func (s *Server) createChangeRun(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Change                string `json:"change"`
		ArchitectureAffecting bool   `json:"architecture_affecting"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	run, err := s.service.CreateChangeRun(r.Context(), r.PathValue("id"), input.Change, input.ArchitectureAffecting)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) revisions(w http.ResponseWriter, r *http.Request) {
	values, err := s.store.ListRevisions(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *Server) plannerRevisions(w http.ResponseWriter, r *http.Request) {
	values, err := s.store.ListPlannerRevisions(r.Context(), r.PathValue("id"))
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

func (s *Server) deleteRun(w http.ResponseWriter, r *http.Request) {
	if err := s.service.DeleteRun(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusConflict, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

func (s *Server) runEventsStream(w http.ResponseWriter, r *http.Request) {
	rawAfter := strings.TrimSpace(r.URL.Query().Get("after"))
	if rawAfter == "" {
		rawAfter = strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	}
	after, _ := strconv.ParseInt(rawAfter, 10, 64)
	if rawAfter == "" {
		var err error
		after, err = s.store.LatestEventID(r.Context())
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, err)
			return
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}
	_, _ = fmt.Fprintf(w, "id: %d\nevent: ready\ndata: {}\n\n", after)
	flusher.Flush()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		events, err := s.store.EventsAfter(r.Context(), after, 500)
		if err != nil {
			observability.Logger(s.log, r.Context()).Warn("stream global run events", "error", err)
			return
		}
		for _, event := range events {
			encoded, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(w, "id: %d\nevent: run\ndata: %s\n\n", event.ID, encoded)
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

func (s *Server) eventHistory(w http.ResponseWriter, r *http.Request) {
	page, err := s.store.EventsPage(r.Context(), r.PathValue("id"), r.URL.Query().Get("cursor"), queryLimit(r, 100))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) trace(w http.ResponseWriter, r *http.Request) {
	if _, err := s.store.GetRun(r.Context(), r.PathValue("id")); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	filter, err := traceFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	page, err := s.store.TracePage(r.Context(), r.PathValue("id"), filter, queryLimit(r, 100))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) traceRaw(w http.ResponseWriter, r *http.Request) {
	if !s.store.ForensicsEnabled() {
		writeError(w, http.StatusForbidden, fmt.Errorf("local forensic mode is disabled"))
		return
	}
	id, err := strconv.ParseInt(r.PathValue("event_id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid trace event id"))
		return
	}
	value, err := s.store.TraceRaw(r.Context(), r.PathValue("id"), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	_ = s.store.RecordAuditEvent(r.Context(), operatorFromRequest(r), "forensics.raw_revealed", "trace_event", strconv.FormatInt(id, 10), map[string]any{"run_id": r.PathValue("id")})
	writeJSON(w, http.StatusOK, map[string]any{"event_id": id, "raw": value})
}

func (s *Server) traceExport(w http.ResponseWriter, r *http.Request) {
	filter, err := traceFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	includeRaw := r.URL.Query().Get("include") == "raw"
	if includeRaw && !s.store.ForensicsEnabled() {
		writeError(w, http.StatusForbidden, fmt.Errorf("local forensic mode is disabled"))
		return
	}
	items := []map[string]any{}
	for {
		page, err := s.store.TracePage(r.Context(), r.PathValue("id"), filter, 200)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		for _, event := range page.Items {
			item := map[string]any{"event": event}
			if includeRaw && event.RawAvailable {
				raw, err := s.store.TraceRaw(r.Context(), event.RunID, event.ID)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err)
					return
				}
				item["raw"] = raw
			}
			items = append(items, item)
		}
		if page.NextCursor == "" {
			break
		}
		filter.Cursor = page.NextCursor
	}
	_ = s.store.RecordAuditEvent(r.Context(), operatorFromRequest(r), "forensics.trace_exported", "run", r.PathValue("id"), map[string]any{"include": r.URL.Query().Get("include"), "records": len(items)})
	if r.URL.Query().Get("format") == "ndjson" {
		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		encoder := json.NewEncoder(w)
		for _, item := range items {
			_ = encoder.Encode(item)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run_id": r.PathValue("id"), "exported_at": time.Now().UTC(), "items": items})
}

func (s *Server) agentTurns(w http.ResponseWriter, r *http.Request) {
	page, err := s.store.AgentTurnsPage(r.Context(), r.PathValue("id"), r.URL.Query().Get("cursor"), queryLimit(r, 50))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) agentTurn(w http.ResponseWriter, r *http.Request) {
	turn, err := s.store.AgentTurn(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	actions, err := s.store.AgentActionsForTurn(r.Context(), turn.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"turn": turn, "actions": actions})
}

func (s *Server) sandboxExecutions(w http.ResponseWriter, r *http.Request) {
	values, err := s.store.SandboxExecutions(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func traceFilter(r *http.Request) (domain.TraceFilter, error) {
	query := r.URL.Query()
	value := domain.TraceFilter{Cursor: query.Get("cursor"), Stage: query.Get("stage"), Type: query.Get("type"), Severity: query.Get("severity"), ProviderID: query.Get("provider_id"), Tool: query.Get("tool"), Actor: query.Get("actor"), EntityID: query.Get("entity_id"), Query: query.Get("q")}
	for raw, target := range map[string]**time.Time{"from": &value.From, "to": &value.To} {
		if text := query.Get(raw); text != "" {
			parsed, err := time.Parse(time.RFC3339, text)
			if err != nil {
				return value, fmt.Errorf("%s must be RFC3339", raw)
			}
			*target = &parsed
		}
	}
	return value, nil
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
		observability.Logger(logger, r.Context()).Info("http request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(started).Milliseconds(), "remote", strings.Split(r.RemoteAddr, ":")[0])
	})
}

func Shutdown(ctx context.Context, server *http.Server) error { return server.Shutdown(ctx) }
