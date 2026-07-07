package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm/faketest"
	"github.com/gongahkia/paw/internal/stage"
)

func TestResumeFromTrace(t *testing.T) {
	isolateEnv(t)
	server := faketest.NewServer()
	t.Cleanup(server.Close)
	configureBrain(t, server.URL)
	server.RespondOpenAI("Prior VerifyResult JSON", `{"done":true,"reasoning":"done"}`)

	dir := t.TempDir()
	chdir(t, dir)
	writeTestFile(t, dir, "notes.txt", "target\n")
	tracePath := filepath.Join(dir, ".paw", "trace-task-resume.ndjson")
	env := envelope.NewEnvelope("task-resume", "target", dir)
	env.Stage = "gather"
	env.Turn = 2
	env.Budget.Turn = 2
	env.Budget.DroneTokens = 7
	env.Raw = &envelope.RawContext{
		Units: []envelope.RawUnit{{
			ID:        "u001",
			Kind:      "file_slice",
			Path:      "notes.txt",
			StartLine: 1,
			EndLine:   1,
			Text:      "target\n",
		}},
		TotalBytes: len("target\n"),
	}
	writeTrace(t, tracePath, stage.TraceEvent{Stage: "gather", Turn: 2, Envelope: env})

	stdout, stderr, err := executeRootErr(t, append(configArgs(t), "resume", "task-resume", "--raw-context", "--quiet"), "")
	if err != nil {
		t.Fatalf("resume: %v stderr=%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
	var got envelope.Envelope
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	if !got.Done || got.Stage != "plan" || got.Turn != 2 || got.Budget.DroneTokens != 7 {
		t.Fatalf("resumed envelope = %#v", got)
	}
	traceData, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	trace := string(traceData)
	if !strings.Contains(trace, `"stage":"compress"`) || !strings.Contains(trace, `"stage":"plan"`) {
		t.Fatalf("trace not appended:\n%s", trace)
	}
}

func TestResumeMalformedTraceFails(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	chdir(t, dir)
	tracePath := filepath.Join(dir, ".paw", "trace-task-bad.ndjson")
	if err := os.MkdirAll(filepath.Dir(tracePath), 0o755); err != nil {
		t.Fatalf("mkdir trace: %v", err)
	}
	if err := os.WriteFile(tracePath, []byte(`{"stage":"gather"`), 0o644); err != nil {
		t.Fatalf("write trace: %v", err)
	}

	_, stderr, err := executeRootErr(t, append(configArgs(t), "resume", "task-bad", "--quiet"), "")
	if err == nil || !strings.Contains(err.Error(), "read trace") {
		t.Fatalf("err = %v stderr=%s", err, stderr)
	}
}

func writeTrace(t *testing.T, path string, events ...stage.TraceEvent) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir trace: %v", err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create trace: %v", err)
	}
	enc := json.NewEncoder(file)
	for _, event := range events {
		if err := enc.Encode(event); err != nil {
			t.Fatalf("encode trace: %v", err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close trace: %v", err)
	}
}
