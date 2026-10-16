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
  gator goal plan ID add --title TEXT [--due RFC3339] [--evidence REF]
  gator goal plan ID step STEP_ID pending|doing|blocked|done [--due RFC3339 | --clear-due] [--evidence REF | --clear-evidence]
  gator goal plan ID remove STEP_ID --yes
  gator goal plan ID review-at RFC3339|clear
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
			if _, err := fmt.Fprintf(out, "%s  %-7s  %s  due %s\n", step.ID, step.Status, step.Title, formatPlanTime(step.DueAt)); err != nil {
				return err
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
		var evidence evidenceValues
		flags.Var(&evidence, "evidence", "source or confirmation reference")
		if err := flags.Parse(args[2:]); err != nil || len(flags.Args()) != 0 {
			return errors.New(goalPlanUsage)
		}
		change.Kind, change.Title, change.Evidence = "add_step", *title, evidence
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
		change.DueAt, err = parsePlanTime(*due)
		if err != nil {
			return err
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
