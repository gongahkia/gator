package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gongahkia/norbot/internal/config"
)

func TestOIDCProtectsMetricsAndServesConsole(t *testing.T) {
	server := New(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).WithOIDC(config.OIDC{Issuer: "https://issuer.example", Audience: "norbot", GroupsClaim: "groups", OperatorGroups: []string{"operators"}, ClientID: "norbot-console"})
	handler := server.Handler()
	metrics := httptest.NewRecorder()
	handler.ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metrics.Code != http.StatusUnauthorized {
		t.Fatalf("metrics status = %d", metrics.Code)
	}
	console := httptest.NewRecorder()
	handler.ServeHTTP(console, httptest.NewRequest(http.MethodGet, "/", nil))
	if console.Code != http.StatusOK {
		t.Fatalf("console status = %d", console.Code)
	}
}

func TestPublicProfileRequiresHTTPSAndMetricsToken(t *testing.T) {
	t.Setenv("NORBOT_METRICS_TOKEN", "test-token")
	server := New(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).WithSecurity(config.Security{Public: true, HTTP: config.HTTPPolicy{RequireHTTPS: true, MetricsTokenEnv: "NORBOT_METRICS_TOKEN", RatePerMinute: 10, RateBurst: 1}})
	handler := server.Handler()
	plain := httptest.NewRecorder()
	handler.ServeHTTP(plain, httptest.NewRequest(http.MethodGet, "/", nil))
	if plain.Code != http.StatusUpgradeRequired {
		t.Fatalf("plain status=%d", plain.Code)
	}
	metrics := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	handler.ServeHTTP(metrics, request)
	if metrics.Code != http.StatusUnauthorized {
		t.Fatalf("metrics status=%d", metrics.Code)
	}
}

func TestClientLimiterEnforcesBurst(t *testing.T) {
	limiter := newClientLimiter(config.HTTPPolicy{RatePerMinute: 1, RateBurst: 1})
	if !limiter.allow("operator:one") || limiter.allow("operator:one") {
		t.Fatal("limiter did not enforce burst")
	}
}

func TestAppFilterValidatesRFC3339Bounds(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/apps?status=running&search=calendar&updated_after=2026-07-01T00:00:00Z&updated_before=2026-08-01T00:00:00Z", nil)
	filter, err := appFilter(request)
	if err != nil || filter.Status != "running" || filter.Search != "calendar" || filter.UpdatedAfter == nil || filter.UpdatedBefore == nil {
		t.Fatalf("filter=%#v err=%v", filter, err)
	}
	if !filter.UpdatedAfter.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("updated_after=%v", filter.UpdatedAfter)
	}
	if _, err := appFilter(httptest.NewRequest(http.MethodGet, "/api/apps?updated_after=tomorrow", nil)); err == nil {
		t.Fatal("invalid app filter accepted")
	}
}
