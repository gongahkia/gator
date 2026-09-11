package worktui

import (
	"errors"
	"strings"
	"testing"
)

func TestCodeStatusShowsSelectedModelAndAccess(t *testing.T) {
	m := New(Config{ModelStatus: func() (ModelStatus, error) {
		return ModelStatus{Provider: "codex", Model: "gpt-5.6", Access: "logged in (Gator OAuth credential)"}, nil
	}})
	status := m.codeStatus()
	for _, expected := range []string{
		"model: codex / gpt-5.6",
		"model access: logged in (Gator OAuth credential)",
		"sandbox/network: strict/deny",
	} {
		if !strings.Contains(status, expected) {
			t.Fatalf("status omitted %q:\n%s", expected, status)
		}
	}
}

func TestCodeStatusShowsUnconfiguredAndSafeErrors(t *testing.T) {
	unconfigured := New(Config{ModelStatus: func() (ModelStatus, error) {
		return ModelStatus{Access: "not configured"}, nil
	}}).codeStatus()
	if !strings.Contains(unconfigured, "model: none") || !strings.Contains(unconfigured, "model access: not configured") {
		t.Fatalf("unconfigured status = %q", unconfigured)
	}

	failed := New(Config{ModelStatus: func() (ModelStatus, error) {
		return ModelStatus{}, errors.New("cannot read\nconfiguration")
	}}).codeStatus()
	if !strings.Contains(failed, "model: unavailable") || !strings.Contains(failed, "status unavailable: cannot read configuration") {
		t.Fatalf("failed status = %q", failed)
	}
}
