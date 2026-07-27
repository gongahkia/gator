package observability

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestCorrelationMergesAndCapturesSpan(t *testing.T) {
	span := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled})
	ctx := trace.ContextWithSpanContext(context.Background(), span)
	ctx = With(ctx, Correlation{RunID: "run", Stage: "builder", Attempt: 2})
	ctx = With(ctx, Correlation{ActionID: "action"})
	value := From(ctx)
	if value.RunID != "run" || value.Stage != "builder" || value.Attempt != 2 || value.ActionID != "action" {
		t.Fatalf("correlation=%+v", value)
	}
	if value.TraceID != span.TraceID().String() || value.SpanID != span.SpanID().String() {
		t.Fatalf("span correlation missing: %+v", value)
	}
}
