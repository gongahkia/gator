package ui

import (
	"io"
	"testing"
)

func TestColorizer(t *testing.T) {
	tests := []struct {
		name    string
		tty     bool
		env     string
		noColor bool
		want    string
		enabled bool
	}{
		{name: "tty", tty: true, want: "\x1b[31merror\x1b[0m", enabled: true},
		{name: "not tty", tty: false, want: "error"},
		{name: "NO_COLOR", tty: true, env: "1", want: "error"},
		{name: "no color flag", tty: true, noColor: true, want: "error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			color := NewColorizer(io.Discard,
				WithColorTTY(tt.tty),
				WithNoColor(tt.noColor),
				WithColorEnv(func(key string) string {
					if key == "NO_COLOR" {
						return tt.env
					}
					return ""
				}),
			)
			if got := color.Red("error"); got != tt.want {
				t.Fatalf("red = %q; want %q", got, tt.want)
			}
			if color.Enabled() != tt.enabled {
				t.Fatalf("enabled = %t; want %t", color.Enabled(), tt.enabled)
			}
		})
	}
}

func TestColorizerHelpers(t *testing.T) {
	color := NewColorizer(io.Discard, WithColorTTY(true), WithColorEnv(func(string) string { return "" }))
	tests := map[string]string{
		color.Yellow("warn"): "\x1b[33mwarn\x1b[0m",
		color.Green("ok"):    "\x1b[32mok\x1b[0m",
		color.Dim("meta"):    "\x1b[2mmeta\x1b[0m",
	}
	for got, want := range tests {
		if got != want {
			t.Fatalf("color helper = %q; want %q", got, want)
		}
	}
}
