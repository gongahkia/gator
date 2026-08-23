package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gongahkia/gator/internal/journal"
)

const childUsage = `usage:
  gator child list RUN_RECORD_PATH
  gator child show RUN_RECORD_PATH CHILD_RUN_ID
  gator child batches RUN_RECORD_PATH
  gator child batch RUN_RECORD_PATH BATCH_ID`

// childCommand exposes parent-owned writer manifests without replaying model
// conversation content or patch text. It is a recovery surface for retained
// child worktrees, including children that failed before they could save a
// complete child session of their own.
func childCommand(arguments []string, out io.Writer) error {
	if len(arguments) < 2 {
		return errors.New(childUsage)
	}
	switch arguments[0] {
	case "list":
		if len(arguments) != 2 {
			return errors.New(childUsage)
		}
		manifests, err := journal.ListChildManifests(arguments[1])
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				_, writeErr := fmt.Fprintln(out, "No retained writer children.")
				return writeErr
			}
			return err
		}
		if len(manifests) == 0 {
			_, err := fmt.Fprintln(out, "No retained writer children.")
			return err
		}
		if _, err := fmt.Fprintf(out, "Retained writer children: %s\n", arguments[1]); err != nil {
			return err
		}
		for _, manifest := range manifests {
			role := manifest.Role
			if role == "" {
				role = "-"
			}
			patch := "none"
			if manifest.PatchBytes > 0 {
				patch = fmt.Sprintf("%d bytes", manifest.PatchBytes)
			}
			batch := ""
			if manifest.BatchID != "" {
				batch = "  batch=" + manifest.BatchID
			}
			if _, err := fmt.Fprintf(out, "  %s  %s  role=%s  patch=%s%s\n", manifest.ID, manifest.Status, role, patch, batch); err != nil {
				return err
			}
		}
		return nil
	case "show":
		if len(arguments) != 3 {
			return errors.New(childUsage)
		}
		manifest, err := journal.LoadChildManifest(arguments[1], arguments[2])
		if err != nil {
			return err
		}
		return printChildManifest(manifest, out)
	case "batches":
		if len(arguments) != 2 {
			return errors.New(childUsage)
		}
		batches, err := journal.ListChildBatchManifests(arguments[1])
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				_, writeErr := fmt.Fprintln(out, "No retained parallel writer batches.")
				return writeErr
			}
			return err
		}
		if len(batches) == 0 {
			_, err := fmt.Fprintln(out, "No retained parallel writer batches.")
			return err
		}
		if _, err := fmt.Fprintf(out, "Retained parallel writer batches: %s\n", arguments[1]); err != nil {
			return err
		}
		for _, batch := range batches {
			if _, err := fmt.Fprintf(out, "  %s  %s  children=%s  conflicts=%d\n", batch.ID, batch.Status, strings.Join(batch.ChildIDs, ","), len(batch.Conflicts)); err != nil {
				return err
			}
		}
		return nil
	case "batch":
		if len(arguments) != 3 {
			return errors.New(childUsage)
		}
		batch, err := journal.LoadChildBatchManifest(arguments[1], arguments[2])
		if err != nil {
			return err
		}
		return printChildBatchManifest(batch, out)
	default:
		return fmt.Errorf("unknown child command %q\n%s", arguments[0], childUsage)
	}
}

func printChildManifest(manifest journal.ChildManifest, out io.Writer) error {
	fields := []struct {
		name  string
		value string
	}{
		{"id", manifest.ID},
		{"parent run", manifest.ParentRunID},
		{"kind", manifest.Kind},
		{"status", string(manifest.Status)},
		{"role", valueOrDash(manifest.Role)},
		{"batch", valueOrDash(manifest.BatchID)},
		{"repository", manifest.Repository},
		{"worktree", valueOrDash(manifest.WorktreePath)},
		{"baseline", valueOrDash(manifest.BaseCommit)},
		{"child record", valueOrDash(manifest.StatePath)},
		{"task sha256", manifest.TaskSHA256},
		{"patch", childPatchSummary(manifest)},
		{"patch sha256", valueOrDash(manifest.PatchSHA256)},
		{"review required", fmt.Sprintf("%t", manifest.ReviewRequired)},
		{"worktree retained", fmt.Sprintf("%t", manifest.WorktreeRetained)},
		{"started", manifest.StartedAt.Format("2006-01-02T15:04:05Z07:00")},
		{"updated", manifest.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")},
	}
	if manifest.FinishedAt != nil {
		fields = append(fields, struct {
			name  string
			value string
		}{"finished", manifest.FinishedAt.Format("2006-01-02T15:04:05Z07:00")})
	}
	if manifest.Error != "" {
		fields = append(fields, struct {
			name  string
			value string
		}{"error", strings.ReplaceAll(manifest.Error, "\n", " ")})
	}
	if _, err := fmt.Fprintln(out, "Writer child manifest:"); err != nil {
		return err
	}
	for _, field := range fields {
		if _, err := fmt.Fprintf(out, "  %s: %s\n", field.name, field.value); err != nil {
			return err
		}
	}
	return nil
}

func printChildBatchManifest(manifest journal.ChildBatchManifest, out io.Writer) error {
	if _, err := fmt.Fprintln(out, "Parallel writer batch manifest:"); err != nil {
		return err
	}
	fields := []struct {
		name  string
		value string
	}{
		{"id", manifest.ID},
		{"parent run", manifest.ParentRunID},
		{"status", string(manifest.Status)},
		{"children", strings.Join(manifest.ChildIDs, ", ")},
		{"conflicts", fmt.Sprintf("%d", len(manifest.Conflicts))},
		{"started", manifest.StartedAt.Format("2006-01-02T15:04:05Z07:00")},
		{"updated", manifest.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")},
	}
	if manifest.FinishedAt != nil {
		fields = append(fields, struct {
			name  string
			value string
		}{"finished", manifest.FinishedAt.Format("2006-01-02T15:04:05Z07:00")})
	}
	if manifest.Error != "" {
		fields = append(fields, struct {
			name  string
			value string
		}{"error", strings.ReplaceAll(manifest.Error, "\n", " ")})
	}
	for _, field := range fields {
		if _, err := fmt.Fprintf(out, "  %s: %s\n", field.name, field.value); err != nil {
			return err
		}
	}
	for index, conflict := range manifest.Conflicts {
		if _, err := fmt.Fprintf(out, "  conflict %d: %s; children=%s; paths=%s; %s\n", index+1, conflict.Kind, strings.Join(conflict.ChildIDs, ","), strings.Join(conflict.Paths, ","), conflict.Detail); err != nil {
			return err
		}
	}
	return nil
}

func childPatchSummary(manifest journal.ChildManifest) string {
	if manifest.PatchBytes == 0 {
		return "none"
	}
	if manifest.PatchAvailable {
		return fmt.Sprintf("%d bytes (available to parent)", manifest.PatchBytes)
	}
	return fmt.Sprintf("%d bytes (inspect retained worktree)", manifest.PatchBytes)
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
