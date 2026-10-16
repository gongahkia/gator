package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/state"
)

const goalUsage = `Gator goals — ongoing responsibilities

  gator goal add [--source DIR] [--context TEXT] TITLE
  gator goal list
  gator goal show ID
  gator goal context ID TEXT
  gator goal pause|resume|complete ID

Use gator work --goal ID "what should we do next?" to continue a goal's
conversation. A goal remembers its own context and conversation across runs.
Its context does not grant tools, network access, or external-action approval.`

func goalCommand(args []string, out io.Writer) error {
	if len(args) == 0 || isHelpArgument(args[0]) {
		_, err := fmt.Fprintln(out, goalUsage)
		return err
	}
	stateDir, err := state.ResolveDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	store, err := goal.Open(stateDir)
	if err != nil {
		return err
	}
	switch args[0] {
	case "add":
		flags := flag.NewFlagSet("goal add", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		source := flags.String("source", ".", "source directory")
		context := flags.String("context", "", "background and next steps")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		title := strings.TrimSpace(strings.Join(flags.Args(), " "))
		record, err := store.Add(title, *context, *source)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "%s  %s\n", record.ID, record.Title)
		return err
	case "list":
		if len(args) != 1 {
			return errors.New("usage: gator goal list")
		}
		records, err := store.List()
		if err != nil {
			return err
		}
		for _, record := range records {
			if _, err := fmt.Fprintf(out, "%s  %-9s %s\n", record.ID, record.Status, record.Title); err != nil {
				return err
			}
		}
		return nil
	case "show":
		if len(args) != 2 {
			return errors.New("usage: gator goal show ID")
		}
		record, err := store.Load(args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Goal: %s\nTitle: %s\nStatus: %s\nSource: %s\nConversation: %s\nContext: %s\n", record.ID, record.Title, record.Status, record.SourcePath, valueOrDash(record.ConversationID), valueOrDash(record.Context))
		return err
	case "context":
		if len(args) < 3 {
			return errors.New("usage: gator goal context ID TEXT")
		}
		record, err := store.SetContext(args[1], strings.Join(args[2:], " "))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Updated %s\n", record.ID)
		return err
	case "pause", "resume", "complete":
		if len(args) != 2 {
			return fmt.Errorf("usage: gator goal %s ID", args[0])
		}
		status := map[string]goal.Status{"pause": goal.Paused, "resume": goal.Active, "complete": goal.Completed}[args[0]]
		record, err := store.SetStatus(args[1], status)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "%s  %s\n", record.ID, record.Status)
		return err
	default:
		return fmt.Errorf("unknown goal command %q; run 'gator goal --help'", args[0])
	}
}
