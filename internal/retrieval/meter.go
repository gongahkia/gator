package retrieval

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/usage"
)

type metered struct {
	embedder Embedder
	state    string
}

func Meter(embedder Embedder, stateDir string) Embedder { return metered{embedder, stateDir} }
func (m metered) ID() string                            { return m.embedder.ID() }
func (m metered) Embed(ctx context.Context, input []string) ([][]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Gemini's adapter makes one HTTP request per input; the others batch.
	if strings.HasPrefix(m.ID(), "gemini/") && len(input) > 1 {
		var out [][]float32
		for _, text := range input {
			vectors, err := m.Embed(ctx, []string{text})
			if err != nil {
				return nil, err
			}
			out = append(out, vectors...)
		}
		return out, nil
	}
	if len(input) == 0 {
		return m.embedder.Embed(ctx, input)
	}
	s, err := usage.Open(m.state)
	if err != nil {
		return nil, err
	}
	var tokens int64
	for _, text := range input {
		tokens += int64(len(text))
	}
	r, err := s.Reserve(time.Now(), "retrieval", m.ID(), tokens)
	if err != nil {
		return nil, err
	}
	vectors, err := m.embedder.Embed(ctx, input)
	// Embedding adapters do not all report tokens. Keep the input-byte
	// reservation explicitly estimated rather than inventing provider usage.
	return vectors, errors.Join(err, r.Finish(0, false, err != nil, nil))
}
