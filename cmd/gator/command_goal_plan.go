package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/goal"
)

const goalPlanUsage = `usage:
  gator goal plan ID
  gator goal plan ID outcome TEXT
  gator goal plan ID add --title TEXT [--due RFC3339] [--start RFC3339 --end RFC3339] [--depends-on STEP_ID] [--evidence REF]
  gator goal plan ID step STEP_ID pending|doing|blocked|done [--due RFC3339 | --clear-due] [--start RFC3339 --end RFC3339 | --clear-schedule] [--depends-on STEP_ID | --clear-dependencies] [--evidence REF | --clear-evidence]
  gator goal plan ID remove STEP_ID --yes
  gator goal plan ID review-at RFC3339|clear
  gator goal plan ID occurrence STEP_ID YYYY-MM-DD pending|doing|blocked|done|skipped|clear [--start RFC3339 --end RFC3339]
  gator goal plan ID reviewed`

type evidenceValues []string

func (v *evidenceValues) String() string { return strings.Join(*v, ", ") }
func (v *evidenceValues) Set(value string) error {
	*v = append(*v, strings.TrimSpace(value))
	return nil
}

func goalPlanCommand(store goal.Store, args []string, out io.Writer) error {
	if len(args) < 1 {
		return errors.New(goalPlanUsage)
	}
	id := args[0]
	plan, err := store.LoadPlan(id)
	if err != nil {
		return err
	}
	if len(args) == 1 {
		if _, err := fmt.Fprintf(out, "Goal plan %s (revision %d)\nOutcome: %s\nLast reviewed: %s\nNext review: %s\n", id, plan.Revision, valueOrDash(plan.Outcome), formatPlanTime(plan.LastReviewAt), formatPlanTime(plan.NextReviewAt)); err != nil {
			return err
		}
		for _, step := range plan.Steps {
			if step.Recurrence != nil {
				fmt.Fprintf(out, "Repeats: %s every %d · %s · until %s\n", step.Recurrence.Frequency, step.Recurrence.Interval, step.Recurrence.Timezone, formatPlanTime(step.Recurrence.Until))
			}
			for _, o := range step.Occurrences {
				fmt.Fprintf(out, "  occurrence %s: %s · %s to %s\n", o.Date, o.Status, formatPlanTime(o.StartsAt), formatPlanTime(o.EndsAt))
			}
			if _, err := fmt.Fprintf(out, "%s  %-7s  %s  due %s\n", step.ID, step.Status, step.Title, formatPlanTime(step.DueAt)); err != nil {
				return err
			}
			if !step.StartsAt.IsZero() {
				fmt.Fprintf(out, "  scheduled: %s to %s\n", formatPlanTime(step.StartsAt), formatPlanTime(step.EndsAt))
			}
			if len(step.DependsOn) > 0 {
				fmt.Fprintf(out, "  depends on: %s\n", strings.Join(step.DependsOn, ", "))
			}
			for _, evidence := range step.Evidence {
				if _, err := fmt.Fprintf(out, "  evidence: %s\n", evidence); err != nil {
					return err
				}
			}
		}
		return nil
	}
	change := goal.PlanChange{ExpectedRevision: plan.Revision}
	switch args[1] {
	case "outcome":
		if len(args) < 3 {
			return errors.New(goalPlanUsage)
		}
		change.Kind, change.Outcome = "set_outcome", strings.Join(args[2:], " ")
	case "add":
		flags := flag.NewFlagSet("goal plan add", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		title := flags.String("title", "", "step title")
		due := flags.String("due", "", "RFC3339 deadline")
		schedule := planScheduleFlags(flags)
		var evidence evidenceValues
		flags.Var(&evidence, "evidence", "source or confirmation reference")
		if err := flags.Parse(args[2:]); err != nil || len(flags.Args()) != 0 {
			return errors.New(goalPlanUsage)
		}
		change.Kind, change.Title, change.Evidence = "add_step", *title, evidence
		if err := schedule(&change); err != nil {
			return err
		}
		change.DueAt, err = parsePlanTime(*due)
		if err != nil {
			return err
		}
	case "step":
		if len(args) < 4 {
			return errors.New(goalPlanUsage)
		}
		flags := flag.NewFlagSet("goal plan step", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		due := flags.String("due", "", "RFC3339 deadline")
		schedule := planScheduleFlags(flags)
		clearDue := flags.Bool("clear-due", false, "remove deadline")
		clearEvidence := flags.Bool("clear-evidence", false, "remove evidence references")
		var evidence evidenceValues
		flags.Var(&evidence, "evidence", "source or confirmation reference")
		if err := flags.Parse(args[4:]); err != nil || len(flags.Args()) != 0 {
			return errors.New(goalPlanUsage)
		}
		change.Kind, change.StepID, change.Status = "update_step", args[2], goal.StepStatus(args[3])
		change.ClearDue, change.ClearEvidence = *clearDue, *clearEvidence
		change.Evidence = evidence
		if len(evidence) == 0 {
			change.Evidence = nil
		}
		if err := schedule(&change); err != nil {
			return err
		}
		change.DueAt, err = parsePlanTime(*due)
		if err != nil {
			return err
		}

	case "occurrence":
		if len(args) < 5 {
			return errors.New(goalPlanUsage)
		}
		flags := flag.NewFlagSet("goal plan occurrence", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		start := flags.String("start", "", "rescheduled start")
		end := flags.String("end", "", "rescheduled end")
		if err := flags.Parse(args[5:]); err != nil || len(flags.Args()) != 0 {
			return errors.New(goalPlanUsage)
		}
		o := &goal.OccurrenceOverride{Date: args[3], Status: args[4]}
		o.StartsAt, err = parsePlanTime(*start)
		if err != nil {
			return err
		}
		o.EndsAt, err = parsePlanTime(*end)
		if err != nil {
			return err
		}
		change.Kind, change.StepID, change.Occurrence = "update_occurrence", args[2], o
		if args[4] == "clear" {
			change.Kind = "clear_occurrence"
		}
	case "remove":
		if len(args) != 4 || args[3] != "--yes" {
			return errors.New(goalPlanUsage)
		}
		change.Kind, change.StepID = "remove_step", args[2]
	case "review-at":
		if len(args) != 3 {
			return errors.New(goalPlanUsage)
		}
		if args[2] == "clear" {
			change.Kind = "clear_next_review"
			break
		}
		change.Kind = "set_next_review"
		change.NextReviewAt, err = parsePlanTime(args[2])
		if err != nil {
			return err
		}
	case "reviewed":
		if len(args) != 2 {
			return errors.New(goalPlanUsage)
		}
		change.Kind = "mark_reviewed"
	default:
		return errors.New(goalPlanUsage)
	}
	updated, err := store.ApplyPlanChange(id, change)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Updated %s plan to revision %d.\n", id, updated.Revision)
	return err
}

func parsePlanTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("deadline must use RFC3339 with a timezone: %w", err)
	}
	return parsed.UTC(), nil
}
func formatPlanTime(value time.Time) string {
	if value.IsZero() {
		return "–"
	}
	return value.Format(time.RFC3339)
}

func planScheduleFlags(flags *flag.FlagSet) func(*goal.PlanChange) error {
	start := flags.String("start", "", "RFC3339 scheduled start")
	end := flags.String("end", "", "RFC3339 scheduled end")
	clear := flags.Bool("clear-schedule", false, "remove scheduled block")

	repeat := flags.String("repeat", "", "daily or weekly recurring block")
	interval := flags.Int("interval", 1, "recurrence interval, 1 through 52")
	timezone := flags.String("timezone", "", "IANA recurrence timezone")
	until := flags.String("until", "", "last permitted occurrence start, RFC3339")
	clearRepeat := flags.Bool("clear-repeat", false, "remove recurrence after clearing overrides")
	clearDeps := flags.Bool("clear-dependencies", false, "remove dependencies")
	var deps evidenceValues
	flags.Var(&deps, "depends-on", "step ID or GOAL_ID/STEP_ID; repeat for multiple dependencies")
	return func(change *goal.PlanChange) error {
		var err error
		change.StartsAt, err = parsePlanTime(*start)
		if err != nil {
			return err
		}
		change.EndsAt, err = parsePlanTime(*end)
		if err != nil {
			return err
		}

		change.ClearRecurrence = *clearRepeat
		if *repeat != "" {
			end, err := parsePlanTime(*until)
			if err != nil {
				return err
			}
			change.Recurrence = &goal.Recurrence{Frequency: *repeat, Interval: *interval, Timezone: *timezone, Until: end}
		} else if *timezone != "" || *until != "" || *interval != 1 {
			return errors.New("recurrence options require --repeat")
		}
		change.ClearSchedule, change.ClearDependencies, change.DependsOn = *clear, *clearDeps, deps
		return nil
	}
}

func goalAgendaCommand(store goal.Store, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("goal agenda", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	days := flags.Int("days", 7, "days ahead, 1 through 31")
	id := flags.String("goal", "", "restrict to one goal")
	if err := flags.Parse(args); err != nil || len(flags.Args()) != 0 || *days < 1 || *days > 31 {
		return errors.New("usage: gator goal agenda [--days 1..31] [--goal ID]")
	}
	now := time.Now()
	agenda, err := store.Agenda(*id, now, now.Add(time.Duration(*days)*24*time.Hour))
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "Local agenda · last reported progress")
	if len(agenda.Items) == 0 {
		_, err = fmt.Fprintln(out, "No unfinished steps in this window.")
		return err
	}
	for _, item := range agenda.Items {
		fmt.Fprintf(out, "%s · %s · %s\n", item.GoalTitle, item.Step.Title, item.Step.Status)
		if !item.Step.DueAt.IsZero() {
			label := "due"
			if item.Overdue {
				label = "overdue"
			}
			fmt.Fprintf(out, "  %s %s\n", label, item.Step.DueAt.Local().Format(time.RFC3339))
		}
		if item.OccurrenceDate != "" {
			fmt.Fprintf(out, "  occurrence %s\n", item.OccurrenceDate)
		}
		if item.Scheduled {
			fmt.Fprintf(out, "  scheduled %s to %s\n", item.Step.StartsAt.Local().Format(time.RFC3339), item.Step.EndsAt.Local().Format(time.RFC3339))
		}
		if len(item.BlockedBy) > 0 {
			fmt.Fprintf(out, "  waiting on %s\n", strings.Join(item.BlockedBy, ", "))
		}
		if len(item.Conflicts) > 0 {
			fmt.Fprintf(out, "  overlaps %s\n", strings.Join(item.Conflicts, ", "))
		}
	}
	if agenda.Truncated {
		fmt.Fprintf(out, "Showing %d of %d steps; select a goal for the full view. Overlaps cover shown steps only.\n", len(agenda.Items), agenda.Total)
	}
	return nil
}
