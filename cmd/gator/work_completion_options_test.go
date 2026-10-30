package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestWorkCommandRetainsExplicitCompletionChecksAndScopes(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	t.Setenv("GATOR_PROVIDER", "openai")
	t.Setenv("GATOR_MODEL", "test-model")
	m := &workScriptedModel{turns: []agent.Turn{{Text: "Inspected."}}}
	var output bytes.Buffer
	err := runWorkTask([]string{"--source", t.TempDir(), "--mode", "inspect", "--run-id", "completion-options", "--verify", "python3 -B verify.py", "--scope", "normalize.py", "Inspect only"}, strings.NewReader(""), &output, func(_, _, _ string) (agent.Model, error) { return m, nil })
	if err != nil {
		t.Fatal(err)
	}
	history, err := workhistory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	run, err := history.Load("completion-options")
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := worksession.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := sessions.LoadRevision(run.ConversationID, run.ID)
	if err != nil || revision.Replay == nil {
		t.Fatalf("configuration missing: %v", err)
	}
	var configuration struct {
		Code struct {
			Verification [][]string
			Scopes       []string
		}
	}
	if err := json.Unmarshal(revision.Replay.Configuration, &configuration); err != nil {
		t.Fatal(err)
	}
	checks := configuration.Code.Verification
	if len(checks) != 1 || strings.Join(checks[0], " ") != "python3 -B verify.py" || len(configuration.Code.Scopes) != 1 || configuration.Code.Scopes[0] != "normalize.py" {
		t.Fatalf("lost explicit check/scope grants: %+v", configuration.Code)
	}
}

func TestWorkCommandShowsRetainedCompletionEvidence(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	directory := filepath.Join(state, "gator", "completions", "completed-run")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	id := "completion-" + strings.Repeat("a", 32)
	data, _ := json.Marshal(map[string]any{"version": 1, "id": id, "run_id": "completed-run", "status": "completed", "objective": "Fix a parser", "attempts": []any{map[string]any{"number": 1, "feedback": "First check failed"}, map[string]any{"number": 2, "stage": "completed"}}})
	if err := os.WriteFile(filepath.Join(directory, id+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if !isWorkSessionCommand("completions") {
		t.Fatal("missing public command route")
	}
	var output bytes.Buffer
	if err := workSessionCommand([]string{"completions", "completed-run", id}, nil, &output, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "First check failed") || !strings.Contains(output.String(), "Fix a parser") {
		t.Fatal("missing retained loop evidence", output.String())
	}
	if err := workSessionCommand([]string{"completions", "../../outside"}, nil, &output, nil); err == nil {
		t.Fatal("accepted traversal")
	}
}
