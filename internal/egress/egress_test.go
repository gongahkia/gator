package egress

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/envelope"
)

func TestBuildReportsSecretKindsWithoutValues(t *testing.T) {
	raw := &envelope.RawContext{Units: []envelope.RawUnit{{ID: "u001", Path: "config.env", Kind: "file_slice", Text: "key=ghp_abcdefghijklmnopqrstuvwxyz123456\n"}}}
	manifest := Build(raw)
	if len(manifest.Findings) != 1 || manifest.Findings[0].Kind != "github_token" || manifest.Findings[0].Count != 1 {
		t.Fatalf("findings = %#v", manifest.Findings)
	}
	if strings.Contains(manifest.Units[0].SHA256, "ghp_") {
		t.Fatal("manifest leaked token")
	}
}

func TestDetectReportsHighConfidenceSecretKinds(t *testing.T) {
	raw := &envelope.RawContext{Units: []envelope.RawUnit{{Text: strings.Join([]string{
		"ghp_abcdefghijklmnopqrst",
		"sk-abcdefghijklmnopqrst",
		"AKIAABCDEFGHIJKLMNOP",
		"-----BEGIN PRIVATE KEY-----\nkey\n-----END PRIVATE KEY-----",
		"eyJabcdefghij.abcdefghij.abcdefghij",
	}, "\n")}}}
	findings := Detect(raw)
	if got := findingsByKind(findings); !equalFindings(got, map[string]int{
		"github_token":   1,
		"openai_key":     1,
		"aws_access_key": 1,
		"private_key":    1,
		"jwt":            1,
	}) {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestDetectIgnoresNearMisses(t *testing.T) {
	raw := &envelope.RawContext{Units: []envelope.RawUnit{{Text: "sk-short AKIA123 eyJshort.short.short"}}}
	if findings := Detect(raw); len(findings) != 0 {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestRedactRemovesDetectedValues(t *testing.T) {
	raw := &envelope.RawContext{Units: []envelope.RawUnit{{ID: "u001", Text: "token=sk-abcdefghijklmnopqrstuvwxyz123456"}}}
	result := Redact(raw)
	if strings.Contains(result.Raw.Units[0].Text, "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("redaction leaked secret: %q", result.Raw.Units[0].Text)
	}
	if !strings.Contains(result.Raw.Units[0].Text, "[REDACTED:openai_key]") {
		t.Fatalf("redaction = %q", result.Raw.Units[0].Text)
	}
}

func TestRedactForEgressKeepsFindingWithoutSecretPayload(t *testing.T) {
	secret := "sk-abcdefghijklmnopqrstuvwxyz123456"
	raw := &envelope.RawContext{Units: []envelope.RawUnit{{ID: "u001", Text: "token=" + secret}}}
	result, manifest := RedactForEgress(raw)
	if strings.Contains(result.Raw.Units[0].Text, secret) {
		t.Fatalf("redacted raw = %q", result.Raw.Units[0].Text)
	}
	if len(manifest.Findings) != 1 || manifest.Findings[0].Kind != "openai_key" {
		t.Fatalf("manifest findings = %#v", manifest.Findings)
	}
	if strings.Contains(manifest.Units[0].SHA256, secret) {
		t.Fatalf("manifest leaked secret: %#v", manifest)
	}
}

func TestEnforceUsesConfiguredLimits(t *testing.T) {
	manifest := Manifest{Units: []UnitManifest{{ID: "u001"}, {ID: "u002"}}, TotalBytes: 10}
	if err := Enforce(config.EgressPolicy{MaxFiles: 1, MaxBytes: 20}, manifest); err == nil {
		t.Fatal("expected file limit error")
	}
	if err := Enforce(config.EgressPolicy{MaxFiles: 2, MaxBytes: 9}, manifest); err == nil {
		t.Fatal("expected byte limit error")
	}
	if err := Enforce(config.EgressPolicy{MaxFiles: 2, MaxBytes: 10}, manifest); err != nil {
		t.Fatalf("within limits: %v", err)
	}
}

func TestPrepareBlocksSecretsByDefaultPolicy(t *testing.T) {
	raw := &envelope.RawContext{Units: []envelope.RawUnit{{ID: "u001", Text: "token=sk-abcdefghijklmnopqrstuvwxyz123456"}}}
	_, _, err := Prepare(config.Defaults().Policy.Egress, raw)
	if err == nil || !strings.Contains(err.Error(), "egress blocked") {
		t.Fatalf("error = %v", err)
	}
}

func TestPrepareRedactsWhenSecretBlockingIsExplicitlyDisabled(t *testing.T) {
	raw := &envelope.RawContext{Units: []envelope.RawUnit{{ID: "u001", Text: "token=sk-abcdefghijklmnopqrstuvwxyz123456"}}}
	cfg := config.Defaults().Policy.Egress
	cfg.BlockSecrets = false
	result, _, err := Prepare(cfg, raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Raw.Units[0].Text, "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("raw = %q", result.Raw.Units[0].Text)
	}
}

func TestProviderApprovalReceiptBindsManifestAndDestination(t *testing.T) {
	manifest := Manifest{Units: []UnitManifest{{ID: "u001", Kind: "file_slice", Bytes: 3, SHA256: "abc"}}, TotalBytes: 3}
	approved, err := Approve(manifest, "openai", "https://api.example.test/v1", time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyProviderApproval(approved, "openai", "https://api.example.test/v1"); err != nil {
		t.Fatalf("verify receipt: %v", err)
	}
	if err := VerifyProviderApproval(approved, "openai", "https://other.example.test/v1"); !errors.Is(err, ErrInvalidProviderApproval) {
		t.Fatalf("other endpoint error = %v", err)
	}
	approved.TotalBytes++
	if err := VerifyProviderApproval(approved, "openai", "https://api.example.test/v1"); !errors.Is(err, ErrInvalidProviderApproval) {
		t.Fatalf("tampered manifest error = %v", err)
	}
}

func TestProviderApprovalReceiptRejectsMissingFields(t *testing.T) {
	manifest := Manifest{}
	if _, err := NewProviderApprovalReceipt(manifest, "", "https://api.example.test/v1", time.Now(), false); !errors.Is(err, ErrInvalidProviderApproval) {
		t.Fatalf("missing transport error = %v", err)
	}
	if _, err := NewProviderApprovalReceipt(manifest, "openai", "https://api.example.test/v1", time.Time{}, false); !errors.Is(err, ErrInvalidProviderApproval) {
		t.Fatalf("missing timestamp error = %v", err)
	}
	if err := VerifyProviderApproval(manifest, "openai", "https://api.example.test/v1"); !errors.Is(err, ErrInvalidProviderApproval) {
		t.Fatalf("missing receipt error = %v", err)
	}
}

func findingsByKind(findings []Finding) map[string]int {
	result := make(map[string]int, len(findings))
	for _, finding := range findings {
		result[finding.Kind] = finding.Count
	}
	return result
}

func equalFindings(got, want map[string]int) bool {
	if len(got) != len(want) {
		return false
	}
	for kind, count := range want {
		if got[kind] != count {
			return false
		}
	}
	return true
}
