package stage

import (
	"encoding/json"
	"io"
	"sync"
)

type TraceEvent struct {
	Stage        string `json:"stage"`
	Turn         int    `json:"turn"`
	InputBytes   int    `json:"input_bytes"`
	OutputBytes  int    `json:"output_bytes"`
	Tokens       int    `json:"tokens"`
	DroppedItems int    `json:"dropped_items"`
	UsedFallback bool   `json:"used_fallback"`
	DurationMS   int64  `json:"duration_ms"`
}

type Tracer struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func NewTracer(w io.Writer) *Tracer {
	return &Tracer{enc: json.NewEncoder(w)}
}

func (t *Tracer) Write(event TraceEvent) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.enc.Encode(event)
}
