package egress

import (
	"strings"
	"testing"

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
