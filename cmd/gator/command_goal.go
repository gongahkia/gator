package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/state"
)

const goalUsage = `Gator goals — ongoing responsibilities

  gator goal add [--source DIR] [--context TEXT] TITLE
  gator goal list
  gator goal show ID
  gator goal plan ID [outcome|add|step|remove|review-at|reviewed ...]
  gator goal context ID TEXT
  gator goal pause|resume|complete ID
  gator goal chat ID
  gator goal schedule ID --cron '0 9 * * *' [--timezone AREA/CITY] [--connector ID] [--mode inspect|draft] [--artifact PATH] --task TEXT
  gator goal watch ID [--task TEXT] [--artifact PATH]
  gator goal schedules ID
  gator goal unschedule ID JOB_ID

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
	case "plan":
		return goalPlanCommand(store, args[1:], out)
	case "chat":
		if len(args) != 2 {
			return errors.New("usage: gator goal chat ID")
		}
		return workInteractiveGoal(args[1])
	case "add":
		flags := flag.NewFlagSet("goal add", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		source := flags.String("source", "", "source directory (default: private empty goal workspace)")
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
	case "schedule":
		if len(args) < 2 {
			return errors.New("usage: gator goal schedule ID --cron CRON --task TEXT")
		}
		record, err := store.Load(args[1])
		if err != nil {
			return err
		}
		if record.Status != goal.Active {
			return errors.New("resume this goal before scheduling it")
		}
		settings, err := loadSettings()
		if err != nil {
			return err
		}
		flags := flag.NewFlagSet("goal schedule", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		cron := flags.String("cron", "", "five-field cron schedule")
		timezone := flags.String("timezone", settings.JobDefaults.Timezone, "IANA timezone")
		missed := flags.String("missed", "run_once", "skip or run_once")
		modeName := flags.String("mode", "inspect", "inspect or draft")
		artifactPath := flags.String("artifact", "", "required artifact for draft mode")
		task := flags.String("task", "", "recurring check instruction")
		changeOnly := flags.Bool("change-only", false, "skip a local-only check when files and saved context are unchanged")
		var connectors connectorFlags
		flags.Var(&connectors, "connector", "selected connected source for this recurring check")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		if len(flags.Args()) != 0 {
			return errors.New("unexpected goal schedule arguments; use --task TEXT")
		}
		if *cron == "" || strings.TrimSpace(*task) == "" {
			return errors.New("goal schedule requires --cron and --task")
		}
		if len(connectors) > 0 {
			registry, err := connector.NewRegistry(settings.Connectors)
			if err != nil {
				return err
			}
			for _, id := range connectors {
				descriptor, found := registry.Get(id)
				if !found {
					return fmt.Errorf("connector %q is not configured", id)
				}
				readable := false
				for _, operation := range descriptor.Operations() {
					if operation.Capability == action.ConnectedRead && connector.PermissionSet(settings.ConnectorPermissions).Resolve(id, operation.ID, operation.Capability) == connector.PermissionAllow {
						readable = true
						break
					}
				}
				if !readable {
					return fmt.Errorf("connector %q has no unattended read operation; review its scopes and permissions", id)
				}
			}
		}
		mode := action.Mode(*modeName)
		var contract artifact.Contract
		if mode == action.Inspect {
			if *artifactPath != "" {
				return errors.New("--artifact requires --mode draft")
			}
			contract = artifact.InspectionContract()
		} else if mode == action.Draft {
			if *artifactPath == "" {
				return errors.New("draft schedule requires --artifact PATH")
			}
			contract, err = workContract(mode, action.Forbid, artifactFlags{*artifactPath}, nil)
			if err != nil {
				return err
			}
		} else {
			return errors.New("scheduled goals may only inspect or draft")
		}
		jobStore, _, err := jobStore()
		if err != nil {
			return err
		}
		definition, err := jobStore.Save(jobs.Definition{Name: goalJobName(record), GoalID: record.ID, Enabled: true, Schedule: *cron, Timezone: *timezone, Missed: *missed, SourcePath: record.SourcePath, RefreshSnapshot: true, ChangeOnly: *changeOnly, Objective: strings.TrimSpace(*task), Mode: mode, Contract: contract, ConnectorIDs: connectors, MaxSteps: 24})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Scheduled %s for %s (%s %s). Run 'gator job supervisor' or install the per-user macOS/Linux service to execute it while Gator is closed.\n", record.ID, definition.ID, definition.Schedule, definition.Timezone)
		return err
	case "watch":
		if len(args) < 2 {
			return errors.New("usage: gator goal watch ID [--task TEXT] [--artifact PATH]")
		}
		record, err := store.Load(args[1])
		if err != nil {
			return err
		}
		if record.Status != goal.Active {
			return errors.New("resume this goal before watching it")
		}
		flags := flag.NewFlagSet("goal watch", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		task := flags.String("task", "Review changes and due steps, identify the next useful action, and propose any plan update for approval", "goal watch instruction")
		artifactPath := flags.String("artifact", "", "required draft on every model run, such as venue-reply.md")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		if len(flags.Args()) != 0 || strings.TrimSpace(*task) == "" {
			return errors.New("usage: gator goal watch ID [--task TEXT] [--artifact PATH]")
		}
		mode, contract := action.Inspect, artifact.InspectionContract()
		if *artifactPath != "" {
			mode = action.Draft
			contract, err = workContract(mode, action.Forbid, artifactFlags{*artifactPath}, nil)
			if err != nil {
				return err
			}
		}
		jobStore, _, err := jobStore()
		if err != nil {
			return err
		}
		definitions, err := jobStore.List()
		if err != nil {
			return err
		}
		for _, definition := range definitions {
			if definition.GoalID == record.ID && definition.GoalWatch && definition.Enabled {
				return fmt.Errorf("goal already has an active watch %s; edit or disable it before adding another", definition.ID)
			}
		}
		definition, err := jobStore.Save(jobs.Definition{Name: goalJobName(record), GoalID: record.ID, GoalWatch: true, Enabled: true, Schedule: "* * * * *", Timezone: "UTC", Missed: "run_once", SourcePath: record.SourcePath, RefreshSnapshot: true, ChangeOnly: true, Objective: strings.TrimSpace(*task), Mode: mode, Contract: contract, MaxSteps: 24})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Watching %s with %s. Start 'gator job supervisor' or install its per-user service; it checks about once a minute while running. Review follow-ups in gator inbox.\n", record.ID, definition.ID)
		return err
	case "schedules":
		if len(args) != 2 {
			return errors.New("usage: gator goal schedules ID")
		}
		if _, err := store.Load(args[1]); err != nil {
			return err
		}
		jobStore, _, err := jobStore()
		if err != nil {
			return err
		}
		definitions, err := jobStore.List()
		if err != nil {
			return err
		}
		for _, definition := range definitions {
			if definition.GoalID == args[1] {
				if _, err := fmt.Fprintf(out, "%s  %t  %s %s  %s\n", definition.ID, definition.Enabled, definition.Schedule, definition.Timezone, definition.Objective); err != nil {
					return err
				}
			}
		}
		return nil
	case "unschedule":
		if len(args) != 3 {
			return errors.New("usage: gator goal unschedule ID JOB_ID")
		}
		jobStore, _, err := jobStore()
		if err != nil {
			return err
		}
		definition, err := jobStore.Load(args[2])
		if err != nil {
			return err
		}
		if definition.GoalID != args[1] {
			return errors.New("job does not belong to this goal")
		}
		if err := jobStore.Remove(args[2]); err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Unscheduled %s.\n", args[2])
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

func goalJobName(record goal.Record) string {
	name := strings.Join(strings.Fields(record.Title), " ")
	for len(name) > 128 {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	return strings.TrimSpace(name)
}
