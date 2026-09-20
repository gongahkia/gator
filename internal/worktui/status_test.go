package worktui

import (
	"errors"
	"strings"
	"testing"
)

func TestWorkStatusShowsSelectedModelAndAccess(t *testing.T) {
	m := New(Config{ModelStatus: func() (ModelStatus, error) {
		return ModelStatus{Provider: "codex", Model: "gpt-5.6", Access: "logged in (Gator OAuth credential)"}, nil
	}})
	status := m.workStatus()
	for _, expected := range []string{
		"model: codex / gpt-5.6",
		"model access: logged in (Gator OAuth credential)",
		"internal specialists: managed by Gator",
	} {
		if !strings.Contains(status, expected) {
			t.Fatalf("status omitted %q:\n%s", expected, status)
		}
	}
}

func TestWorkStatusShowsUnconfiguredAndSafeErrors(t *testing.T) {
	unconfigured := New(Config{ModelStatus: func() (ModelStatus, error) {
		return ModelStatus{Access: "not configured"}, nil
	}}).workStatus()
	if !strings.Contains(unconfigured, "model: none") || !strings.Contains(unconfigured, "model access: not configured") {
		t.Fatalf("unconfigured status = %q", unconfigured)
	}

	failed := New(Config{ModelStatus: func() (ModelStatus, error) {
		return ModelStatus{}, errors.New("cannot read\nconfiguration")
	}}).workStatus()
	if !strings.Contains(failed, "model: unavailable") || !strings.Contains(failed, "status unavailable: cannot read configuration") {
		t.Fatalf("failed status = %q", failed)
	}
}
