package workapp

import "testing"

func TestRestartReadinessNeverCancelsWorkOrDropsPendingMessages(t *testing.T) {
	cancelled := false
	for name, model := range map[string]Model{
		"running":         {running: true, cancel: func() { cancelled = true }},
		"cancelling":      {cancelling: true},
		"queued":          {queue: []queuedRun{{prompt: "Keep this request"}}},
		"setup prompt":    {pendingPrompt: "Keep this first request"},
		"external action": {bridgeExternal: &BridgeExternal{Kind: "login"}},
		"review":          {pendingBundleAction: &pendingBundleAction{}},
	} {
		t.Run(name, func(t *testing.T) {
			if model.RestartBlockReason() == "" {
				t.Fatal("active application accepted a restart")
			}
			if cancelled {
				t.Fatal("readiness cancelled the work")
			}
		})
	}
	if reason := New(Config{CurrentFolder: t.TempDir()}).RestartBlockReason(); reason != "" {
		t.Fatal(reason)
	}
}
