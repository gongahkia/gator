package worksession

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/gongahkia/gator/internal/workactivity"
)

func (s Store) activityPath(conversation, revision string) string {
	return filepath.Join(s.root, conversation, "activity", revision+".json")
}

// A missing, damaged or newer display sidecar never prevents restoring the
// canonical revision. Keep optional UI state outside the strict replay schema.
func (s Store) readActivity(conversation, revision string) *workactivity.Trace {
	file, err := os.Open(s.activityPath(conversation, revision))
	if err != nil {
		return nil
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 512*1024 {
		return nil
	}
	var trace workactivity.Trace
	decoder := json.NewDecoder(io.LimitReader(file, 512*1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&trace) != nil || trace.FinishedAt == nil || trace.StartedAt.IsZero() || len(trace.Entries) > 160 {
		return nil
	}
	if trace.State != "completed" && trace.State != "failed" && trace.State != "stopped" {
		return nil
	}
	textBytes := 0
	ids := map[string]bool{}
	for _, entry := range trace.Entries {
		textBytes += len(entry.Text)
		if entry.ID == "" || ids[entry.ID] || len(entry.Tool) > 160 || textBytes > 64*1024 {
			return nil
		}
		ids[entry.ID] = true
		if entry.Kind != "text" && entry.Kind != "tool" && entry.Kind != "status" {
			return nil
		}
		if entry.State != "completed" && entry.State != "failed" && entry.State != "stopped" {
			return nil
		}
	}
	return &trace
}
