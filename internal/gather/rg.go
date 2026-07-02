package gather

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gongahkia/paw/internal/envelope"
)

var termRE = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_./-]{2,}`)

func collectSearchHits(ctx context.Context, cwd, instruction string, maxBytes int) ([]envelope.RawUnit, error) {
	terms := instructionTerms(instruction)
	if len(terms) == 0 || cwd == "" {
		return nil, nil
	}
	if _, err := exec.LookPath("rg"); err == nil {
		return runRipgrep(ctx, cwd, terms, maxBytes)
	}
	return runGrep(ctx, cwd, terms, maxBytes)
}

func instructionTerms(instruction string) []string {
	seen := map[string]bool{}
	var terms []string
	for _, raw := range termRE.FindAllString(instruction, -1) {
		term := strings.Trim(raw, "./-")
		key := strings.ToLower(term)
		if stopTerm(key) || seen[key] {
			continue
		}
		seen[key] = true
		terms = append(terms, term)
		if len(terms) == 8 {
			break
		}
	}
	return terms
}

func runRipgrep(ctx context.Context, cwd string, terms []string, maxBytes int) ([]envelope.RawUnit, error) {
	hits := map[string]*searchHit{}
	for _, term := range terms {
		args := []string{"--json", "-n", "--color", "never", "--glob", "!.git/**", "--glob", "!node_modules/**", "--glob", "!.paw/**", "--", term, cwd}
		out, err := exec.CommandContext(ctx, "rg", args...).Output()
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			continue
		}
		if err != nil {
			return nil, err
		}
		parseRGJSON(cwd, out, hits, maxBytes)
	}
	return searchUnits(hits), nil
}

func parseRGJSON(cwd string, out []byte, hits map[string]*searchHit, maxBytes int) {
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		var event rgEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Type != "match" {
			continue
		}
		path := relPath(cwd, event.Data.Path.Text)
		line := event.Data.LineNumber
		text := strings.TrimRight(event.Data.Lines.Text, "\n")
		addHit(hits, path, line, text, maxBytes)
	}
}

func runGrep(ctx context.Context, cwd string, terms []string, maxBytes int) ([]envelope.RawUnit, error) {
	hits := map[string]*searchHit{}
	for _, term := range terms {
		args := []string{"-RIn", "--exclude-dir=.git", "--exclude-dir=node_modules", "--exclude-dir=.paw", "--", term, cwd}
		out, err := exec.CommandContext(ctx, "grep", args...).Output()
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			continue
		}
		if err != nil {
			return nil, err
		}
		parseGrep(cwd, out, hits, maxBytes)
	}
	return searchUnits(hits), nil
}

func parseGrep(cwd string, out []byte, hits map[string]*searchHit, maxBytes int) {
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ":", 3)
		if len(parts) != 3 {
			continue
		}
		line, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		addHit(hits, relPath(cwd, parts[0]), line, parts[2], maxBytes)
	}
}

func addHit(hits map[string]*searchHit, path string, line int, text string, maxBytes int) {
	if path == "" {
		return
	}
	hit := hits[path]
	if hit == nil {
		hit = &searchHit{path: path, start: line, end: line}
		hits[path] = hit
	}
	if line < hit.start {
		hit.start = line
	}
	if line > hit.end {
		hit.end = line
	}
	next := hit.text + strconv.Itoa(line) + ": " + text + "\n"
	if maxBytes <= 0 || len(next) <= maxBytes {
		hit.text = next
	}
}

func searchUnits(hits map[string]*searchHit) []envelope.RawUnit {
	paths := make([]string, 0, len(hits))
	for path := range hits {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	units := make([]envelope.RawUnit, 0, len(paths))
	for _, path := range paths {
		hit := hits[path]
		units = append(units, envelope.RawUnit{
			Kind:      "search_hits",
			Path:      path,
			StartLine: hit.start,
			EndLine:   hit.end,
			Text:      hit.text,
		})
	}
	return units
}

func relPath(cwd, path string) string {
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(rel)
}

func stopTerm(term string) bool {
	switch term {
	case "the", "and", "for", "with", "that", "this", "from", "into", "fix", "bug", "file", "code":
		return true
	default:
		return false
	}
}

type searchHit struct {
	path  string
	start int
	end   int
	text  string
}

type rgEvent struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		LineNumber int `json:"line_number"`
		Lines      struct {
			Text string `json:"text"`
		} `json:"lines"`
	} `json:"data"`
}
