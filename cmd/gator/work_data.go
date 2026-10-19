package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gongahkia/gator/internal/lifecycle"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/worktui"
)

func dataControl(ctx context.Context, stateDir, action string, args []string) (worktui.ControlScreen, error) {
	s, err := lifecycle.Open(stateDir)
	if err != nil {
		return worktui.ControlScreen{}, err
	}
	notice := ""
	reset := false
	switch action {
	case "backup", "verify":
		if len(args) != 1 {
			return worktui.ControlScreen{}, errors.New("choose one backup file path")
		}
		if action == "backup" {
			m, err := s.Export(ctx, args[0])
			if err != nil {
				return worktui.ControlScreen{}, err
			}
			notice = fmt.Sprintf("Exported %d private content files to %s. Store the archive privately; it is not encrypted.", len(m.Files), args[0])
		} else {
			m, err := lifecycle.VerifyBackup(ctx, args[0])
			if err != nil {
				return worktui.ControlScreen{}, err
			}
			notice = fmt.Sprintf("Verified %d files and their checksums. Backup created %s for state directory %s.", len(m.Files), m.CreatedAt.Format(time.RFC1123), m.StateDir)
		}
	case "retention":
		if len(args) != 3 {
			return worktui.ControlScreen{}, errors.New("provide history, saved knowledge, and snapshot retention days")
		}
		var days [3]int
		for i, arg := range args {
			days[i], err = strconv.Atoi(arg)
			if err != nil {
				return worktui.ControlScreen{}, err
			}
		}
		p := lifecycle.Policy{HistoryDays: days[0], KnowledgeDays: days[1], SnapshotDays: days[2]}
		if err = s.SavePolicy(p); err != nil {
			return worktui.ControlScreen{}, err
		}
		notice = "Retention saved. Automatic cleanup runs at startup and daily while the local supervisor is active. Preview cleanup to review the current eligible files."
	case "plan", "cleanup":
		var p lifecycle.Plan
		if action == "cleanup" {
			p, err = s.PreviewRetention(ctx, time.Now())
		} else {
			if len(args) != 1 {
				return worktui.ControlScreen{}, errors.New("choose a deletion scope")
			}
			p, err = s.PreviewDelete(ctx, args[0])
		}
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		return dataPlanScreen(p), nil
	case "apply":
		if len(args) != 1 {
			return worktui.ControlScreen{}, errors.New("preview data removal before applying it")
		}
		p, err := s.LoadPlan(args[0])
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		count, err := s.Apply(ctx, args[0])
		if err != nil {
			return worktui.ControlScreen{}, fmt.Errorf("removed %d files before stopping: %w; refresh and preview remaining data", count, err)
		}
		notice = fmt.Sprintf("Removed %d selected private files. Original projects and external copies were preserved.", count)
		reset = p.Scope == "all" || p.Scope == "history" || p.Scope == "retention"
	case "delete-options":
		return worktui.ControlScreen{Panel: uisnapshot.Panel{Kind: "data", Title: "Choose private content to delete", Blocks: []uisnapshot.Block{{Lines: []string{"Each choice opens a file-count and scope review. Nothing is removed until you confirm that review.", "Deleting a saved fact alone does not erase it from prior transcripts. All retained Work and knowledge includes those local historical copies.", "Original external project files, exports, credentials, provider settings, browser profiles, and shared usage counters remain."}}}}, Actions: []worktui.ControlAction{{Key: "K", Label: "Saved knowledge and retrieval cache", Action: "plan", Args: []string{"knowledge"}}, {Key: "H", Label: "Work history, transcripts, and excerpts", Action: "plan", Args: []string{"history"}}, {Key: "S", Label: "Unused source snapshots", Action: "plan", Args: []string{"snapshots"}}, {Key: "A", Label: "All retained Work and knowledge", Action: "plan", Args: []string{"all"}}}}, nil
	}
	view, err := s.Inventory(ctx)
	if err != nil {
		return worktui.ControlScreen{}, err
	}
	lines := []string{"Private state: " + view.StateDir, "Original project directories are outside these controls.", "Saved knowledge and historical inputs have separate lifetimes. Forgetting removes future recall; history can still contain an earlier excerpt.", "Backups include local snapshots, transcripts, drafts, and selected connected data. They exclude credentials, provider settings, runtime tokens, browser profiles, and usage counters.", "Deletion removes local files; it cannot promise secure erasure on SSDs or remove provider-held data and external backups."}
	for _, group := range view.Groups {
		lines = append(lines, fmt.Sprintf("%s: %d files · %s", group.Name, group.Files, byteLabel(group.Bytes)))
	}
	p := view.Policy
	retention := []string{"Zero days means keep until explicitly deleted.", fmt.Sprintf("Completed history: %s · saved facts/notes: %s · unused snapshots: %s", daysLabel(p.HistoryDays), daysLabel(p.KnowledgeDays), daysLabel(p.SnapshotDays)), "Ongoing goals, their conversations, pending suggestions, the latest successful check per job, and referenced snapshots are protected.", "Automatic expiry of saved knowledge applies to facts and notes; skills and learnings remain until explicitly removed.", "Automatic cleanup runs on Gator startup and daily while the supervisor is active. It waits when Work is using the data."}
	screen := worktui.ControlScreen{ResetConversation: reset, Panel: uisnapshot.Panel{Kind: "data", Title: "Private data and retention", Blocks: []uisnapshot.Block{{Title: "Storage and copies", Lines: lines}, {Title: "Retention", Lines: retention}}}}
	if notice != "" {
		screen.Panel.Blocks = append([]uisnapshot.Block{{Title: "Action result", Lines: []string{notice}}}, screen.Panel.Blocks...)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return screen, err
	}
	destination := filepath.Join(home, "gator-private-backup-"+time.Now().Format("20060102-150405")+".tar.gz")
	screen.Actions = []worktui.ControlAction{
		{Key: "B", Label: "Export private backup", Action: "backup", Description: "Write an unencrypted private archive containing retained Work, source snapshots, saved knowledge, and transcripts. Credentials and provider settings are excluded. Choose a path outside Gator's private data directory and keep the archive private.", Fields: []worktui.ControlField{{Label: "New backup file", Value: destination}}, Review: true},
		{Key: "V", Label: "Verify a backup", Action: "verify", Fields: []worktui.ControlField{{Label: "Existing backup file"}}, Description: "Read the archive and validate its file list and checksums. This does not restore or overwrite any data.", Review: true},
		{Key: "E", Label: "Edit retention", Action: "retention", Description: "Review automatic deletion of eligible local files. Zero keeps data until explicitly deleted. Ongoing goals, pending suggestions, current job baselines and referenced snapshots are protected. Original projects and external backups remain.", Fields: []worktui.ControlField{{Label: "Completed history days (0 keep)", Value: strconv.Itoa(p.HistoryDays)}, {Label: "Saved facts/notes days (0 keep)", Value: strconv.Itoa(p.KnowledgeDays)}, {Label: "Unused snapshot days (0 keep)", Value: strconv.Itoa(p.SnapshotDays)}}, Review: true},
		{Key: "C", Label: "Preview retention cleanup", Action: "cleanup"},
		{Key: "D", Label: "Choose content to delete", Action: "delete-options"},
		{Key: "R", Label: "Refresh", Action: "status"},
	}
	return screen, nil
}
func dataPlanScreen(p lifecycle.Plan) worktui.ControlScreen {
	lines := []string{"Scope: " + p.Scope, fmt.Sprintf("Selected: %d files · %s", p.Files, byteLabel(p.Bytes)), "Review ID: " + p.ID, "This review expires after one hour and is rejected if selected data changes."}
	lines = append(lines, p.Notes...)
	var paths []string
	for _, item := range p.Items {
		if !item.Directory {
			paths = append(paths, item.Path)
			if len(paths) == 20 {
				break
			}
		}
	}
	if len(paths) == 0 {
		paths = []string{"No files are eligible."}
	} else if p.Files > len(paths) {
		paths = append(paths, fmt.Sprintf("… %d more files; the saved review contains the complete list", p.Files-len(paths)))
	}
	screen := worktui.ControlScreen{Panel: uisnapshot.Panel{Kind: "confirmation", Title: "Review private data removal", Blocks: []uisnapshot.Block{{Title: "Removal agreement", Lines: lines}, {Title: "Selected file examples", Lines: paths}}}}
	if p.Files > 0 {
		screen.Actions = []worktui.ControlAction{{Key: "Enter", Label: fmt.Sprintf("Delete %d reviewed files", p.Files), Action: "apply", Args: []string{p.ID}}}
	}
	return screen
}
func daysLabel(days int) string {
	if days == 0 {
		return "keep until deleted"
	}
	return strconv.Itoa(days) + " days"
}
func byteLabel(bytes int64) string {
	if bytes < 1024 {
		return strconv.FormatInt(bytes, 10) + " B"
	}
	if bytes < 1<<20 {
		return fmt.Sprintf("%.1f KiB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(bytes)/(1<<20))
}

