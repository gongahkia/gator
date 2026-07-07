package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/paw/internal/envelope"
)

func TestProgressWritesPlainNonTTYLine(t *testing.T) {
	var out bytes.Buffer
	env := envelope.NewEnvelope("task", "target", "/repo")
	env.Raw = &envelope.RawContext{Units: []envelope.RawUnit{{Kind: "file_slice"}}}
	progress := NewProgress(&out)

	progress.StageDone("gather", env, 1200*time.Millisecond)

	got := out.String()
	if got != "[gather] collected 1 units (1.2s)\n" {
		t.Fatalf("progress = %q", got)
	}
	if progress.IsTTY() {
		t.Fatal("buffer detected as tty")
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("progress contains ANSI: %q", got)
	}
}

func TestProgressQuietSuppressesOutput(t *testing.T) {
	var out bytes.Buffer
	NewProgress(&out, WithQuiet(true)).StageDone("plan", envelope.NewEnvelope("task", "target", "/repo"), time.Second)
	if out.String() != "" {
		t.Fatalf("quiet progress = %q", out.String())
	}
}
