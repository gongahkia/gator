package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestWorkTaskAnswerCommandRoutingAndInput(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	if !isWorkSessionCommand("answer-task") || !commandUsesPrivateData([]string{"work", "answer-task"}) {
		t.Fatal("answer-task missing public/private command routing")
	}
	for _, test := range []struct{ input, want string }{
		{`{"format":"Markdown"}`, "JSON object"},
		{strings.Repeat("x", 32*1024+1), "exceed"},
		{`{"format":["Markdown"]} {}`, "JSON object"},
		{`{}`, "invalid task answer"},
	} {
		var output bytes.Buffer
		err := workSessionCommand([]string{"answer-task", "run", "subagent-001", strings.Repeat("a", 64), "-"}, strings.NewReader(test.input), &output, nil)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("input accepted or wrong error: %v", err)
		}
	}
}
