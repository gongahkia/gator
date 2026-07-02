package compress

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm"
	"github.com/gongahkia/paw/internal/schema"
)

type Compress struct {
	Client          llm.Client
	DisableCompress bool
}

func New(client llm.Client) *Compress {
	return &Compress{Client: client}
}

func (c *Compress) Name() string {
	return "compress"
}

func (c *Compress) Run(ctx context.Context, in *envelope.Envelope) (*envelope.Envelope, error) {
	out := *in
	out.Stage = c.Name()
	if in.Raw == nil || len(in.Raw.Units) == 0 {
		out.Digest = &envelope.ContextDigest{}
		return &out, nil
	}
	if c.DisableCompress || c.Client == nil {
		out.Digest = &envelope.ContextDigest{}
		return &out, nil
	}
	rawSchema := schema.Raw("context_digest")
	resp, err := c.Client.Chat(ctx, llm.ChatRequest{
		Messages: []llm.ChatMessage{
			{Role: "system", Content: "Select only real, relevant spans from the provided raw context."},
			{Role: "user", Content: rawPrompt(in)},
		},
		Temperature: 0,
		JSONSchema:  rawSchema,
	})
	if err != nil {
		return nil, err
	}
	var digest envelope.ContextDigest
	if err := json.Unmarshal([]byte(resp.Content), &digest); err != nil {
		return nil, err
	}
	out.Digest = &digest
	return &out, nil
}

func rawPrompt(env *envelope.Envelope) string {
	b, _ := json.Marshal(env.Raw)
	return fmt.Sprintf("instruction:\n%s\n\nraw_context:\n%s", env.Instruction, b)
}
