package gather

import (
	"strconv"
	"strings"
	"testing"

	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/envelope"
)

func TestCollectSyntaxContextsFunction(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "calc.go", strings.Join([]string{
		"package fixture",
		"",
		"func Add(a int, b int) int {",
		"\ttotal := a + b",
		"\treturn total",
		"}",
		"",
	}, "\n"))
	units, err := collectSyntaxContexts(dir, []envelope.RawUnit{syntaxHit("calc.go", 5, "return total")}, 4096)
	if err != nil {
		t.Fatalf("syntax contexts: %v", err)
	}
	if len(units) != 1 || units[0].Kind != "syntax_context" || units[0].StartLine != 3 || units[0].EndLine != 6 {
		t.Fatalf("units = %#v", units)
	}
	if !strings.Contains(units[0].Text, "func Add") || !strings.Contains(units[0].Text, "return total") {
		t.Fatalf("context text = %q", units[0].Text)
	}
}

func TestCollectSyntaxContextsMethod(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "counter.go", strings.Join([]string{
		"package fixture",
		"",
		"type Counter struct{ n int }",
		"",
		"func (c Counter) Inc() int {",
		"\treturn c.n + 1",
		"}",
		"",
	}, "\n"))
	units, err := collectSyntaxContexts(dir, []envelope.RawUnit{syntaxHit("counter.go", 6, "return c.n + 1")}, 4096)
	if err != nil {
		t.Fatalf("syntax contexts: %v", err)
	}
	if len(units) != 1 || !strings.Contains(units[0].Text, "func (c Counter) Inc() int") {
		t.Fatalf("units = %#v", units)
	}
}

func TestCollectSyntaxContextsTypeDeclaration(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", strings.Join([]string{
		"package fixture",
		"",
		"type Config struct {",
		"\tName string",
		"}",
		"",
	}, "\n"))
	units, err := collectSyntaxContexts(dir, []envelope.RawUnit{syntaxHit("types.go", 4, "Name string")}, 4096)
	if err != nil {
		t.Fatalf("syntax contexts: %v", err)
	}
	if len(units) != 1 || units[0].StartLine != 3 || units[0].EndLine != 5 || !strings.Contains(units[0].Text, "type Config struct") {
		t.Fatalf("units = %#v", units)
	}
}

func TestCollectSyntaxContextsDedupesAndCapsBytes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "long.go", strings.Join([]string{
		"package fixture",
		"",
		"func Long() string {",
		"\tvalue := \"" + strings.Repeat("x", 80) + "\"",
		"\treturn value",
		"}",
		"",
	}, "\n"))
	hit := envelope.RawUnit{
		Kind:      "search_hits",
		Path:      "long.go",
		StartLine: 4,
		EndLine:   5,
		Text:      "4: value :=\n5: return value\n",
	}
	units, err := collectSyntaxContexts(dir, []envelope.RawUnit{hit, hit}, 48)
	if err != nil {
		t.Fatalf("syntax contexts: %v", err)
	}
	if len(units) != 1 {
		t.Fatalf("units = %#v", units)
	}
	if len(units[0].Text) > 48 {
		t.Fatalf("context exceeded max bytes: %#v", units[0])
	}
}

func TestGatherSyntaxParseErrorFallsBackToLineSlice(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "broken.go", "package fixture\nfunc Broken( {\n\treturn target\n}\n")
	got := gatherDir(t, dir, config.GatherConfig{MaxDepth: 2, MaxFileBytes: 512}, "inspect target")
	if hasKind(got.Raw, "syntax_context") {
		t.Fatalf("parse error emitted syntax context: %#v", got.Raw.Units)
	}
	if !hasKind(got.Raw, "file_slice") {
		t.Fatalf("parse error did not fall back to file slice: %#v", got.Raw.Units)
	}
}

func syntaxHit(path string, line int, text string) envelope.RawUnit {
	return envelope.RawUnit{
		Kind:      "search_hits",
		Path:      path,
		StartLine: line,
		EndLine:   line,
		Text:      strconv.Itoa(line) + ": " + text + "\n",
	}
}
