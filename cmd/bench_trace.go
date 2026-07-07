package cmd

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type benchTraceEvent struct {
	Stage  string `json:"stage"`
	Tokens int    `json:"tokens"`
}

func readTrace(path string) (int, int, bool) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer func() { _ = file.Close() }()
	var brain, drone int
	seen := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event benchTraceEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		seen = true
		switch event.Stage {
		case "plan", "edit":
			brain += event.Tokens
		case "compress":
			drone += event.Tokens
		}
	}
	return brain, drone, seen
}

func isTracePath(path string) bool {
	clean := filepath.ToSlash(path)
	return strings.Contains(clean, "/.paw/") || strings.Contains(filepath.Base(clean), "trace")
}
