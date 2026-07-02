package gather

import (
	"context"
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
	env := envelope.NewEnvelope("task", instruction, fixtureDir(t))
	got, err := New(cfg).Run(context.Background(), env)
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if got.Raw == nil {
		t.Fatal("raw context nil")
	}
	return got
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
