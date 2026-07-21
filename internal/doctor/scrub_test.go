package doctor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddScrubsDiagnosticSecrets(t *testing.T) {
	secret := "sk-abcdefghijklmnopqrstuvwxyz123456"
	r := runner{opts: Options{SeverityMin: SeverityInfo}}
	r.add(Finding{ID: "test", Section: "test", Severity: SeverityError, Status: StatusFail, Message: secret, Detail: secret, Fix: secret, Command: secret, Path: secret, Metadata: map[string]string{secret: secret}})
	data, err := json.Marshal(r.report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) || !strings.Contains(string(data), "[REDACTED:openai_key]") {
		t.Fatalf("report = %s", data)
	}
}

func TestPolicyPreflightAppearsInJSONReport(t *testing.T) {
	report := Run(context.Background(), Options{CWD: t.TempDir(), Only: []string{"policy"}})
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"section":"policy"`) || !hasFinding(report, "policy.validation", StatusOK) {
		t.Fatalf("report = %s", data)
	}
}

func TestPolicyPreflightReportsValidationDiagnostic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[policy.risk]\nmax_files = 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := Run(context.Background(), Options{CWD: dir, ConfigPath: path, Only: []string{"policy"}})
	if !hasFinding(report, "policy.validation", StatusFail) {
		t.Fatalf("report = %#v", report)
	}
}

func TestPolicyPreflightReportsDeniedConfiguredEndpoint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[brain]\ntransport = \"openai\"\nbase_url = \"https://api.example.test/v1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := Run(context.Background(), Options{CWD: dir, ConfigPath: path, Only: []string{"policy"}})
	if !hasFinding(report, "policy.endpoint.brain", StatusFail) {
		t.Fatalf("report = %#v", report)
	}
}

func hasFinding(report Report, id string, status Status) bool {
	for _, finding := range report.Findings {
		if finding.ID == id && finding.Status == status {
			return true
		}
	}
	return false
}
