package log

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONLoggerRecordsContextFields(t *testing.T) {
	var out bytes.Buffer
	logger, err := New(Config{Format: FormatJSON, Level: "debug", Writer: &out})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	ctx := With(context.Background(), logger)
	From(ctx).DebugContext(ctx, "stage trace", "stage", "gather")

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &record); err != nil {
		t.Fatalf("decode log: %v\n%s", err, out.String())
	}
	if record["level"] != "DEBUG" || record["msg"] != "stage trace" || record["stage"] != "gather" {
		t.Fatalf("record = %#v", record)
	}
}

func TestTextLoggerFiltersDebugByDefault(t *testing.T) {
	var out bytes.Buffer
	logger, err := New(Config{Writer: &out})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	logger.Debug("hidden", "stage", "gather")
	if out.Len() != 0 {
		t.Fatalf("debug output = %q", out.String())
	}
	logger.Info("shown", "stage", "gather")
	got := out.String()
	if !strings.Contains(got, "level=INFO") || !strings.Contains(got, "msg=shown") || !strings.Contains(got, "stage=gather") {
		t.Fatalf("info output = %q", got)
	}
}

func TestInvalidConfig(t *testing.T) {
	if _, err := New(Config{Format: "xml"}); err == nil {
		t.Fatal("expected format error")
	}
	if _, err := New(Config{Level: "trace"}); err == nil {
		t.Fatal("expected level error")
	}
}
