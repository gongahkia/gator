package stage

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/gongahkia/paw/internal/envelope"
)

type TraceEvent struct {
	Stage        string             `json:"stage"`
	Turn         int                `json:"turn"`
	InputBytes   int                `json:"input_bytes"`
	OutputBytes  int                `json:"output_bytes"`
	Tokens       int                `json:"tokens"`
	TokenSource  string             `json:"token_source,omitempty"`
	DroppedItems int                `json:"dropped_items"`
	UsedFallback bool               `json:"used_fallback"`
	DurationMS   int64              `json:"duration_ms"`
	Envelope     *envelope.Envelope `json:"envelope,omitempty"`
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
			"token_source", event.TokenSource,
			"dropped_items", event.DroppedItems,
			"used_fallback", event.UsedFallback,
			"duration_ms", event.DurationMS,
		)
	} else if t.mirror != nil {
		_, _ = fmt.Fprintf(t.mirror, "stage=%s turn=%d input_bytes=%d output_bytes=%d tokens=%d token_source=%s dropped_items=%d used_fallback=%t duration_ms=%d\n",
			event.Stage,
			event.Turn,
			event.InputBytes,
			event.OutputBytes,
			event.Tokens,
			event.TokenSource,
			event.DroppedItems,
			event.UsedFallback,
			event.DurationMS,
		)
	}
	return nil
}

func ReadTrace(path string) (*envelope.Envelope, error) {
	events, err := ReadTraceEvents(path)
	if err != nil {
		return nil, err
	}
	var last *envelope.Envelope
	for i := range events {
		if events[i].Envelope != nil {
			last = events[i].Envelope
		}
	}
	if last == nil {
		return nil, fmt.Errorf("trace %s contains no envelope snapshots", path)
	}
	return last, nil
}

func ReadTraceEvents(path string) ([]TraceEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	dec := json.NewDecoder(file)
	var events []TraceEvent
	for {
		var event TraceEvent
		if err := dec.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("read trace %s event %d: %w", path, len(events)+1, err)
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("trace %s is empty", path)
	}
	return events, nil
}
