package workrun

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/gongahkia/gator/internal/workspace"
)

const (
	maxInteractionRuns    = 4096
	maxInteractionRecords = 256
	maxInteractionBytes   = 32 * 1024 * 1024
)

// ReadInteractionRecords inspects retained decisions without contacting the
// original process or conferring authority to answer one from another process.
// An empty runID scans all retained runs. The caller must treat "pending" as
// recorded state rather than proof that a process is still waiting.
func ReadInteractionRecords(stateDir, runID string) ([]InteractionRecord, error) {
	if runID != "" && !validInteractionRunID(runID) {
		return nil, errors.New("invalid Work run ID")
	}
	base := filepath.Join(stateDir, "gator", "interactions")
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list Work interactions: %w", err)
	}
	if len(entries) > maxInteractionRuns {
		return nil, errors.New("too many retained Work runs to scan; specify a run ID")
	}
	root, err := workspace.Open(base)
	if err != nil {
		return nil, err
	}
	var records []InteractionRecord
	var totalBytes int
	for _, entry := range entries {
		name := entry.Name()
		if !validInteractionRunID(name) || !entry.IsDir() || (runID != "" && name != runID) {
			continue
		}
		files, err := os.ReadDir(filepath.Join(base, name))
		if err != nil {
			return nil, fmt.Errorf("list decisions for %s: %w", name, err)
		}
		for _, file := range files {
			if !file.Type().IsRegular() || !strings.HasSuffix(file.Name(), ".json") {
				continue
			}
			id, err := strconv.Atoi(strings.TrimSuffix(file.Name(), ".json"))
			if err != nil || id < 1 {
				continue
			}
			if len(records) >= maxInteractionRecords {
				return nil, errors.New("too many retained decisions to scan; specify a run ID")
			}
			payload, err := root.ReadRegularFile(filepath.Join(name, file.Name()), 1024*1024)
			if err != nil {
				return nil, fmt.Errorf("read decision %s/%d: %w", name, id, err)
			}
			totalBytes += len(payload)
			if totalBytes > maxInteractionBytes {
				return nil, errors.New("retained decisions exceed the safe scan size; specify a run ID")
			}
			var record InteractionRecord
			if err := json.Unmarshal(payload, &record); err != nil {
				return nil, fmt.Errorf("decode decision %s/%d: %w", name, id, err)
			}
			if record.Version != 1 || record.Interaction.ID != id || record.Interaction.Kind == "" ||
				!slices.Contains([]string{"pending", "resolved", "cancelled"}, record.Status) ||
				(record.Status == "resolved") != (record.Approved != nil) {
				return nil, fmt.Errorf("invalid retained decision %s/%d", name, id)
			}
			record.RunID = name
			records = append(records, record)
		}
	}
	slices.SortFunc(records, func(a, b InteractionRecord) int {
		if a.RunID != b.RunID {
			return strings.Compare(b.RunID, a.RunID)
		}
		return b.Interaction.ID - a.Interaction.ID
	})
	return records, nil
}

func validInteractionRunID(id string) bool {
	if len(id) < 1 || len(id) > 96 {
		return false
	}
	for i, c := range id {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || i > 0 && (c == '-' || c == '_') {
			continue
		}
		return false
	}
	return true
}
