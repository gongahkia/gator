package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gongahkia/gator/internal/workspace"
)

func TestReadFilePhysicalLinesPreserveCompleteByteCoverage(t *testing.T) {
	for _, text := range []string{"one\ntwo\n", "one\ntwo", "\n", "one\n\n"} {
		t.Run(text, func(t *testing.T) {
			path := t.TempDir()
			if err := os.WriteFile(filepath.Join(path, "source.txt"), []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			root, _ := workspace.Open(path)
			result, err := (ReadFile{Root: root}).Execute(context.Background(), []byte(`{"path":"source.txt"}`))
			if err != nil {
				t.Fatal(err)
			}
			var envelope struct {
				Result struct {
					Content    string `json:"content"`
					EndLine    int    `json:"end_line"`
					EndOffset  int    `json:"end_offset"`
					TotalBytes int    `json:"total_bytes"`
				} `json:"result"`
			}
			if err := json.Unmarshal([]byte(result.Content), &envelope); err != nil {
				t.Fatal(err)
			}
			wantLines := 2
			if text == "\n" {
				wantLines = 1
			}
			r := envelope.Result
			if r.Content != text || r.EndLine != wantLines || r.EndOffset != len(text) || r.TotalBytes != len(text) {
				t.Fatalf("physical lines or byte coverage incorrect: %+v", r)
			}
			args, _ := json.Marshal(map[string]any{"path": "source.txt", "start_line": wantLines + 1})
			if _, err := (ReadFile{Root: root}).Execute(context.Background(), args); err == nil {
				t.Fatal("read beyond physical final line accepted")
			}
		})
	}
}
