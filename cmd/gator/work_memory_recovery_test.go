package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestMemoryCorruptionKeepsHealthyContextAndVisibleRecoveryAcrossSurfaces(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	store, err := memory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	healthy, err := store.Add("preference", "Prefer concise harbor briefings", "", "fixture", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	bad, err := store.Add("fact", "Must not enter model context", "", "fixture", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(state, "gator", "memory", bad.ID+".json")
	broken := []byte(`{"text":"Must not enter model context`)
	if err := os.WriteFile(path, broken, 0600); err != nil {
		t.Fatal(err)
	}
	for _, foreground := range []bool{false, true} {
		request := workrun.Request{Objective: "harbor briefings", ForegroundApp: foreground}
		if err := applyMemoryContext(state, &request); err != nil {
			t.Fatal("corruption blocked work preparation", err)
		}
		if !strings.Contains(request.MemoryContext, healthy.Text) || !strings.Contains(request.MemoryContext, "unreadable") || !strings.Contains(request.RecallStatus, "unreadable") || strings.Contains(request.MemoryContext, "Must not enter") || len(request.ContextItems) != 1 || request.ContextItems[0].ID != healthy.ID {
			t.Fatalf("incorrect partial recall: %+v", request)
		}
	}
	for _, args := range [][]string{{"list"}, {"search", "harbor"}, {"diagnose"}} {
		var output bytes.Buffer
		if err := memoryCommand(args, &output); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), "unreadable") || strings.Contains(output.String(), "Must not enter") {
			t.Fatalf("missing safe CLI diagnostics: %s", output.String())
		}
		if args[0] != "diagnose" && !strings.Contains(output.String(), healthy.Text) {
			t.Fatal("CLI omitted healthy record")
		}
	}
	view, err := readWorkContext(state, "", "")
	if err != nil || !strings.Contains(view.Recall, "unreadable") || len(view.Items) != 1 || view.Items[0].ID != healthy.ID {
		t.Fatalf("context view: %+v %v", view, err)
	}
	result, err := workMemoryResult(state, "", []string{"list"})
	if err != nil || result == nil || !strings.Contains(result.Summary, "unreadable") || len(result.Items) != 1 {
		t.Fatalf("memory result: %+v %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status, err := workRecallStatus(ctx, state)
	if err != nil || !strings.Contains(status.Storage, "unreadable") {
		t.Fatalf("memory settings: %+v %v", status, err)
	}
	retained, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(retained, broken) {
		t.Fatal("recovery inspection modified original evidence", err)
	}
}
