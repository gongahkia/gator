package doctor

import (
	"encoding/json"
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
