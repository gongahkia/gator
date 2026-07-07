package compress

import (
	"context"
	"encoding/json"

	"github.com/gongahkia/paw/internal/budget"
	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm"
	"github.com/gongahkia/paw/internal/schema"
)

type Compress struct {
	Client          llm.Client
	DisableCompress bool
	RawContext      bool
	UsedFallback    bool
	DroppedItems    int
}

func New(client llm.Client) *Compress {
	return &Compress{Client: client}
}

func (c *Compress) Name() string {
	return "compress"
}

func (c *Compress) TraceMetadata() (int, bool) {
	return c.DroppedItems, c.UsedFallback
}

func (c *Compress) Run(ctx context.Context, in *envelope.Envelope) (*envelope.Envelope, error) {
	out := *in
	c.UsedFallback = false
	c.DroppedItems = 0
	out.Stage = c.Name()
	if c.RawContext {
		out.Digest = nil
		return &out, nil
	}
	if in.Raw == nil || len(in.Raw.Units) == 0 {
		out.Digest = ptr(fallbackDigest(in.Instruction, in.Raw, defaultFallbackTokens))
		return &out, nil
	}
	if c.DisableCompress || c.Client == nil {
		out.Digest = ptr(fallbackDigest(in.Instruction, in.Raw, defaultFallbackTokens))
		c.UsedFallback = true
		return &out, nil
	}
	rawSchema := schema.Raw("context_digest")
	resp, err := c.Client.Chat(ctx, llm.ChatRequest{
		Messages:    droneMessages(in, rawSchema),
		Temperature: 0,
		JSONSchema:  rawSchema,
	})
	if err != nil {
		return nil, err
	}
	budget.AddDrone(&out.Budget, resp.Usage.InputTokens+resp.Usage.OutputTokens)
	var digest envelope.ContextDigest
	if err := json.Unmarshal([]byte(resp.Content), &digest); err != nil {
		out.Digest = ptr(fallbackDigest(in.Instruction, in.Raw, defaultFallbackTokens))
		c.UsedFallback = true
		return &out, nil
	}
	validated, stats, err := validateDigest(in.Raw, digest)
	c.DroppedItems = stats.Dropped
	if err != nil || tooManyDropped(stats) {
		out.Digest = ptr(fallbackDigest(in.Instruction, in.Raw, defaultFallbackTokens))
		c.UsedFallback = true
		return &out, nil
	}
	out.Digest = &validated
	return &out, nil
}

func tooManyDropped(stats validationStats) bool {
	return stats.Total > 0 && float64(stats.Dropped)/float64(stats.Total) > 0.5
}

func ptr(d envelope.ContextDigest) *envelope.ContextDigest {
	return &d
}
