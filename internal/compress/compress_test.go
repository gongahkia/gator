package compress

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/egress"
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
	if got.Budget.DroneTokenSource != llm.TokenSourceProvider {
		t.Fatalf("drone token source = %q", got.Budget.DroneTokenSource)
	}
	assertDigestQuotesFromRaw(t, got.Raw, got.Digest)
}

func TestCompressCapsOutputTokens(t *testing.T) {
	client := &captureCompressClient{content: digestJSON(t, validDigest())}
	stage := New(client)
	if _, err := stage.Run(context.Background(), rawEnvelope()); err != nil {
		t.Fatalf("compress: %v", err)
	}
	if client.request.MaxTokens != maxCompressOutputTokens {
		t.Fatalf("max tokens = %d", client.request.MaxTokens)
	}
}

func TestCompressRedactsSecretsBeforeModelEgress(t *testing.T) {
	secret := "sk-abcdefghijklmnopqrstuvwxyz123456"
	client := &captureCompressClient{content: digestJSON(t, validDigest())}
	stage := New(client)
	env := rawEnvelope()
	env.Raw.Units = append(env.Raw.Units, envelope.RawUnit{ID: "u003", Kind: "file_slice", Path: "config.env", Text: "token=" + secret})
	got, err := stage.Run(context.Background(), env)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	messages, err := json.Marshal(client.request.Messages)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(messages), secret) || strings.Contains(got.Raw.Units[2].Text, secret) {
		t.Fatalf("model egress leaked secret: messages=%s raw=%q", messages, got.Raw.Units[2].Text)
	}
	if got.Egress == nil || len(got.Egress.Findings) != 1 || got.Egress.Findings[0].Kind != "openai_key" {
		t.Fatalf("egress manifest = %#v", got.Egress)
	}
}

func TestCompressBlocksSecretWithoutProviderApproval(t *testing.T) {
	client := &captureCompressClient{content: digestJSON(t, validDigest())}
	stage := New(client)
	stage.SetEgressPolicy(config.Defaults().Policy.Egress, "openai", "https://api.example.test/v1")
	env := rawEnvelope()
	env.Raw.Units = append(env.Raw.Units, envelope.RawUnit{ID: "u003", Text: "token=sk-abcdefghijklmnopqrstuvwxyz123456"})
	if _, err := stage.Run(context.Background(), env); err == nil || !strings.Contains(err.Error(), "egress blocked") {
		t.Fatalf("secret egress error = %v", err)
	}
	if len(client.request.Messages) != 0 {
		t.Fatalf("model request = %#v", client.request)
	}
}

func TestCompressAllowsSecretWithMatchingProviderApproval(t *testing.T) {
	client := &captureCompressClient{content: digestJSON(t, validDigest())}
	stage := New(client)
	stage.SetEgressPolicy(config.Defaults().Policy.Egress, "openai", "https://api.example.test/v1")
	env := rawEnvelope()
	env.Raw.Units = append(env.Raw.Units, envelope.RawUnit{ID: "u003", Text: "token=sk-abcdefghijklmnopqrstuvwxyz123456"})
	_, manifest := egress.RedactForEgress(env.Raw)
	approved, err := egress.Approve(manifest, "openai", "https://api.example.test/v1", time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC), false)
	if err != nil {
		t.Fatal(err)
	}
	env.Egress = &approved
	if _, err := stage.Run(context.Background(), env); err != nil {
		t.Fatalf("approved egress error = %v", err)
	}
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
	assertDropCount(t, stage, dropPathMismatch, 1)
	assertDigestQuotesFromRaw(t, got.Raw, got.Digest)
}

func TestCompressDropsUnknownUnit(t *testing.T) {
	stage, got := runCompress(t, digestWithInvalidItem(envelope.DigestItem{
		UnitID:    "missing",
		Path:      "calc.go",
		Relevance: 100,
		Spans:     []envelope.DigestSpan{{StartLine: 2, EndLine: 2, Quote: "func KnownSymbol() string"}},
	}))
	if stage.UsedFallback || stage.DroppedItems != 1 {
		t.Fatalf("fallback=%v dropped=%d", stage.UsedFallback, stage.DroppedItems)
	}
	if len(got.Digest.Items) != 1 {
		t.Fatalf("digest = %#v", got.Digest)
	}
	assertDropCount(t, stage, dropUnknownUnit, 1)
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
	assertDropCount(t, stage, dropQuoteMissing, 1)
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
	assertDropCount(t, stage, dropLineOutOfRange, 1)
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
	assertDropCount(t, stage, dropPathMismatch, 1)
	assertDropCount(t, stage, dropUnknownUnit, 1)
	assertDropCount(t, stage, dropTooManyDropped, 1)
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
	assertDropCount(t, stage, dropSchemaError, 1)
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

func assertDropCount(t *testing.T, stage *Compress, reason string, want int) {
	t.Helper()
	if got := stage.ValidationDrops[reason]; got != want {
		t.Fatalf("drop %s = %d, want %d; all=%v", reason, got, want, stage.ValidationDrops)
	}
}

type captureCompressClient struct {
	request llm.ChatRequest
	content string
}

func (c *captureCompressClient) Chat(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	c.request = req
	return &llm.ChatResponse{
		Content: c.content,
		Usage:   llm.Usage{InputTokens: 11, OutputTokens: 7},
	}, nil
}
