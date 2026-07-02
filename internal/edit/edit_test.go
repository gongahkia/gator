package edit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm"
	"github.com/gongahkia/paw/internal/llm/faketest"
)

func TestEditAppliesGoodDiff(t *testing.T) {
	dir := fixtureRepo(t)
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondOpenAI("", goodDiff())

	got, err := New(llm.NewOpenAIClient(srv.URL, "key", "brain")).Run(context.Background(), editEnvelope(dir))
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if got.Patch == nil || got.Patch.Files[0] != "calc.go" {
		t.Fatalf("patch = %#v", got.Patch)
	}
	if got.Budget.BrainInputTokens != 11 || got.Budget.BrainOutputTokens != 7 {
		t.Fatalf("budget = %#v", got.Budget)
	}
	content := readFile(t, filepath.Join(dir, "calc.go"))
	if !strings.Contains(content, "return a + b") {
		t.Fatalf("file not edited:\n%s", content)
	}
}

func TestEditRetriesOnceThenReturnsStructuredFailure(t *testing.T) {
	dir := fixtureRepo(t)
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondOpenAI("", badDiff())
	srv.RespondOpenAI("", badDiff())

	_, err := New(llm.NewOpenAIClient(srv.URL, "key", "brain")).Run(context.Background(), editEnvelope(dir))
	var failure *ApplyFailure
	if !errors.As(err, &failure) {
		t.Fatalf("expected ApplyFailure, got %v", err)
	}
	if got := len(srv.Requests()); got != 2 {
		t.Fatalf("requests = %d", got)
	}
	content := readFile(t, filepath.Join(dir, "calc.go"))
	if strings.Contains(content, "MISSING") {
		t.Fatalf("bad patch changed file:\n%s", content)
	}
}

func editEnvelope(cwd string) *envelope.Envelope {
	env := envelope.NewEnvelope("task", "fix Add", cwd)
	env.Plan = &envelope.Plan{
		NextAction: &envelope.NextAction{
			Kind:        "edit_file",
			Description: "change Add to use addition",
			TargetPath:  "calc.go",
		},
	}
	env.Digest = &envelope.ContextDigest{Summary: "calc.go Add subtracts instead of adding"}
	return env
}

func fixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join("..", "gather", "testdata", "repo", "calc.go")
	data := readFile(t, src)
	if err := os.WriteFile(filepath.Join(dir, "calc.go"), []byte(data), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return dir
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func goodDiff() string {
	return strings.Join([]string{
		"--- a/calc.go",
		"+++ b/calc.go",
		"@@ -6,4 +6,4 @@",
		"",
		" func Add(a int, b int) int {",
		"-\treturn a - b",
		"+\treturn a + b",
		" }",
		"",
	}, "\n")
}

func badDiff() string {
	return strings.Join([]string{
		"--- a/calc.go",
		"+++ b/calc.go",
		"@@ -1,3 +1,3 @@",
		" package fixture",
		"-missing",
		"+MISSING",
		" func KnownSymbol() string {",
		"",
	}, "\n")
}
