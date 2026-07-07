package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

type benchRow struct {
	Config      string
	Tasks       int
	PassRate    string
	BrainTokens string
	DroneTokens string
	WallSeconds string
}

func (s benchSummary) row(config string) benchRow {
	return benchRow{
		Config:      config,
		Tasks:       s.Tasks,
		PassRate:    ratio(s.Passed, s.Tasks),
		BrainTokens: formatMedian(s.BrainTokens, 0),
		DroneTokens: formatMedian(s.DroneTokens, 0),
		WallSeconds: formatMedian(s.WallSeconds, 1),
	}
}

func printBenchTable(w io.Writer, rows []benchRow) error {
	if _, err := fmt.Fprintln(w, "| config | tasks | pass@1 | brain_in_tok/task (median) | drone_tok/task | wall_s/task |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| --- | ---: | ---: | ---: | ---: | ---: |"); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(w, row.markdown()); err != nil {
			return err
		}
	}
	return nil
}

const (
	resultsStartMarker = "<!-- paw-results:start -->"
	resultsEndMarker   = "<!-- paw-results:end -->"
)

func updateResultsFile(path string, row benchRow) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	start, end := markerBounds(lines)
	if start == -1 || end == -1 || start >= end {
		return fmt.Errorf("results table markers not found in %s", path)
	}
	next := row.markdown()
	replaced := false
	for i := start + 1; i < end; i++ {
		if strings.HasPrefix(lines[i], "| "+row.Config+" |") {
			lines[i] = next
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines[:end], append([]string{next}, lines[end:]...)...)
	}
	out := strings.Join(lines, "\n")
	return os.WriteFile(path, []byte(out), 0o644)
}

func markerBounds(lines []string) (int, int) {
	start, end := -1, -1
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case resultsStartMarker:
			start = i
		case resultsEndMarker:
			end = i
			return start, end
		}
	}
	return start, end
}

func (r benchRow) markdown() string {
	return fmt.Sprintf("| %s | %d | %s | %s | %s | %s |", r.Config, r.Tasks, r.PassRate, r.BrainTokens, r.DroneTokens, r.WallSeconds)
}

func ratio(numerator, denominator int) string {
	if denominator == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", float64(numerator)/float64(denominator))
}

func formatMedian(values []float64, decimals int) string {
	if len(values) == 0 {
		return "n/a"
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	value := sorted[mid]
	if len(sorted)%2 == 0 {
		value = (sorted[mid-1] + sorted[mid]) / 2
	}
	return fmt.Sprintf("%."+strconv.Itoa(decimals)+"f", value)
}
