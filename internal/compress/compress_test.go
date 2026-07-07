package compress

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm"
	"github.com/gongahkia/paw/internal/llm/faketest"
)

func TestCompressValidDigestPasses(t *testing.T) {
	stage, got := runCompress(t, validDigest())
	if stage.UsedFallback {
		t.Fatal("unexpected fallback")
	}
	if len(got.Digest.Items) != 1 || got.Digest.Items[0].UnitID != "u001" {
		t.Fatalf("digest = %#v", got.Digest)
	}
	if got.Budget.DroneTokens != 18 {
		t.Fatalf("drone tokens = %d", got.Budget.DroneTokens)
	}
	assertDigestQuotesFromRaw(t, got.Raw, got.Digest)
}

func TestCompressDropsHallucinatedPath(t *testing.T) {
	stage, got := runCompress(t, digestWithInvalidItem(envelope.DigestItem{
		UnitID:    "u001",
		Path:      "ghost.go",
		Relevance: 100,
		Spans:     []envelope.DigestSpan{{StartLine: 2, EndLine: 2, Quote: "func KnownSymbol() string"}},
	}))
	if stage.UsedFallback || stage.DroppedItems != 1 {
		t.Fatalf("fallback=%v dropped=%d", stage.UsedFallback, stage.DroppedItems)
	}
	if len(got.Digest.Items) != 1 {
		t.Fatalf("digest = %#v", got.Digest)
	}
	assertDigestQuotesFromRaw(t, got.Raw, got.Digest)
}

func TestCompressDropsNonVerbatimQuote(t *testing.T) {
	stage, got := runCompress(t, digestWithInvalidItem(envelope.DigestItem{
		UnitID:    "u001",
		Path:      "calc.go",
		Relevance: 100,
		Spans:     []envelope.DigestSpan{{StartLine: 2, EndLine: 2, Quote: "func Invented()"}},
	}))
	if stage.UsedFallback || stage.DroppedItems != 1 {
		t.Fatalf("fallback=%v dropped=%d", stage.UsedFallback, stage.DroppedItems)
	}
	assertDigestQuotesFromRaw(t, got.Raw, got.Digest)
}

func TestCompressDropsBadLineRange(t *testing.T) {
	stage, got := runCompress(t, digestWithInvalidItem(envelope.DigestItem{
		UnitID:    "u001",
		Path:      "calc.go",
		Relevance: 100,
		Spans:     []envelope.DigestSpan{{StartLine: 99, EndLine: 100, Quote: "func KnownSymbol() string"}},
	}))
	if stage.UsedFallback || stage.DroppedItems != 1 {
		t.Fatalf("fallback=%v dropped=%d", stage.UsedFallback, stage.DroppedItems)
	}
	assertDigestQuotesFromRaw(t, got.Raw, got.Digest)
}

func TestCompressFallsBackWhenTooManyDropped(t *testing.T) {
	stage, got := runCompress(t, envelope.ContextDigest{
		Summary: "mostly bad",
		Items: []envelope.DigestItem{
			validDigest().Items[0],
			{UnitID: "u001", Path: "ghost.go", Relevance: 100, Spans: []envelope.DigestSpan{{StartLine: 2, EndLine: 2, Quote: "func KnownSymbol() string"}}},
			{UnitID: "missing", Path: "calc.go", Relevance: 100, Spans: []envelope.DigestSpan{{StartLine: 2, EndLine: 2, Quote: "func KnownSymbol() string"}}},
		},
	})
	if !stage.UsedFallback {
		t.Fatal("expected fallback")
	}
	assertDigestQuotesFromRaw(t, got.Raw, got.Digest)
}

func TestCompressFallsBackOnUnparseable(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondOllama("", "not-json")
	stage := New(llm.NewOllamaClient(srv.URL, "drone"))
	got, err := stage.Run(context.Background(), rawEnvelope())
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	if !stage.UsedFallback {
		t.Fatal("expected fallback")
	}
	assertDigestQuotesFromRaw(t, got.Raw, got.Digest)
}

func runCompress(t *testing.T, digest envelope.ContextDigest) (*Compress, *envelope.Envelope) {
	t.Helper()
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondOllama("", digestJSON(t, digest))
	stage := New(llm.NewOllamaClient(srv.URL, "drone"))
	got, err := stage.Run(context.Background(), rawEnvelope())
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	return stage, got
}

func rawEnvelope() *envelope.Envelope {
	env := envelope.NewEnvelope("task", "fix KnownSymbol Add", "/repo")
	env.Raw = &envelope.RawContext{
		Units: []envelope.RawUnit{
			{
				ID:        "u001",
				Kind:      "file_slice",
				Path:      "calc.go",
				StartLine: 1,
				EndLine:   5,
				Text:      "package fixture\nfunc KnownSymbol() string {\nreturn \"known\"\n}\n",
			},
			{
				ID:        "u002",
				Kind:      "file_slice",
				Path:      "calc.go",
				StartLine: 7,
				EndLine:   9,
				Text:      "func Add(a int, b int) int {\nreturn a - b\n}\n",
			},
		},
		TotalBytes: 107,
	}
	return env
}

func validDigest() envelope.ContextDigest {
	return envelope.ContextDigest{
		Summary: "known symbol is relevant",
		Items: []envelope.DigestItem{
			{
				UnitID:    "u001",
				Path:      "calc.go",
				Relevance: 90,
				Spans: []envelope.DigestSpan{
					{StartLine: 2, EndLine: 2, Quote: "func KnownSymbol() string"},
				},
			},
		},
	}
}

func digestWithInvalidItem(item envelope.DigestItem) envelope.ContextDigest {
	digest := validDigest()
	digest.Items = append(digest.Items, item)
	return digest
}

func digestJSON(t *testing.T, digest envelope.ContextDigest) string {
	t.Helper()
	b, err := json.Marshal(digest)
	if err != nil {
		t.Fatalf("marshal digest: %v", err)
	}
	return string(b)
}

func assertDigestQuotesFromRaw(t *testing.T, raw *envelope.RawContext, digest *envelope.ContextDigest) {
	t.Helper()
	units := map[string]string{}
	for _, unit := range raw.Units {
		units[unit.ID] = unit.Text
	}
	for _, item := range digest.Items {
		for _, span := range item.Spans {
			if !strings.Contains(units[item.UnitID], span.Quote) {
				t.Fatalf("quote %q absent from raw unit %q", span.Quote, item.UnitID)
			}
		}
	}
}
