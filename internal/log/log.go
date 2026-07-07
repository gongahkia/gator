package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

const (
	FormatText = "text"
	FormatJSON = "json"
)

type Config struct {
	Format string
	Level  string
	Writer io.Writer
}

type contextKey struct{}

func init() {
	_, _ = Install(Config{})
}

func Install(cfg Config) (*slog.Logger, error) {
	logger, err := New(cfg)
	if err != nil {
		return nil, err
	}
	slog.SetDefault(logger)
	return logger, nil
}

func New(cfg Config) (*slog.Logger, error) {
	w := cfg.Writer
	if w == nil {
		w = os.Stderr
	}
	level, err := ParseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}
	opts := &slog.HandlerOptions{Level: level}
	switch normalize(cfg.Format) {
	case "", FormatText:
		return slog.New(slog.NewTextHandler(w, opts)), nil
	case FormatJSON:
		return slog.New(slog.NewJSONHandler(w, opts)), nil
	default:
		return nil, fmt.Errorf("unsupported log format %q", cfg.Format)
	}
}

func ParseLevel(raw string) (slog.Level, error) {
	switch normalize(raw) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unsupported log level %q", raw)
	}
}

func IsJSON(format string) bool {
	return normalize(format) == FormatJSON
}

func With(ctx context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, logger)
}

func From(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if logger, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && logger != nil {
			return logger
		}
	}
	return slog.Default()
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
