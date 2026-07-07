package ui

import (
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/gongahkia/paw/internal/envelope"
)

type Progress struct {
	w          io.Writer
	quiet      bool
	tty        bool
	color      Colorizer
	logger     *slog.Logger
	structured bool
}

type ProgressOption func(*Progress)

func WithQuiet(quiet bool) ProgressOption {
	return func(p *Progress) {
		p.quiet = quiet
	}
}

func WithColorizer(color Colorizer) ProgressOption {
	return func(p *Progress) {
		p.color = color
	}
}

func WithLogger(logger *slog.Logger) ProgressOption {
	return func(p *Progress) {
		p.logger = logger
	}
}

func WithStructured(structured bool) ProgressOption {
	return func(p *Progress) {
		p.structured = structured
	}
}

func NewProgress(w io.Writer, opts ...ProgressOption) *Progress {
	p := &Progress{w: w, tty: isTerminal(w), color: NewColorizer(w)}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *Progress) StageDone(stage string, env *envelope.Envelope, duration time.Duration) {
	if p == nil || p.quiet || p.w == nil {
		return
	}
	if p.structured && p.logger != nil {
		p.logger.Info("stage complete", "stage", stage, "summary", stageSummary(stage, env), "duration_ms", duration.Milliseconds())
		return
	}
	_, _ = fmt.Fprintf(p.w, "%s %s (%s)\n", p.color.Green("["+stage+"]"), p.summary(stage, env), p.color.Dim(formatDuration(duration)))
}

func (p *Progress) IsTTY() bool {
	return p != nil && p.tty
}

func (p *Progress) summary(stage string, env *envelope.Envelope) string {
	summary := stageSummary(stage, env)
	if stage != "verify" || env == nil || env.Verify == nil {
		return summary
	}
	if env.Verify.Passed {
		return p.color.Green(summary)
	}
	return p.color.Yellow(summary)
}

func stageSummary(stage string, env *envelope.Envelope) string {
	switch stage {
	case "gather":
		if env != nil && env.Raw != nil {
			return fmt.Sprintf("collected %d units", len(env.Raw.Units))
		}
	case "compress":
		if env != nil && env.Digest != nil {
			return fmt.Sprintf("digest has %d items", len(env.Digest.Items))
		}
		return "raw context"
	case "plan":
		if env != nil && env.Plan != nil {
			if env.Plan.Done {
				return "done"
			}
			if env.Plan.NextAction != nil && env.Plan.NextAction.Kind != "" {
				return "next " + env.Plan.NextAction.Kind
			}
		}
	case "edit":
		if env != nil && env.Patch != nil {
			return fmt.Sprintf("patched %d files", len(env.Patch.Files))
		}
	case "verify":
		if env != nil && env.Verify != nil {
			if env.Verify.Passed {
				return "passed"
			}
			return "failed"
		}
	}
	return "done"
}

func formatDuration(d time.Duration) string {
	return fmt.Sprintf("%.1fs", d.Seconds())
}
