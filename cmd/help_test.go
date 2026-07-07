package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHelpGolden(t *testing.T) {
	isolateEnv(t)
	tests := []struct {
		name string
		args []string
	}{
		{name: "root", args: nil},
		{name: "bench", args: []string{"bench"}},
		{name: "compress", args: []string{"compress"}},
		{name: "doctor", args: []string{"doctor"}},
		{name: "doctor_models", args: []string{"doctor", "models"}},
		{name: "edit", args: []string{"edit"}},
		{name: "gather", args: []string{"gather"}},
		{name: "models", args: []string{"models"}},
		{name: "models_list", args: []string{"models", "list"}},
		{name: "plan", args: []string{"plan"}},
		{name: "resume", args: []string{"resume"}},
		{name: "run", args: []string{"run"}},
		{name: "verify", args: []string{"verify"}},
		{name: "version", args: []string{"version"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{}, tt.args...)
			args = append(args, "--help")
			out, stderr, err := executeRootErr(t, args, "")
			if err != nil {
				t.Fatalf("help %v: %v stderr=%s", args, err, stderr)
			}
			assertGoldenText(t, filepath.Join("help", tt.name+".txt"), out)
		})
	}
}

func assertGoldenText(t *testing.T, rel, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", rel)
	if os.Getenv("PAW_UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Fatalf("%s mismatch\ngot:\n%s\nwant:\n%s", rel, got, want)
	}
}
