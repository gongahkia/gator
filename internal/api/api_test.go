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
