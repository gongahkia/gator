package watch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindMarkersCommentStyles(t *testing.T) {
	data := []byte(strings.Join([]string{
		"# ai: fix shell",
		"// ai: fix go",
		"/* ai: fix css */",
		"<!-- ai: fix html -->",
		"// TODO(ai): custom",
	}, "\n"))
	got, err := FindMarkers("file.txt", data, MarkerOptions{
		Markers:      []string{"ai:", "TODO(ai):"},
		ContextLines: 1,
		DisplayPath:  "file.txt",
	})
	if err != nil {
		t.Fatalf("find markers: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("markers = %#v", got)
	}
	for _, want := range []string{"fix shell", "fix go", "fix css", "fix html", "custom"} {
		if !strings.Contains(gotText(got), want) {
			t.Fatalf("missing %q in %#v", want, got)
		}
	}
	if !strings.Contains(got[1].Instruction, "file.txt:2") || !strings.Contains(got[1].Instruction, "Context:") {
		t.Fatalf("instruction = %q", got[1].Instruction)
	}
}

func TestRemoveMarkersPreservesInlineCode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	data := []byte("func f() { return 1 } // ai: fix\n// ai: remove me\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	markers, err := FindMarkers(path, data, MarkerOptions{})
	if err != nil {
		t.Fatalf("find markers: %v", err)
	}
	if err := RemoveMarkers(path, markers); err != nil {
		t.Fatalf("remove markers: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	want := "func f() { return 1 }\n"
	if string(got) != want {
		t.Fatalf("file = %q want %q", got, want)
	}
}

func gotText(markers []Marker) string {
	var out strings.Builder
	for _, marker := range markers {
		out.WriteString(marker.Text)
		out.WriteByte('\n')
	}
	return out.String()
}
