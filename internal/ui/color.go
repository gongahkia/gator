package ui

import (
	"io"
	"os"

	"golang.org/x/term"
)

type Colorizer struct {
	enabled bool
}

type ColorOption func(*colorConfig)

type colorConfig struct {
	tty     *bool
	noColor bool
	getenv  func(string) string
}

func WithNoColor(noColor bool) ColorOption {
	return func(cfg *colorConfig) {
		cfg.noColor = noColor
	}
}

func WithColorTTY(tty bool) ColorOption {
	return func(cfg *colorConfig) {
		cfg.tty = &tty
	}
}

func WithColorEnv(getenv func(string) string) ColorOption {
	return func(cfg *colorConfig) {
		cfg.getenv = getenv
	}
}

func NewColorizer(w io.Writer, opts ...ColorOption) Colorizer {
	cfg := colorConfig{getenv: os.Getenv}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.getenv == nil {
		cfg.getenv = os.Getenv
	}
	tty := isTerminal(w)
	if cfg.tty != nil {
		tty = *cfg.tty
	}
	return Colorizer{enabled: tty && !cfg.noColor && cfg.getenv("NO_COLOR") == ""}
}

func (c Colorizer) Enabled() bool {
	return c.enabled
}

func (c Colorizer) Red(s string) string {
	return c.wrap("31", s)
}

func (c Colorizer) Yellow(s string) string {
	return c.wrap("33", s)
}

func (c Colorizer) Green(s string) string {
	return c.wrap("32", s)
}

func (c Colorizer) Dim(s string) string {
	return c.wrap("2", s)
}

func (c Colorizer) wrap(code, s string) string {
	if !c.enabled {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}
