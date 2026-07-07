package stage

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	mu     sync.Mutex
	enc    *json.Encoder
	mirror io.Writer
	logger *slog.Logger
}

type TracerOption func(*Tracer)

func WithMirror(w io.Writer) TracerOption {
	return func(t *Tracer) {
		t.mirror = w
	}
}

func WithLogger(logger *slog.Logger) TracerOption {
	return func(t *Tracer) {
		t.logger = logger
	}
}

func NewTracer(w io.Writer, opts ...TracerOption) *Tracer {
	tracer := &Tracer{enc: json.NewEncoder(w)}
	for _, opt := range opts {
		opt(tracer)
	}
	return tracer
}

func (t *Tracer) Write(event TraceEvent) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.enc.Encode(event); err != nil {
		return err
	}
	if t.logger != nil {
		t.logger.Debug("stage trace",
			"stage", event.Stage,
			"turn", event.Turn,
			"input_bytes", event.InputBytes,
			"output_bytes", event.OutputBytes,
			"tokens", event.Tokens,
			"dropped_items", event.DroppedItems,
			"used_fallback", event.UsedFallback,
			"duration_ms", event.DurationMS,
		)
	} else if t.mirror != nil {
		_, _ = fmt.Fprintf(t.mirror, "stage=%s turn=%d input_bytes=%d output_bytes=%d tokens=%d dropped_items=%d used_fallback=%t duration_ms=%d\n",
			event.Stage,
			event.Turn,
			event.InputBytes,
			event.OutputBytes,
			event.Tokens,
			event.DroppedItems,
			event.UsedFallback,
			event.DurationMS,
		)
	}
	return nil
}
