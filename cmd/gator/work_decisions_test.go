package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/workrun"
)

func TestWorkDecisionsShowsPendingAcrossRunsAndHistoryForOneRun(t *testing.T) {
	state := t.TempDir()
	for _, item := range []struct {
		run    string
		status string
	}{{"run-one", "pending"}, {"run-two", "cancelled"}} {
		dir := filepath.Join(state, "gator", "interactions", item.run)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(workrun.InteractionRecord{Version: 1, Status: item.status, Interaction: workrun.Interaction{ID: 1, Kind: "action", Preview: "private preview"}})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "1.json"), payload, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	if err := workDecisionsCommand(state, nil, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "run-one #1 pending (run unconfirmed) action") || strings.Contains(output.String(), "run-two") || strings.Contains(output.String(), "private preview") {
		t.Fatalf("pending view = %q", output.String())
	}
	if err := os.WriteFile(filepath.Join(state, "gator", "interactions", "run-one", "active.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := workDecisionsCommand(state, nil, &output); err != nil || !strings.Contains(output.String(), "run-one #1 pending (stale) action") {
		t.Fatalf("stopped pending view = %q, %v", output.String(), err)
	}
	output.Reset()
	if err := workDecisionsCommand(state, []string{"run-two", "--json"}, &output); err != nil {
		t.Fatal(err)
	}
	var result struct {
		LiveStatus string                      `json:"live_status"`
		Decisions  []workrun.InteractionRecord `json:"decisions"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.LiveStatus != "per_run_lock" || len(result.Decisions) != 1 || result.Decisions[0].Status != "cancelled" {
		t.Fatalf("retained decision JSON = %#v, %v", result, err)
	}
	denied := false
	for id := 2; id <= 261; id++ {
		payload, err := json.Marshal(workrun.InteractionRecord{Version: 1, Status: "resolved", Interaction: workrun.Interaction{ID: id, Kind: "action"}, Approved: &denied})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(state, "gator", "interactions", "run-two", fmt.Sprintf("%d.json", id)), payload, 0600); err != nil {
			t.Fatal(err)
		}
	}
	output.Reset()
	if err := workDecisionsCommand(state, nil, &output); err != nil || !strings.Contains(output.String(), "run-one #1 pending (stale) action") || strings.Contains(output.String(), "run-two") {
		t.Fatalf("pending view after resolved history = %q, %v", output.String(), err)
	}
}
