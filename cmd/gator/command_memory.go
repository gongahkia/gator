package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/state"
)

const memoryUsage = `Gator memory — explicit, inspectable personal knowledge

  gator memory add [--kind fact|preference|decision|note] [--goal ID] [--source REF|--source-file PATH] [--expires YYYY-MM-DD] TEXT
  gator memory list [--goal ID] [--all]
  gator memory search [--goal ID] QUERY
  gator memory show ID
  gator memory correct ID [--source-file PATH] TEXT
  gator memory forget ID

Goal memories are visible only in that goal; global memories are shared.
The agent uses bounded, relevant active memories as context, never as authority.`

func memoryCommand(args []string, out io.Writer) error {
	if len(args) == 0 || isHelpArgument(args[0]) {
		_, err := fmt.Fprintln(out, memoryUsage)
		return err
	}
	stateDir, err := state.ResolveDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	store, err := memory.Open(stateDir)
	if err != nil {
		return err
	}
	switch args[0] {
	case "add":
		flags := flag.NewFlagSet("memory add", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		kind := flags.String("kind", "fact", "memory kind")
		goalID := flags.String("goal", "", "goal ID")
		source := flags.String("source", "user", "source reference")
		sourceFile := flags.String("source-file", "", "link to a bounded local file and detect changes")
		expires := flags.String("expires", "", "expiry date")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *goalID != "" {
			goals, err := goal.Open(stateDir)
			if err != nil {
				return err
			}
			if _, err := goals.Load(*goalID); err != nil {
				return err
			}
		}
		var expiry time.Time
		if *expires != "" {
			expiry, err = time.ParseInLocation("2006-01-02", *expires, time.Local)
			if err != nil {
				return errors.New("--expires must be YYYY-MM-DD")
			}
			expiry = expiry.AddDate(0, 0, 1)
			if !expiry.After(time.Now()) {
				return errors.New("expiry must be in the future")
			}
		}
		if *sourceFile != "" && *source != "user" {
			return errors.New("choose --source or --source-file, not both")
		}
		var record memory.Record
		if *sourceFile != "" {
			record, err = store.AddFromFile(*kind, strings.Join(flags.Args(), " "), *goalID, *sourceFile, expiry)
		} else {
			record, err = store.Add(*kind, strings.Join(flags.Args(), " "), *goalID, *source, expiry)
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "%s  %s\n", record.ID, record.Text)
		return err
	case "list":
		flags := flag.NewFlagSet("memory list", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		goalID := flags.String("goal", "", "goal ID")
		all := flags.Bool("all", false, "include expired")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if len(flags.Args()) != 0 {
			return errors.New("usage: gator memory list [--goal ID] [--all]")
		}
		records, err := store.List(*goalID, *all)
		if err != nil {
			return err
		}
		return printMemories(out, records)
	case "search":
		flags := flag.NewFlagSet("memory search", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		goalID := flags.String("goal", "", "goal ID")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		records, err := store.Search(strings.Join(flags.Args(), " "), *goalID, 30)
		if err != nil {
			return err
		}
		return printMemories(out, records)
	case "show":
		if len(args) != 2 {
			return errors.New("usage: gator memory show ID")
		}
		record, err := store.Load(args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "ID: %s\nKind: %s\nGoal: %s\nSource: %s\nSource status: %s\nExpires: %s\nCreated: %s\nUpdated: %s\nText: %s\n", record.ID, record.Kind, valueOrDash(record.GoalID), valueOrDash(record.Source), valueOrDash(record.SourceStatus), formatMemoryExpiry(record.ExpiresAt), record.CreatedAt.Format(time.RFC3339), record.UpdatedAt.Format(time.RFC3339), record.Text)
		return err
	case "correct":
		if len(args) < 3 {
			return errors.New("usage: gator memory correct ID [--source-file PATH] TEXT")
		}
		flags := flag.NewFlagSet("memory correct", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		sourceFile := flags.String("source-file", "", "relink to a reviewed local source file")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		if len(flags.Args()) == 0 {
			return errors.New("memory correction text is required")
		}
		previous, err := store.Load(args[1])
		if err != nil {
			return err
		}
		var record memory.Record
		if *sourceFile != "" {
			record, err = store.CorrectFromFile(args[1], strings.Join(flags.Args(), " "), *sourceFile)
		} else {
			record, err = store.Correct(args[1], strings.Join(flags.Args(), " "), previous.Source)
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Corrected %s.\n", record.ID)
		return err
	case "forget":
		if len(args) != 2 {
			return errors.New("usage: gator memory forget ID")
		}
		if err := store.Forget(args[1]); err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Forgot %s.\n", args[1])
		return err
	default:
		return fmt.Errorf("unknown memory command %q", args[0])
	}
}

func printMemories(out io.Writer, records []memory.Record) error {
	for _, record := range records {
		line := record.Text
		if record.SourceStatus == "changed" || record.SourceStatus == "unavailable" {
			line += " [source " + record.SourceStatus + "; excluded from automatic recall]"
		}
		if _, err := fmt.Fprintf(out, "%s  %-10s %-29s %s\n", record.ID, record.Kind, valueOrDash(record.GoalID), line); err != nil {
			return err
		}
	}
	return nil
}
func formatMemoryExpiry(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Format("2006-01-02")
}
