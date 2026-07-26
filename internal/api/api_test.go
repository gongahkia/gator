package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

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
