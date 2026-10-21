package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	clearGoal := false
	var removedConversations []string
	var sourceMoves []worktui.SourceMove
	switch action {
	case "finish-restore-review":
		path := filepath.Join(stateDir, "gator", "data-control", "restore-reviewed.json")
		if err = os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)), 0600); err != nil {
			return worktui.ControlScreen{}, err
		}
		notice = "Restore setup review marked complete. No checks or provider requests were enabled."
	case "relink-source":
		if len(args) != 1 {
			return worktui.ControlScreen{}, errors.New("choose a source folder to relink")
		}
		return worktui.ControlScreen{Panel: uisnapshot.Panel{Kind: "data", Title: "Relink a moved source folder", Blocks: []uisnapshot.Block{{Lines: []string{"Missing source: " + args[0], "Choose the corresponding existing project folder. The next screen reviews every metadata change; source files are never changed."}}}}, Actions: []worktui.ControlAction{{Key: "Enter", Label: "Choose the replacement folder", Action: "relink-plan", Fields: []worktui.ControlField{{Label: "Old source folder", Kind: "directory", Value: args[0]}, {Label: "Existing new source folder", Kind: "directory"}}, Review: true}}}, nil

	case "relink-plan":
		if len(args) != 2 {
			return worktui.ControlScreen{}, errors.New("choose the old and new source directories")
		}
		plan, err := s.PreviewRelink(ctx, args[0], args[1])
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		return relinkPlanScreen(plan), nil
	case "relink-apply":
		if len(args) != 1 {
			return worktui.ControlScreen{}, errors.New("review source mapping before applying it")
		}
		plan, err := s.LoadRelinkPlan(args[0])
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		count, err := s.Relink(ctx, args[0])
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		sourceMoves = []worktui.SourceMove{{From: plan.From, To: plan.To}}
		notice = fmt.Sprintf("Relinked %d metadata references. Affected checks are paused. Existing transcript and artifact bytes retain their original evidence.", count)

	case "restore-plan":
		if len(args) != 3 {
			return worktui.ControlScreen{}, errors.New("choose the archive, a new profile directory, and optional source mappings")
		}
		mappings, err := lifecycle.SourceMappings(strings.ReplaceAll(args[2], ";", "\n"))
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		plan, err := s.PreviewRestore(ctx, args[0], args[1], mappings)
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		return restorePlanScreen(plan), nil
	case "restore-apply":
		if len(args) != 1 {
			return worktui.ControlScreen{}, errors.New("preview the restore before applying it")
		}
		receipt, err := s.Restore(ctx, args[0])
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		notice = "Restored a separate profile at " + receipt.Plan.Target + ". This session still uses " + stateDir + ". Start Gator with " + profileLaunch(receipt.Plan.Target) + ". Review provider setup, source folders, shared pause, and paused checks in that profile."

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
		if len(args) != 4 {
			return worktui.ControlScreen{}, errors.New("provide history, saved knowledge, and snapshot retention days")
		}
		var days [3]int
		for i, arg := range args[1:] {
			days[i], err = strconv.Atoi(arg)
			if err != nil {
				return worktui.ControlScreen{}, err
			}
		}
		p := lifecycle.Policy{HistoryDays: days[0], KnowledgeDays: days[1], SnapshotDays: days[2]}
		var expected lifecycle.Policy
		if err = json.Unmarshal([]byte(args[0]), &expected); err != nil {
			return worktui.ControlScreen{}, err
		}
		if err = s.SaveReviewedPolicy(p, expected); err != nil {
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
		reset = p.Scope == "all" || p.Scope == "history"
		clearGoal = p.Scope == "all"
		if p.Scope == "retention" {
			seen := map[string]bool{}
			for _, item := range p.Items {
				parts := strings.Split(filepath.ToSlash(item.Path), "/")
				if len(parts) > 1 && parts[0] == "conversations" && !seen[parts[1]] {
					removedConversations = append(removedConversations, parts[1])
					seen[parts[1]] = true
				}
			}
		}
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
	var restored lifecycle.RestoreReceipt
	if data, readErr := os.ReadFile(filepath.Join(view.StateDir, "gator", "data-control", "restored.json")); readErr == nil {
		if err = json.Unmarshal(data, &restored); err != nil {
			return worktui.ControlScreen{}, err
		}
		lines = append(lines, "Restored profile: "+restored.Plan.Target, "Original archive: "+restored.Plan.Archive, "Open this profile with "+profileLaunch(view.StateDir), "Use L to relink a moved source folder. Set up /model and connectors, then review /usage and paused /followup checks.")
		sources, sourceErr := s.SourceFolders(ctx)
		if sourceErr != nil {
			return worktui.ControlScreen{}, sourceErr
		}
		for _, source := range sources {
			status := "available"
			if info, err := os.Stat(source.Destination); err != nil || !info.IsDir() {
				status = "missing — relink this source with L"
			}
			lines = append(lines, source.Destination+" · "+status)
		}
	}
	p := view.Policy
	expected, _ := json.Marshal(p)
	retention := []string{"Zero days means keep until explicitly deleted.", fmt.Sprintf("Completed history: %s · saved facts/notes: %s · unused snapshots: %s", daysLabel(p.HistoryDays), daysLabel(p.KnowledgeDays), daysLabel(p.SnapshotDays)), "Ongoing goals, their conversations, pending suggestions, the latest successful check per job, and referenced snapshots are protected.", "Automatic expiry of saved knowledge applies to facts and notes; skills and learnings remain until explicitly removed.", "Automatic cleanup runs on Gator startup and daily while the supervisor is active. It waits when Work is using the data."}
	if !view.LastSweep.At.IsZero() {
		retention = append(retention, fmt.Sprintf("Last automatic cleanup: %s · %d files removed", view.LastSweep.At.Local().Format(time.RFC1123), view.LastSweep.Removed))
	}
	screen := worktui.ControlScreen{SourceMoves: sourceMoves, ResetConversation: reset, ClearGoal: clearGoal, RemovedConversations: removedConversations, Panel: uisnapshot.Panel{Kind: "data", Title: "Private data and retention", Blocks: []uisnapshot.Block{{Title: "Storage and copies", Lines: lines}, {Title: "Retention", Lines: retention}}}}
	if notice != "" {
		screen.Panel.Blocks = append([]uisnapshot.Block{{Title: "Action result", Lines: []string{notice}}}, screen.Panel.Blocks...)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return screen, err
	}
	destination := filepath.Join(home, "gator-private-backup-"+time.Now().Format("20060102-150405")+".tar.gz")
	screen.Actions = []worktui.ControlAction{
		{Key: "L", Label: "Relink a moved source folder", Action: "relink-plan", Description: "Preview changes to source references in goals, conversations, checks, frozen source metadata, and historical workspace navigation. Original source files and sealed artifact bytes are untouched. Affected checks will be paused.", Fields: []worktui.ControlField{{Label: "Old source folder", Kind: "directory"}, {Label: "Existing new source folder", Kind: "directory"}}, Review: true},
		{Key: "I", Label: "Restore into a new profile", Action: "restore-plan", Description: "Verify a backup and preview migration into a new profile directory. The current profile stays active. Review the exact destination and source mappings before final confirmation.", Fields: []worktui.ControlField{{Label: "Existing backup archive", Kind: "file"}, {Label: "New profile directory", Kind: "save", Value: filepath.Join(home, "gator-restored-"+time.Now().Format("20060102-150405"))}, {Label: "Source folder mappings (optional)", Kind: "mappings", Help: "Match original folders to their existing replacements. Leave empty to retain original paths. Original folder paths cannot contain = or ;, and replacement paths cannot contain ;."}}, Review: true},
		{Key: "B", Label: "Export private backup", Action: "backup", Description: "Write an unencrypted private archive containing retained Work, source snapshots, saved knowledge, and transcripts. Credentials and provider settings are excluded. Choose a path outside Gator's private data directory and keep the archive private.", Fields: []worktui.ControlField{{Label: "New backup file", Kind: "save", Value: destination}}, Review: true},
		{Key: "V", Label: "Verify a backup", Action: "verify", Fields: []worktui.ControlField{{Label: "Existing backup file", Kind: "file"}}, Description: "Read the archive and validate its file list and checksums. This does not restore or overwrite any data.", Review: true},
		{Key: "E", Label: "Edit retention", Action: "retention", Args: []string{string(expected)}, Description: "Review automatic deletion of eligible local files. Zero keeps data until explicitly deleted. Ongoing goals, pending suggestions, current job baselines and referenced snapshots are protected. Original projects and external backups remain.", Fields: []worktui.ControlField{{Label: "Completed history days (0 keep)", Kind: "number", Value: strconv.Itoa(p.HistoryDays)}, {Label: "Saved facts/notes days (0 keep)", Kind: "number", Value: strconv.Itoa(p.KnowledgeDays)}, {Label: "Unused snapshot days (0 keep)", Kind: "number", Value: strconv.Itoa(p.SnapshotDays)}}, Review: true},
		{Key: "C", Label: "Preview retention cleanup", Action: "cleanup"},
		{Key: "D", Label: "Choose content to delete", Action: "delete-options"},
		{Key: "R", Label: "Refresh", Action: "status"},
	}
	if restored.Plan.Target != "" {
		screen.Actions = append(screen.Actions, worktui.ControlAction{Key: "F", Label: "Finish restore setup review", Action: "finish-restore-review", Review: true, Description: "Mark the restore checklist reviewed. This does not enable jobs, unpause usage, or grant any permissions. Source, model, and background issues remain visible in /overview."})
	}
	return screen, nil
}
func dataPlanScreen(p lifecycle.Plan) worktui.ControlScreen {
	lines := []string{"Scope: " + p.Scope, fmt.Sprintf("Selected: %d files · %d folders · %s", p.Files, p.Folders, byteLabel(p.Bytes)), "Review ID: " + p.ID, "This review expires after one hour and is rejected if selected data changes."}
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
	if len(p.Items) > 0 {
		screen.Actions = []worktui.ControlAction{{Key: "Enter", Label: fmt.Sprintf("Delete %d files and %d folders", p.Files, p.Folders), Action: "apply", Args: []string{p.ID}}}
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

func restorePlanScreen(p lifecycle.RestorePlan) worktui.ControlScreen {
	lines := []string{
		"Review ID: " + p.ID,
		"Archive: " + p.Archive,
		"Archive SHA256: " + p.ArchiveSHA256,
		"Original profile: " + p.Origin,
		"New profile: " + p.Target,
		fmt.Sprintf("Verified archive: %d files · %s", p.Files, byteLabel(p.Bytes)),
		fmt.Sprintf("%d enabled checks will be paused; %d interrupted runs become failed history.", p.PausedChecks, p.InterruptedRuns),
		"The review expires after one hour. Changed archive bytes, paths, source availability, or mappings require a new preview.",
	}
	lines = append(lines, p.Notes...)
	var sources []string
	for _, source := range p.Sources {
		status := "available"
		if !source.Available {
			status = "missing — choose an existing source mapping to resume this project"
		}
		sources = append(sources, source.Original+" → "+source.Destination+" · "+status)
	}
	if len(sources) == 0 {
		sources = []string{"No source folders are referenced."}
	}
	return worktui.ControlScreen{Panel: uisnapshot.Panel{Kind: "confirmation", Title: "Review restore into a fresh profile", Blocks: []uisnapshot.Block{{Title: "Restore agreement", Lines: lines}, {Title: "Source folders and mappings", Lines: sources}}}, Actions: []worktui.ControlAction{{Key: "Enter", Label: "Restore this reviewed profile", Action: "restore-apply", Args: []string{p.ID}}}}
}

func profileLaunch(root string) string {
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	return "GATOR_STATE_DIR=" + quote(root) + " GATOR_CONFIG_DIR=" + quote(filepath.Join(root, "config")) + " gator"
}
func relinkPlanScreen(p lifecycle.RelinkPlan) worktui.ControlScreen {
	lines := []string{"From: " + p.From, "To: " + p.To, fmt.Sprintf("Update %d navigation metadata files. Affected checks will be paused.", len(p.Edits)), "Original projects, artifact bytes, transcript text, and evidence hashes remain unchanged.", "The review expires after one hour; changed references require a fresh preview.", "An interrupted update is rolled back from a private recovery journal before this profile is used again."}
	var paths []string
	for _, edit := range p.Edits {
		paths = append(paths, edit.Path)
	}
	return worktui.ControlScreen{Panel: uisnapshot.Panel{Kind: "confirmation", Title: "Review source folder relink", Blocks: []uisnapshot.Block{{Title: "Source mapping", Lines: lines}, {Title: "Affected metadata", Lines: paths}}}, Actions: []worktui.ControlAction{{Key: "Enter", Label: "Relink these reviewed source references", Action: "relink-apply", Args: []string{p.ID}}}}
}
