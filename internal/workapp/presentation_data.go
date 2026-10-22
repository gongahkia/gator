package workapp

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/gongahkia/gator/internal/learning"
)

// View is an empty Bubble Tea adapter; the React terminal UI owns all rendering.
func (m Model) View() string { return "" }

func safeInboxText(value string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, value)
}

func historyArtifactLines(items []ArtifactSummary, issue string) []string {
	lines := make([]string, 0, min(3, len(items))+1)
	for _, item := range items[:min(3, len(items))] {
		mark := "✓"
		if !item.Valid {
			mark = "!"
		}
		lines = append(lines, fmt.Sprintf("%s %s · %s · %d bytes", mark, item.Path, item.MediaType, item.Bytes))
	}
	if len(items) > 3 {
		lines = append(lines, fmt.Sprintf("+%d more artifacts", len(items)-3))
	}
	if issue != "" {
		lines = append(lines, issue)
	}
	return lines
}

func historyFeedbackLines(items []learning.Observation) []string {
	lines := make([]string, 0, min(3, len(items)))
	for _, item := range items[:min(3, len(items))] {
		line := string(item.Signal)
		if item.Summary != "" {
			line += " · " + singleLine(item.Summary)
		}
		lines = append(lines, line)
	}
	if len(items) > 3 {
		lines = append(lines, fmt.Sprintf("+%d more observations", len(items)-3))
	}
	return lines
}

func historyLearningLines(items []learning.Record) []string {
	lines := make([]string, 0, min(3, len(items)))
	for _, item := range items[:min(3, len(items))] {
		lines = append(lines, string(item.Status)+" · "+singleLine(item.Content))
	}
	if len(items) > 3 {
		lines = append(lines, fmt.Sprintf("+%d more learnings", len(items)-3))
	}
	return lines
}

func compactLines(items []string, limit int) []string {
	if len(items) == 0 {
		return nil
	}
	result := append([]string(nil), items[:min(limit, len(items))]...)
	if len(items) > limit {
		result = append(result, fmt.Sprintf("+%d more", len(items)-limit))
	}
	return result
}

func boundedSectionText(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 4000 {
		return value
	}
	return strings.TrimSpace(value[:3997]) + "…"
}

func normalizeTheme(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "contrast":
		return "contrast"
	case "mono":
		return "mono"
	default:
		return "gator"
	}
}

func truncate(value string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	if maximum == 1 {
		return "…"
	}
	return string(runes[:maximum-1]) + "…"
}
