package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/lifecycle"
	"github.com/gongahkia/gator/internal/state"
)

func dataCommand(args []string, out io.Writer) error {
	root, err := state.ResolveDir("")
	if err != nil {
		return err
	}
	s, err := lifecycle.Open(root)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if len(args) == 0 || args[0] == "status" {
		if len(args) > 1 && (len(args) != 2 || args[1] != "--json") {
			return errors.New("usage: gator data status [--json]")
		}
		if len(args) == 2 && args[1] == "--json" {
			view, err := s.Inventory(ctx)
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(view)
		}
		screen, err := dataControl(ctx, root, "status", nil)
		if err != nil {
			return err
		}
		for _, block := range screen.Panel.Blocks {
			for _, line := range block.Lines {
				if _, err = fmt.Fprintln(out, line); err != nil {
					return err
				}
			}
		}
		return nil
	}
	switch args[0] {
	case "preview":
		if len(args) < 2 || len(args) > 3 || (len(args) == 3 && args[2] != "--json") {
			return errors.New("usage: gator data preview knowledge|history|snapshots|all [--json]")
		}
		p, err := s.PreviewDelete(ctx, args[1])
		if err != nil {
			return err
		}
		return printDataPlan(out, p, len(args) == 3 && args[2] == "--json")
	case "apply":
		if len(args) != 3 || args[2] != "--yes" {
			return errors.New("usage: gator data apply REVIEW_ID --yes; preview the exact scope first")
		}
		count, err := s.Apply(ctx, args[1])
		if err != nil {
			return fmt.Errorf("removed %d files before stopping: %w", count, err)
		}
		_, err = fmt.Fprintf(out, "Removed %d reviewed private files. Original projects and external copies remain.\n", count)
		return err
	case "cleanup":
		if len(args) > 2 || (len(args) == 2 && args[1] != "--yes") {
			return errors.New("usage: gator data cleanup [--yes]")
		}
		p, err := s.PreviewRetention(ctx, time.Now())
		if err != nil {
			return err
		}
		if len(args) == 1 {
			return printDataPlan(out, p, false)
		}
		count, err := s.Apply(ctx, p.ID)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Removed %d files selected by the saved retention policy.\n", count)
		return err
	case "retention":
		prior, err := s.Policy()
		if err != nil {
			return err
		}
		flags := flag.NewFlagSet("data retention", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		history := flags.Int("history-days", prior.HistoryDays, "completed history; zero keeps data until deletion")
		knowledge := flags.Int("knowledge-days", prior.KnowledgeDays, "saved facts and notes; zero keeps data until deletion")
		snapshots := flags.Int("snapshot-days", prior.SnapshotDays, "unreferenced snapshots; zero keeps data until deletion")
		yes := flags.Bool("yes", false, "save reviewed automatic retention policy")
		if err = flags.Parse(args[1:]); err != nil {
			return err
		}
		if len(flags.Args()) > 0 {
			return errors.New("unexpected retention arguments")
		}
		p := lifecycle.Policy{HistoryDays: *history, KnowledgeDays: *knowledge, SnapshotDays: *snapshots}
		if err = p.Validate(); err != nil {
			return err
		}
		if _, err = fmt.Fprintf(out, "History: %s; saved facts/notes: %s; unused snapshots: %s.\nOngoing goals, pending suggestions, job baselines, and referenced snapshots are protected. Original projects, credentials, and external copies remain.\n", daysLabel(p.HistoryDays), daysLabel(p.KnowledgeDays), daysLabel(p.SnapshotDays)); err != nil {
			return err
		}
		if !*yes {
			_, err = fmt.Fprintln(out, "Review these settings, then repeat with --yes to enable automatic cleanup at startup and daily while the supervisor is active.")
			return err
		}
		return s.SaveReviewedPolicy(p, prior)
	case "export", "verify":
		if len(args) != 2 {
			return errors.New("usage: gator data export NEW_ARCHIVE.tar.gz | verify ARCHIVE.tar.gz")
		}
		if args[0] == "export" {
			manifest, err := s.Export(ctx, args[1])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(out, "Exported %d private content files to %s (unencrypted; credentials and settings excluded).\n", len(manifest.Files), args[1])
			return err
		}
		manifest, err := lifecycle.VerifyBackup(ctx, args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Verified %d files, created %s for %s.\n", len(manifest.Files), manifest.CreatedAt.Format(time.RFC3339), manifest.StateDir)
		return err
	default:
		return errors.New("usage: gator data status|preview|apply|cleanup|retention|export|verify")
	}
}
func printDataPlan(out io.Writer, p lifecycle.Plan, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(out).Encode(p)
	}
	if _, err := fmt.Fprintf(out, "Review %s\nScope: %s · %d files · %s\n%s\n", p.ID, p.Scope, p.Files, byteLabel(p.Bytes), strings.Join(p.Notes, "\n")); err != nil {
		return err
	}
	for _, item := range p.Items {
		if !item.Directory {
			if _, err := fmt.Fprintln(out, item.Path); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintf(out, "Nothing deleted. To apply this reviewed selection: gator data apply %s --yes\n", p.ID)
	return err
}
