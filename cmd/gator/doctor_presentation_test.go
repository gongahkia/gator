package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestDoctorTerminalPresentation(t *testing.T) {
	report := "Repository: detected\nProvider: gemini\nAuthentication (Gator credential or GEMINI_API_KEY): stored\nStrict sandbox: macOS Seatbelt (sandbox-exec)\nWeb search: not configured (set BRAVE_SEARCH_API_KEY)\nDependencies:\n  Git: installed (required for repository discovery)\n  Ollama: missing (optional for reviewed local models)\nReviewed local-model eligibility (Gator guardrail):\n  qwen3-4b: enabled · needs 4.7 GiB RAM\n  qwen3-8b: disabled: requires at least 9.7 GiB RAM\nSuggested verification: go test ./...\n"
	styled := renderDoctorTerminal(report, true)
	for _, want := range []string{"🐊 Gator doctor", "🔧 Dependencies", "🧠 Reviewed local models", "🧪 Suggested verification", doctorGreen + "detected", doctorAmber + "not configured"} {
		if !strings.Contains(styled, want) {
			t.Errorf("styled doctor output missing %q", want)
		}
	}
	plain := renderDoctorTerminal(report, false)
	if strings.Contains(plain, "\x1b[") {
		t.Fatal("NO_COLOR doctor output contains ANSI escape sequences")
	}
	if !strings.Contains(plain, "🐊 Gator doctor") {
		t.Fatal("NO_COLOR doctor output lost its heading")
	}
	for _, want := range []string{"✓ Git:", "○ Ollama:", "✓ qwen3-4b:", "○ qwen3-8b:"} {
		if !strings.Contains(plain, want) {
			t.Errorf("plain doctor output missing %q", want)
		}
	}
}

func TestAnimateDoctorClearsSpinnerBeforeReport(t *testing.T) {
	var output bytes.Buffer
	err := animateDoctor(&output, true, func(out io.Writer) error {
		_, err := io.WriteString(out, "Repository: detected\n")
		return err
	})
	if err != nil {
		t.Fatalf("animate doctor: %v", err)
	}
	got := output.String()
	if !strings.Contains(got, "⠋ Checking Gator") || !strings.Contains(got, doctorClear+doctorBold) {
		t.Fatalf("doctor animation or cleanup missing: %q", got)
	}
	if !strings.Contains(got, "🐊 Gator doctor\x1b[0m\n") || !strings.Contains(got, "Repository:") {
		t.Fatalf("doctor report missing after spinner: %q", got)
	}
}

func TestPresentDoctorUsesPlainOutputForPipes(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	var output bytes.Buffer
	if err := presentDoctor(nil, &output); err != nil {
		t.Fatalf("present doctor: %v", err)
	}
	got := output.String()
	if !strings.HasPrefix(got, "Repository: ") || strings.Contains(got, "\x1b[") || strings.Contains(got, "🐊") {
		t.Fatalf("piped doctor output changed: %q", got)
	}
}
