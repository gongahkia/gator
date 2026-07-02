package verify

import (
	"context"
	"strings"
	"testing"

	"github.com/gongahkia/paw/internal/envelope"
)

func TestVerifyPassingCommand(t *testing.T) {
	stage := New("printf '%s\n' ok")
	env := &envelope.Envelope{Cwd: t.TempDir()}

	got, err := stage.Run(context.Background(), env)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got.Verify == nil {
		t.Fatal("verify result missing")
	}
	if !got.Verify.Passed {
		t.Fatalf("expected pass, got %#v", got.Verify)
	}
	if got.Verify.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", got.Verify.ExitCode)
	}
	if got.Verify.Command != stage.Command {
		t.Fatalf("command mismatch: %q", got.Verify.Command)
	}
}

func TestVerifyFailingCommandDigestKeepsMarkersAndCaps(t *testing.T) {
	stage := &Verify{
		Command: strings.Join([]string{
			"printf '%s\\n'",
			"'prefix one'",
			"'FAIL short'",
			"'expected got'",
			"'tail 012345678901234567890123456789'",
			"'tail 123456789012345678901234567890'",
			"; exit 7",
		}, " "),
		MaxDigestBytes: 64,
	}
	env := &envelope.Envelope{Cwd: t.TempDir()}

	got, err := stage.Run(context.Background(), env)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got.Verify == nil {
		t.Fatal("verify result missing")
	}
	if got.Verify.Passed {
		t.Fatalf("expected failure, got %#v", got.Verify)
	}
	if got.Verify.ExitCode != 7 {
		t.Fatalf("expected exit 7, got %d", got.Verify.ExitCode)
	}
	if !strings.Contains(got.Verify.FailureDigest, "FAIL short") {
		t.Fatalf("digest missing FAIL marker: %q", got.Verify.FailureDigest)
	}
	if !strings.Contains(got.Verify.FailureDigest, "expected got") {
		t.Fatalf("digest missing expected/got marker: %q", got.Verify.FailureDigest)
	}
	if len(got.Verify.FailureDigest) > stage.MaxDigestBytes {
		t.Fatalf("digest exceeded cap: %d > %d", len(got.Verify.FailureDigest), stage.MaxDigestBytes)
	}
	if got.Verify.RawTailBytes <= len(got.Verify.FailureDigest) {
		t.Fatalf("raw bytes not recorded before truncation: raw=%d digest=%d", got.Verify.RawTailBytes, len(got.Verify.FailureDigest))
	}
}
