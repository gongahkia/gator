package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	doctorpkg "github.com/gongahkia/paw/internal/doctor"
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

func TestDoctorJSONOutput(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	chdir(t, dir)

	out := executeRoot(t, append(configArgs(t), "doctor", "--json", "--only", "system.runtime"), "")
	var report doctorpkg.Report
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("json report: %v\n%s", err, out)
	}
	if report.SchemaVersion != 1 || len(report.Findings) != 1 || report.Findings[0].ID != "system.runtime" {
		t.Fatalf("report = %#v", report)
	}
}

func TestDoctorJSONGolden(t *testing.T) {
	report := doctorpkg.Report{
		SchemaVersion: 1,
		Version:       "dev",
		CWD:           "/work",
		Summary:       doctorpkg.Summary{OK: 1, Warning: 1, Error: 1, Fixed: 1, Planned: 1, Skipped: 1},
		Findings: []doctorpkg.Finding{
			{ID: "system.runtime", Section: "system", Severity: doctorpkg.SeverityInfo, Status: doctorpkg.StatusOK, Message: "runtime detected", Detail: "darwin/arm64", Metadata: map[string]string{"go": "go1.26.5"}},
			{ID: "config.bootstrap", Section: "config", Severity: doctorpkg.SeverityInfo, Status: doctorpkg.StatusPlanned, Message: "would create config", Path: "/work/.paw/config.toml"},
		},
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	assertGoldenText(t, filepath.Join("doctor", "report.json"), string(data)+"\n")
}

func TestDoctorLintFailsOnWarning(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	chdir(t, dir)

	out, stderr, err := executeRootErr(t, append(configArgs(t), "doctor", "--lint", "--only", "system.paw_dir"), "")
	if err == nil || !strings.Contains(err.Error(), "doctor lint found issues") {
		t.Fatalf("err = %v stderr=%s out=%s", err, stderr, out)
	}
	if !strings.Contains(out, ".paw directory is missing") {
		t.Fatalf("missing warning:\n%s", out)
	}
}

func TestDoctorFixCreatesRepoConfigAndPawDir(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	chdir(t, dir)
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	withDoctorHealth(t, &fakeEndpointHealth{
		reports: []llm.HealthReport{
			{Transport: "ollama", Checks: []llm.HealthCheck{{Name: "running", Status: llm.HealthOK}}},
			{Transport: "ollama", Checks: []llm.HealthCheck{{Name: "running", Status: llm.HealthOK}}},
		},
	})

	out := executeRoot(t, []string{"doctor", "--fix", "--yes", "--only", "config.bootstrap"}, "")
	if !strings.Contains(out, "created config") {
		t.Fatalf("missing repair output:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".paw", "config.toml")); err != nil {
		t.Fatalf("missing repo config: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, ".paw", "config.toml"))
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("config mode = %o", got)
	}
	ops, err := doctorpkg.ReadOperations(dir, 10)
	if err != nil {
		t.Fatalf("read ops: %v", err)
	}
	if len(ops) != 1 || ops[0].Action != "create config" || ops[0].Status != "ok" {
		t.Fatalf("ops = %#v", ops)
	}
}

func TestDoctorDryRunDoesNotCreateConfigOrLog(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	chdir(t, dir)
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	out := executeRoot(t, []string{"doctor", "--fix", "--dry-run", "--only", "config.bootstrap"}, "")
	if !strings.Contains(out, "would create config") || !strings.Contains(out, "plan=1") {
		t.Fatalf("missing dry-run plan:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".paw", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created config: %v", err)
	}
	ops, err := doctorpkg.ReadOperations(dir, 10)
	if err != nil {
		t.Fatalf("read ops: %v", err)
	}
	if len(ops) != 0 {
		t.Fatalf("dry-run wrote ops = %#v", ops)
	}
}

func TestDoctorFixPreservesConfigModeAndWritesBackup(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	chdir(t, dir)
	path := filepath.Join(dir, "bad.toml")
	if err := os.WriteFile(path, []byte("%%%"), 0o640); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out := executeRoot(t, []string{"--config", path, "doctor", "--fix", "--yes", "--only", "config.load"}, "")
	if !strings.Contains(out, "repaired config") {
		t.Fatalf("missing repair output:\n%s", out)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("config mode = %o", got)
	}
	if got := readTestFile(t, dir, "bad.toml.bak"); got != "%%%" {
		t.Fatalf("backup = %q", got)
	}
}

func TestDoctorFixPromptDeclineSkipsRepair(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	chdir(t, dir)
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	out, stderr, err := executeRootErr(t, []string{"doctor", "--fix", "--only", "config.bootstrap"}, "n\n")
	if err != nil {
		t.Fatalf("doctor prompt: %v stderr=%s", err, stderr)
	}
	if !strings.Contains(stderr, "apply repair create config") || !strings.Contains(out, "skipped config creation") {
		t.Fatalf("prompt output mismatch stdout=%s stderr=%s", out, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".paw", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("declined repair created config: %v", err)
	}
}

func TestDoctorHistoryAndExplain(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	chdir(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, ".paw", "doctor"), 0o755); err != nil {
		t.Fatalf("mkdir log: %v", err)
	}
	if err := doctorpkg.AppendOperation(dir, doctorpkg.Operation{Timestamp: "2026-07-08T00:00:00Z", Action: "create config", Status: "ok", Path: "config.toml"}); err != nil {
		t.Fatalf("append op: %v", err)
	}

	history := executeRoot(t, []string{"doctor", "history"}, "")
	if !strings.Contains(history, "create config") {
		t.Fatalf("history = %s", history)
	}
	explain := executeRoot(t, []string{"doctor", "explain", "config.bootstrap"}, "")
	if !strings.Contains(explain, "Paw can run on defaults") {
		t.Fatalf("explain = %s", explain)
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
