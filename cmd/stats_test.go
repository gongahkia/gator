package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStatsGolden(t *testing.T) {
	isolateEnv(t)
	tracePath := filepath.Join("testdata", "trace", "stats.ndjson")
	out, stderr, err := executeRootErr(t, []string{
		"--trace-file", tracePath,
		"stats",
		"--rate", "brain:in=3.0,brain:out=15.0,brain:cache_creation=4.0,drone=0.25",
	}, "")
	if err != nil {
		t.Fatalf("stats: %v stderr=%s", err, stderr)
	}
	assertGoldenText(t, "stats.txt", out)
}

func TestStatsTaskIDUsesDefaultTracePath(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	chdir(t, dir)
	writeTestFile(t, dir, filepath.Join(".paw", "trace-task-stats.ndjson"), strings.Join([]string{
		`{"stage":"verify","duration_ms":25,"envelope":{"task_id":"task-stats","turn":1,"verify":{"passed":false,"exit_code":1,"command":"go test","failure_digest":"fail","raw_tail_bytes":4},"budget":{"turn":1,"brain_input_tokens":10,"brain_output_tokens":2,"drone_tokens":3}}}`,
		"",
	}, "\n"))
	out, stderr, err := executeRootErr(t, []string{"stats", "task-stats"}, "")
	if err != nil {
		t.Fatalf("stats task id: %v stderr=%s", err, stderr)
	}
	for _, want := range []string{"Task:      task-stats", "Verify:    failed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestStatsRequiresTaskOrTrace(t *testing.T) {
	isolateEnv(t)
	_, stderr, err := executeRootErr(t, []string{"stats"}, "")
	if err == nil || !strings.Contains(err.Error(), "expected task id or --trace-file") {
		t.Fatalf("err = %v stderr=%s", err, stderr)
	}
}
