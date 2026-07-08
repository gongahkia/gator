package gather

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm"
)

func TestGatherFindsKnownSymbol(t *testing.T) {
	env := runGather(t, config.GatherConfig{MaxDepth: 3, MaxFileBytes: 4096}, "inspect KnownSymbol Add")
	if !rawContains(env.Raw, "KnownSymbol") {
		t.Fatalf("raw context did not contain KnownSymbol: %#v", env.Raw)
	}
	if !hasKind(env.Raw, "search_hits") {
		t.Fatalf("raw context missing search_hits: %#v", env.Raw)
	}
	if !hasKind(env.Raw, "file_slice") {
		t.Fatalf("raw context missing file_slice: %#v", env.Raw)
	}
	if !hasKind(env.Raw, "syntax_context") {
		t.Fatalf("raw context missing syntax_context: %#v", env.Raw)
	}
}

func TestGatherRespectsDepthAndByteBounds(t *testing.T) {
	env := runGather(t, config.GatherConfig{MaxDepth: 1, MaxFileBytes: 80}, "inspect KnownSymbol")
	for _, unit := range env.Raw.Units {
		if len(unit.Text) > 80 {
			t.Fatalf("unit exceeded byte bound: %#v", unit)
		}
		if strings.Contains(unit.Text, "deep/level/hidden.txt") || strings.Contains(unit.Text, "node_modules") {
			t.Fatalf("unit included skipped/out-of-depth path: %#v", unit)
		}
	}
}

func TestGatherIncludesVerifyFailure(t *testing.T) {
	env := envelope.NewEnvelope("task", "inspect KnownSymbol", fixtureDir(t))
	env.Verify = &envelope.VerifyResult{FailureDigest: "FAIL expected 2 got 1"}
	got, err := New(config.GatherConfig{MaxDepth: 1, MaxFileBytes: 512}).Run(context.Background(), env)
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if !hasKind(got.Raw, "verify_failure") || !rawContains(got.Raw, "expected 2 got 1") {
		t.Fatalf("raw context missing verify failure: %#v", got.Raw)
	}
}

func TestGatherCleanGitRepoOmitsGitUnits(t *testing.T) {
	dir := cleanGitRepo(t)
	got := gatherDir(t, dir, config.GatherConfig{MaxDepth: 2, MaxFileBytes: 4096}, "inspect alpha")
	if hasKind(got.Raw, "git_status") {
		t.Fatalf("clean repo included git units: %#v", got.Raw.Units)
	}
}

func TestGatherDirtyGitRepoRanksGitUnitsFirst(t *testing.T) {
	dir := cleanGitRepo(t)
	writeFile(t, dir, "file.txt", "alpha changed\n")
	got := gatherDir(t, dir, config.GatherConfig{MaxDepth: 2, MaxFileBytes: 4096}, "inspect alpha")
	if len(got.Raw.Units) == 0 || got.Raw.Units[0].Kind != "git_status" {
		t.Fatalf("first unit = %#v", got.Raw.Units)
	}
	if !hasKind(got.Raw, "git_diff_unstaged") || !rawContains(got.Raw, "+alpha changed") {
		t.Fatalf("raw context missing unstaged diff: %#v", got.Raw.Units)
	}
}

func TestGatherStagedGitRepoIncludesStagedDiff(t *testing.T) {
	dir := cleanGitRepo(t)
	writeFile(t, dir, "file.txt", "alpha staged\n")
	runGit(t, dir, "add", "file.txt")
	got := gatherDir(t, dir, config.GatherConfig{MaxDepth: 2, MaxFileBytes: 4096}, "inspect alpha")
	if !hasKind(got.Raw, "git_status") || !hasKind(got.Raw, "git_diff_staged") || !rawContains(got.Raw, "+alpha staged") {
		t.Fatalf("raw context missing staged diff: %#v", got.Raw.Units)
	}
}

func TestGatherGitUnitsRespectMaxBytes(t *testing.T) {
	dir := cleanGitRepo(t)
	writeFile(t, dir, "file.txt", "alpha "+strings.Repeat("long ", 80)+"\n")
	got := gatherDir(t, dir, config.GatherConfig{MaxDepth: 2, MaxFileBytes: 48}, "inspect alpha")
	foundGit := false
	foundTruncated := false
	for _, unit := range got.Raw.Units {
		if !strings.HasPrefix(unit.Kind, "git_") {
			continue
		}
		foundGit = true
		if len(unit.Text) > 48 {
			t.Fatalf("git unit exceeded max bytes: %#v", unit)
		}
		if strings.Contains(unit.Text, "[truncated]") {
			foundTruncated = true
		}
	}
	if !foundGit || !foundTruncated {
		t.Fatalf("git truncation not observed: %#v", got.Raw.Units)
	}
}

func TestGatherNonGitDirectoryStillSucceeds(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "alpha target\n")
	got := gatherDir(t, dir, config.GatherConfig{MaxDepth: 2, MaxFileBytes: 512}, "inspect target")
	if got.Raw == nil || hasKind(got.Raw, "git_status") || !hasKind(got.Raw, "dir_listing") {
		t.Fatalf("non-git gather = %#v", got.Raw)
	}
}

func TestGatherHasNoLLMClientField(t *testing.T) {
	clientType := reflect.TypeOf((*llm.Client)(nil)).Elem()
	gatherType := reflect.TypeOf(Gather{})
	for i := 0; i < gatherType.NumField(); i++ {
		field := gatherType.Field(i)
		if field.Type.Implements(clientType) || reflect.PointerTo(field.Type).Implements(clientType) {
			t.Fatalf("Gather has llm client field: %s", field.Name)
		}
	}
}

func runGather(t *testing.T, cfg config.GatherConfig, instruction string) *envelope.Envelope {
	t.Helper()
	return gatherDir(t, fixtureDir(t), cfg, instruction)
}

func gatherDir(t *testing.T, dir string, cfg config.GatherConfig, instruction string) *envelope.Envelope {
	t.Helper()
	env := envelope.NewEnvelope("task", instruction, dir)
	got, err := New(cfg).Run(context.Background(), env)
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if got.Raw == nil {
		t.Fatal("raw context nil")
	}
	return got
}

func cleanGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	writeFile(t, dir, "file.txt", "alpha base\n")
	runGit(t, dir, "add", "file.txt")
	runGit(t, dir, "commit", "--no-gpg-sign", "-m", "init")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeFile(t *testing.T, dir, name, text string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func fixtureDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("testdata/repo")
	if err != nil {
		t.Fatalf("fixture dir: %v", err)
	}
	return dir
}

func rawContains(raw *envelope.RawContext, needle string) bool {
	if raw == nil {
		return false
	}
	for _, unit := range raw.Units {
		if strings.Contains(unit.Text, needle) {
			return true
		}
	}
	return false
}

func hasKind(raw *envelope.RawContext, kind string) bool {
	if raw == nil {
		return false
	}
	for _, unit := range raw.Units {
		if unit.Kind == kind {
			return true
		}
	}
	return false
}
