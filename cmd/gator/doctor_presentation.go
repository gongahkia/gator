package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/gongahkia/gator/internal/rattles"
)

const (
	doctorReset = "\x1b[0m"
	doctorGreen = "\x1b[38;5;78m"
	doctorCyan  = "\x1b[38;5;44m"
	doctorAmber = "\x1b[38;5;214m"
	doctorRed   = "\x1b[38;5;203m"
	doctorDim   = "\x1b[2m"
	doctorBold  = "\x1b[1m"
	doctorClear = "\r\x1b[2K"
)

// presentDoctor adds transient feedback only when a person is watching a
// terminal. Piped output retains the stable, unstyled doctor report.
func presentDoctor(arguments []string, out io.Writer) error {
	file, ok := out.(*os.File)
	if !ok || !term.IsTerminal(file.Fd()) || strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return doctor(arguments, out)
	}
	return animateDoctor(out, os.Getenv("NO_COLOR") == "", func(report io.Writer) error {
		return doctor(arguments, report)
	})
}

func animateDoctor(out io.Writer, color bool, check func(io.Writer) error) error {
	var report bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- check(&report) }()
	spinner := rattles.BrailleDots
	ticker := time.NewTicker(spinner.Interval)
	defer ticker.Stop()
	frame := 0
	if _, err := io.WriteString(out, doctorProgress(spinner.Frame(frame), color)); err != nil {
		return err
	}
	for {
		select {
		case err := <-done:
			if _, clearErr := io.WriteString(out, doctorClear); clearErr != nil {
				return clearErr
			}
			if report.Len() > 0 {
				if _, writeErr := io.WriteString(out, renderDoctorTerminal(report.String(), color)); writeErr != nil {
					return writeErr
				}
			}
			return err
		case <-ticker.C:
			frame++
			if _, err := io.WriteString(out, doctorProgress(spinner.Frame(frame), color)); err != nil {
				return err
			}
		}
	}
}

func doctorProgress(frame string, color bool) string {
	return doctorClear + doctorPaint("🐊 "+frame+" Checking Gator, models, and device…", doctorCyan, color)
}

func renderDoctorTerminal(raw string, color bool) string {
	var view strings.Builder
	view.WriteString(doctorPaint("🐊 Gator doctor", doctorBold+doctorGreen, color) + "\n")
	for _, line := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		if line == "" {
			continue
		}
		switch line {
		case "Dependencies:":
			view.WriteString("\n" + doctorPaint("🔧 Dependencies", doctorBold+doctorCyan, color) + "\n")
			continue
		case "Reviewed local-model eligibility (Gator guardrail):":
			view.WriteString("\n" + doctorPaint("🧠 Reviewed local models", doctorBold+doctorCyan, color) + "\n")
			continue
		}
		if strings.HasPrefix(line, "Local model host:") {
			view.WriteString("\n")
			line = "💻 " + line
		} else if strings.HasPrefix(line, "Ollama model storage:") {
			line = "📦 " + line
		} else if strings.HasPrefix(line, "Suggested verification:") {
			view.WriteString("\n")
			line = "🧪 " + line
		}
		if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "Local-model advice:") {
			view.WriteString(doctorPaint(line, doctorDim, color) + "\n")
			continue
		}
		label, value, found := strings.Cut(line, ": ")
		if !found {
			view.WriteString(line + "\n")
			continue
		}
		prefix := ""
		if strings.HasPrefix(label, "  ") {
			prefix = "  "
			label = strings.TrimPrefix(label, prefix)
		}
		if strings.HasPrefix(value, "enabled") || strings.HasPrefix(value, "installed") {
			prefix += "✓ "
		} else if strings.HasPrefix(value, "disabled") || strings.HasPrefix(value, "missing") {
			prefix += "○ "
		}
		view.WriteString(prefix + doctorPaint(label+":", doctorCyan, color) + " " + doctorPaint(value, doctorStatusColor(value), color) + "\n")
	}
	return view.String()
}

func doctorStatusColor(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(lower, "missing (required"), strings.Contains(lower, "not detected"), strings.Contains(lower, "expired"), strings.Contains(lower, "unsupported"):
		return doctorRed
	case strings.Contains(lower, "missing"), strings.Contains(lower, "not configured"), strings.Contains(lower, "disabled"), strings.Contains(lower, "unavailable"):
		return doctorAmber
	case strings.Contains(lower, "detected"), strings.Contains(lower, "installed"), strings.Contains(lower, "stored"), strings.Contains(lower, "enabled"), strings.Contains(lower, "configured"), strings.Contains(lower, "not required"), strings.Contains(lower, "set in environment"), strings.Contains(lower, "seatbelt"), strings.Contains(lower, "bubblewrap"):
		return doctorGreen
	default:
		return ""
	}
}

func doctorPaint(value, code string, color bool) string {
	if !color || code == "" {
		return value
	}
	return fmt.Sprintf("%s%s%s", code, value, doctorReset)
}
