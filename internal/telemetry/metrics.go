package telemetry

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type Metrics struct {
	mu          sync.Mutex
	inFlight    map[string]int64
	stages      map[string]stageMetric
	providers   map[string]providerMetric
	deployments map[string]int64
	caches      map[string]cacheMetric
}

type stageMetric struct {
	Started, Completed, Failed int64
	Duration                   time.Duration
}
type providerMetric struct {
	Calls, Failed int64
	Duration      time.Duration
}
type cacheMetric struct{ Hits, Misses int64 }

func NewMetrics() *Metrics {
	return &Metrics{inFlight: map[string]int64{}, stages: map[string]stageMetric{}, providers: map[string]providerMetric{}, deployments: map[string]int64{}, caches: map[string]cacheMetric{}}
}

func (m *Metrics) StartStage(stage string) func(error) {
	started := time.Now()
	stage = label(stage)
	m.mu.Lock()
	item := m.stages[stage]
	item.Started++
	m.stages[stage] = item
	m.inFlight[stage]++
	m.mu.Unlock()
	return func(err error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		item := m.stages[stage]
		m.inFlight[stage]--
		item.Duration += time.Since(started)
		if err != nil {
			item.Failed++
		} else {
			item.Completed++
		}
		m.stages[stage] = item
	}
}

func (m *Metrics) ObserveProvider(provider string, duration time.Duration, err error) {
	provider = label(provider)
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.providers[provider]
	item.Calls++
	item.Duration += duration
	if err != nil {
		item.Failed++
	}
	m.providers[provider] = item
}
func (m *Metrics) ObserveDeployment(action string) {
	m.mu.Lock()
	m.deployments[label(action)]++
	m.mu.Unlock()
}

func (m *Metrics) ObserveCache(ecosystem string, hit bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.caches[label(ecosystem)]
	if hit {
		item.Hits++
	} else {
		item.Misses++
	}
	m.caches[label(ecosystem)] = item
}

func (m *Metrics) Handler(w http.ResponseWriter, _ *http.Request) {
	m.HandlerWithGauges(w, nil, nil)
}

func (m *Metrics) HandlerWithGauges(w http.ResponseWriter, _ *http.Request, gauges map[string]int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintln(w, "# HELP norbot_up Norbot API availability\n# TYPE norbot_up gauge\nnorbot_up 1")
	fmt.Fprintln(w, "# HELP norbot_stage_runs_total Stage execution outcomes\n# TYPE norbot_stage_runs_total counter")
	for _, key := range ordered(m.stages) {
		item := m.stages[key]
		fmt.Fprintf(w, "norbot_stage_runs_total{stage=%q,outcome=\"started\"} %d\n", key, item.Started)
		fmt.Fprintf(w, "norbot_stage_runs_total{stage=%q,outcome=\"completed\"} %d\n", key, item.Completed)
		fmt.Fprintf(w, "norbot_stage_runs_total{stage=%q,outcome=\"failed\"} %d\n", key, item.Failed)
		fmt.Fprintf(w, "norbot_stage_in_flight{stage=%q} %d\n", key, m.inFlight[key])
		fmt.Fprintf(w, "norbot_stage_duration_seconds_sum{stage=%q} %.6f\n", key, item.Duration.Seconds())
		fmt.Fprintf(w, "norbot_stage_duration_seconds_count{stage=%q} %d\n", key, item.Completed+item.Failed)
	}
	fmt.Fprintln(w, "# HELP norbot_provider_calls_total Provider invocation outcomes\n# TYPE norbot_provider_calls_total counter")
	for _, key := range ordered(m.providers) {
		item := m.providers[key]
		fmt.Fprintf(w, "norbot_provider_calls_total{provider=%q,outcome=\"total\"} %d\n", key, item.Calls)
		fmt.Fprintf(w, "norbot_provider_calls_total{provider=%q,outcome=\"failed\"} %d\n", key, item.Failed)
		fmt.Fprintf(w, "norbot_provider_duration_seconds_sum{provider=%q} %.6f\n", key, item.Duration.Seconds())
		fmt.Fprintf(w, "norbot_provider_duration_seconds_count{provider=%q} %d\n", key, item.Calls)
	}
	fmt.Fprintln(w, "# HELP norbot_deployment_actions_total Deployment lifecycle actions\n# TYPE norbot_deployment_actions_total counter")
	for _, key := range ordered(m.deployments) {
		fmt.Fprintf(w, "norbot_deployment_actions_total{action=%q} %d\n", key, m.deployments[key])
	}
	fmt.Fprintln(w, "# HELP norbot_verification_cache_access_total Dependency cache outcomes\n# TYPE norbot_verification_cache_access_total counter")
	for _, key := range ordered(m.caches) {
		item := m.caches[key]
		fmt.Fprintf(w, "norbot_verification_cache_access_total{ecosystem=%q,outcome=\"hit\"} %d\n", key, item.Hits)
		fmt.Fprintf(w, "norbot_verification_cache_access_total{ecosystem=%q,outcome=\"miss\"} %d\n", key, item.Misses)
	}
	fmt.Fprintln(w, "# HELP norbot_operational_gauge Durable operational state\n# TYPE norbot_operational_gauge gauge")
	for _, key := range ordered(gauges) {
		fmt.Fprintf(w, "norbot_operational_gauge{kind=%q} %d\n", key, gauges[key])
	}
}

func ordered[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func label(value string) string {
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, value)
	if value == "" {
		return "unknown"
	}
	return value
}
