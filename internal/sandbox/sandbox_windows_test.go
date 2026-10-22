package sandbox

import (
	"context"
	"strings"
	"testing"
)

func TestWindowsDefaultPolicyFailsClosed(t *testing.T) {
	prepared, err := Prepare(context.Background(), Request{Dir: t.TempDir(), Argv: []string{"must-not-run.exe"}, Policy: DefaultPolicy()})
	if err == nil {
		prepared.Cleanup()
		t.Fatal("strict policy launched without Windows isolation")
	}
	if !strings.Contains(err.Error(), "Windows preview") {
		t.Fatalf("unhelpful capability error: %v", err)
	}
	if _, err := StrictAvailability(); err == nil {
		t.Fatal("claimed Windows strict sandbox support")
	}
}
