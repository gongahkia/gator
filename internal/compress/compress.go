package compress

import (
	"context"
	"encoding/json"

	"github.com/gongahkia/paw/internal/budget"
	"github.com/gongahkia/paw/internal/egress"
	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm"
	"github.com/gongahkia/paw/internal/schema"
)

const maxCompressOutputTokens = 1024

type Compress struct {
	Client          llm.Client
	DisableCompress bool
	RawContext      bool
	UsedFallback    bool
	DroppedItems    int
	ValidationDrops map[string]int
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

func (c *Compress) ValidationDropCounts() map[string]int {
	return copyDropCounts(c.ValidationDrops)
}

func (c *Compress) Run(ctx context.Context, in *envelope.Envelope) (*envelope.Envelope, error) {
	out := *in
	redacted, manifest := egress.RedactForEgress(in.Raw)
	if in.Egress != nil && len(in.Egress.Findings) > 0 {
		manifest.Findings = append([]egress.Finding(nil), in.Egress.Findings...)
	}
	out.Raw = redacted.Raw
	out.Egress = &manifest
	c.UsedFallback = false
	c.DroppedItems = 0
	c.ValidationDrops = nil
	out.Stage = c.Name()
	if c.RawContext {
		out.Digest = nil
		return &out, nil
	}
	if out.Raw == nil || len(out.Raw.Units) == 0 {
		out.Digest = ptr(fallbackDigest(out.Instruction, out.Raw, defaultFallbackTokens))
		return &out, nil
	}
	if c.DisableCompress || c.Client == nil {
		out.Digest = ptr(fallbackDigest(out.Instruction, out.Raw, defaultFallbackTokens))
		c.UsedFallback = true
		return &out, nil
	}
	rawSchema := schema.Raw("context_digest")
	resp, err := c.Client.Chat(ctx, llm.ChatRequest{
		Messages:    droneMessages(&out, rawSchema),
		Temperature: 0,
		MaxTokens:   maxCompressOutputTokens,
		JSONSchema:  rawSchema,
	})
	if err != nil {
		return nil, err
	}
	budget.AddDrone(&out.Budget, resp.Usage.InputTokens+resp.Usage.OutputTokens, resp.Usage.TokenSource)
	var digest envelope.ContextDigest
	if err := json.Unmarshal([]byte(resp.Content), &digest); err != nil {
		c.recordDrop(dropSchemaError)
		out.Digest = ptr(fallbackDigest(out.Instruction, out.Raw, defaultFallbackTokens))
		c.UsedFallback = true
		return &out, nil
	}
	validated, stats, err := validateDigest(out.Raw, digest)
	c.DroppedItems = stats.Dropped
	c.ValidationDrops = copyDropCounts(stats.Reasons)
	if err != nil || tooManyDropped(stats) {
		if err == nil {
			c.recordReason(dropTooManyDropped)
		}
		out.Digest = ptr(fallbackDigest(out.Instruction, out.Raw, defaultFallbackTokens))
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

func (c *Compress) recordDrop(reason string) {
	c.recordReason(reason)
	c.DroppedItems++
}

func (c *Compress) recordReason(reason string) {
	if c.ValidationDrops == nil {
		c.ValidationDrops = map[string]int{}
	}
	c.ValidationDrops[reason]++
}

func copyDropCounts(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for reason, count := range in {
		if count > 0 {
			out[reason] = count
		}
	}
	return out
}
