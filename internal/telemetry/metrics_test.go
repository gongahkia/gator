package telemetry

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrometheusMetricsContainOutcomes(t *testing.T) {
	metrics := NewMetrics()
	finish := metrics.StartStage("builder")
	finish(errors.New("failed"))
	metrics.ObserveProvider("openai", 0, nil)
	metrics.ObserveDeployment("created")
	metrics.ObserveCache("node", true)
	metrics.ObserveCache("node", false)
	recorder := httptest.NewRecorder()
	metrics.Handler(recorder, httptest.NewRequest("GET", "/metrics", nil))
	for _, expected := range []string{"norbot_stage_runs_total", "stage=\"builder\"", "norbot_provider_calls_total", "provider=\"openai\"", "norbot_deployment_actions_total", "norbot_verification_cache_access_total", "ecosystem=\"node\"", "outcome=\"hit\"", "outcome=\"miss\""} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Fatalf("missing %q: %s", expected, recorder.Body.String())
		}
	}
}
