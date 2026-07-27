package observability

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Correlation identifies one durable operator-visible execution chain.
type Correlation struct {
	RunID       string
	Stage       string
	Attempt     int
	ProviderID  string
	RevisionID  int64
	ApprovalID  int64
	TurnID      string
	ActionID    string
	SandboxID   string
	Actor       string
	TraceID     string
	SpanID      string
	Traceparent string
}

type correlationKey struct{}

func With(ctx context.Context, next Correlation) context.Context {
	value := From(ctx)
	if next.RunID != "" {
		value.RunID = next.RunID
	}
	if next.Stage != "" {
		value.Stage = next.Stage
	}
	if next.Attempt != 0 {
		value.Attempt = next.Attempt
	}
	if next.ProviderID != "" {
		value.ProviderID = next.ProviderID
	}
	if next.RevisionID != 0 {
		value.RevisionID = next.RevisionID
	}
	if next.ApprovalID != 0 {
		value.ApprovalID = next.ApprovalID
	}
	if next.TurnID != "" {
		value.TurnID = next.TurnID
	}
	if next.ActionID != "" {
		value.ActionID = next.ActionID
	}
	if next.SandboxID != "" {
		value.SandboxID = next.SandboxID
	}
	if next.Actor != "" {
		value.Actor = next.Actor
	}
	if next.Traceparent != "" {
		value.Traceparent = next.Traceparent
	}
	return context.WithValue(ctx, correlationKey{}, value)
}

func From(ctx context.Context) Correlation {
	value, _ := ctx.Value(correlationKey{}).(Correlation)
	span := trace.SpanContextFromContext(ctx)
	if span.IsValid() {
		value.TraceID, value.SpanID = span.TraceID().String(), span.SpanID().String()
	}
	if value.Traceparent == "" {
		value.Traceparent = Traceparent(ctx)
	}
	return value
}

func Traceparent(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier.Get("traceparent")
}

func ExtractTraceparent(ctx context.Context, value string) context.Context {
	if strings.TrimSpace(value) == "" {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier{"traceparent": value})
}

func Logger(logger *slog.Logger, ctx context.Context) *slog.Logger {
	value := From(ctx)
	args := []any{}
	for _, item := range []struct{ key, value string }{
		{"run_id", value.RunID}, {"stage", value.Stage}, {"provider_id", value.ProviderID}, {"turn_id", value.TurnID}, {"action_id", value.ActionID}, {"sandbox_id", value.SandboxID}, {"actor", value.Actor}, {"trace_id", value.TraceID}, {"span_id", value.SpanID},
	} {
		if item.value != "" {
			args = append(args, item.key, item.value)
		}
	}
	if value.Attempt != 0 {
		args = append(args, "attempt", value.Attempt)
	}
	if value.RevisionID != 0 {
		args = append(args, "revision_id", value.RevisionID)
	}
	if value.ApprovalID != 0 {
		args = append(args, "approval_id", value.ApprovalID)
	}
	return logger.With(args...)
}

func EntityRefs(value Correlation) map[string]string {
	refs := map[string]string{}
	for key, item := range map[string]string{"stage": value.Stage, "provider_id": value.ProviderID, "turn_id": value.TurnID, "action_id": value.ActionID, "sandbox_id": value.SandboxID, "actor": value.Actor} {
		if item != "" {
			refs[key] = item
		}
	}
	if value.Attempt != 0 {
		refs["attempt"] = fmt.Sprint(value.Attempt)
	}
	if value.RevisionID != 0 {
		refs["revision_id"] = fmt.Sprint(value.RevisionID)
	}
	if value.ApprovalID != 0 {
		refs["approval_id"] = fmt.Sprint(value.ApprovalID)
	}
	return refs
}
