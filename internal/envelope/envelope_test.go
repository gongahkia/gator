package envelope

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	env := NewEnvelope("task-1", "fix the bug", "/work")
	env.Stage = "verify"
	env.Turn = 3
	env.Digest = &ContextDigest{
		Summary: "important context",
		Items: []DigestItem{
			{
				UnitID:    "u001",
				Path:      "main.go",
				Relevance: 90,
				Spans: []DigestSpan{
					{StartLine: 10, EndLine: 12, Quote: "return nil"},
				},
			},
		},
	}
	env.Plan = &Plan{
		Done:      false,
		Reasoning: "needs edit",
		NextAction: &NextAction{
			Kind:        "edit_file",
			Description: "update return path",
			TargetPath:  "main.go",
		},
	}
	env.Patch = &Patch{
		UnifiedDiff: "--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n",
		Files:       []string{"main.go"},
		Note:        "one-line fix",
	}
	env.Verify = &VerifyResult{
		Passed:        false,
		ExitCode:      1,
		Command:       "go test ./...",
		FailureDigest: "FAIL",
		RawTailBytes:  128,
		StdoutBytes:   64,
		StderrBytes:   64,
		TimedOut:      true,
	}
	env.Budget = Budget{
		MaxTurns:          40,
		Turn:              3,
		BrainInputTokens:  100,
		BrainOutputTokens: 20,
		BrainTokenSource:  "provider",
		DroneTokens:       30,
		DroneTokenSource:  "estimate",
		MaxBrainTokens:    200000,
	}
	env.Raw = &RawContext{
		Units: []RawUnit{
			{ID: "u001", Kind: "file_slice", Path: "main.go", StartLine: 1, EndLine: 3, Text: "package main\n"},
		},
		TotalBytes: 13,
	}
	env.Done = true

	var buf bytes.Buffer
	if err := env.Marshal(&buf); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := Unmarshal(&buf)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(env, got) {
		t.Fatalf("round trip mismatch\nwant: %#v\ngot:  %#v", env, got)
	}
}

func TestUnmarshalRejectsVersionMismatch(t *testing.T) {
	input := strings.NewReader(`{"schema_version":"paw.env/0","task_id":"task-1","instruction":"x","cwd":"/work","stage":"","turn":0,"budget":{},"done":false}`)
	_, err := Unmarshal(input)
	if !errors.Is(err, ErrSchemaVersion) {
		t.Fatalf("expected ErrSchemaVersion, got %v", err)
	}
}
