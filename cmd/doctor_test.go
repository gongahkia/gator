package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gongahkia/paw/internal/llm"
)

func TestDoctorModelsPrintsConfiguredEndpointReports(t *testing.T) {
	isolateEnv(t)
	fake := &fakeEndpointHealth{
		reports: []llm.HealthReport{
			{
				Transport: "ollama",
				BaseURL:   "http://localhost:11434",
				Model:     "gpt-oss:20b",
				Checks: []llm.HealthCheck{
					{Name: "running", Status: llm.HealthOK, Detail: "Ollama metadata endpoint responded"},
					{Name: "model", Status: llm.HealthOK, Detail: "gpt-oss:20b"},
				},
			},
			{
				Transport: "ollama",
				BaseURL:   "http://localhost:11434",
				Model:     "qwen3:8b",
				Checks: []llm.HealthCheck{
					{Name: "running", Status: llm.HealthOK, Detail: "Ollama metadata endpoint responded"},
					{Name: "model", Status: llm.HealthOK, Detail: "qwen3:8b"},
				},
			},
		},
	}
	withDoctorHealth(t, fake)

	out := executeRoot(t, append(configArgs(t), "doctor", "models"), "")
	if !strings.Contains(out, "brain: ollama model=gpt-oss:20b base_url=http://localhost:11434") {
		t.Fatalf("missing brain report:\n%s", out)
	}
	if !strings.Contains(out, "drone: ollama model=qwen3:8b base_url=http://localhost:11434") {
		t.Fatalf("missing drone report:\n%s", out)
	}
	if len(fake.endpoints) != 2 || fake.endpoints[0].Model != "gpt-oss:20b" || fake.endpoints[1].Model != "qwen3:8b" {
		t.Fatalf("endpoints = %#v", fake.endpoints)
	}
}

func TestDoctorModelsFailsOnFailedCheck(t *testing.T) {
	isolateEnv(t)
	withDoctorHealth(t, &fakeEndpointHealth{
		reports: []llm.HealthReport{
			{
				Transport: "ollama",
				Model:     "missing:latest",
				Checks: []llm.HealthCheck{
					{Name: "model", Status: llm.HealthFail, Detail: "configured model not found", Action: "ollama pull missing:latest"},
				},
			},
			{
				Transport: "ollama",
				Model:     "qwen3:8b",
				Checks:    []llm.HealthCheck{{Name: "model", Status: llm.HealthOK}},
			},
		},
	})

	out, stderr, err := executeRootErr(t, append(configArgs(t), "doctor", "models"), "")
	if err == nil || !strings.Contains(err.Error(), "model doctor found failures") {
		t.Fatalf("err = %v stderr=%s", err, stderr)
	}
	if !strings.Contains(out, "action: ollama pull missing:latest") {
		t.Fatalf("missing action:\n%s", out)
	}
}

type fakeEndpointHealth struct {
	reports   []llm.HealthReport
	endpoints []llm.EndpointConfig
}

func (f *fakeEndpointHealth) Check(_ context.Context, endpoint llm.EndpointConfig) llm.HealthReport {
	f.endpoints = append(f.endpoints, endpoint)
	if len(f.reports) == 0 {
		return llm.HealthReport{Transport: endpoint.Transport}
	}
	report := f.reports[0]
	f.reports = f.reports[1:]
	return report
}

func withDoctorHealth(t *testing.T, checker endpointHealth) {
	t.Helper()
	old := doctorHealthChecker
	doctorHealthChecker = checker
	t.Cleanup(func() { doctorHealthChecker = old })
}

func executeRootErr(t *testing.T, args []string, input string) (string, string, error) {
	t.Helper()
	resetCLIState(t)
	defer resetCLIState(t)

	var stdout, stderr bytes.Buffer
	rootCmd.SetArgs(args)
	rootCmd.SetIn(strings.NewReader(input))
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	err := rootCmd.ExecuteContext(context.Background())
	return stdout.String(), stderr.String(), err
}
