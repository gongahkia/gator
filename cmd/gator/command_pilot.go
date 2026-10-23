package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/pilot"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/uisnapshot"
)

const pilotGuide = `Project follow-up pilot

Start with /followup new: watch selected project files and optional planning mail/calendar, identify the next useful step, and prepare a response for review.
Review the outcome, sources, notification conditions, frequency, and permitted actions before saving. Open /memory to inspect storage, retrieval, and chat processing.

For each participant, observe setup without coaching; one material change; one unrelated update; a correction; and stopping/resuming work.
Record first route chosen, time to action, wrong turns, and success without help. Review quiet checks against the source truth.

gator pilot record --work WORK_ID --draft usable|light-edits|rewrite|failed --corrections N [--followup useful|unhelpful] [--recovery succeeded|failed] [--reviewer PSEUDONYM] [--note TEXT]
gator pilot report [--goal GOAL_ID] [--json]

Optional observed usability: --setup-seconds N --resume-seconds N --repair-seconds N --unassisted true|false.
Omitting these flags leaves those observations unmeasured.

Rate shown alerts with gator job inbox rate ENTRY_ID useful|noise|wrong.
Review quiet checks with gator job quiet-audit JOB_ID ATTEMPT_ID missed|correct.
Unreviewed outcomes, failed provider calls, and scripted fixtures are not evidence of user success.
See docs/PROJECT_FOLLOWUP_PILOT.md and examples/project-followup for the replay and review procedure.`

type pilotReport struct {
	pilot.Report
	Alerts               inbox.RatingSummary `json:"alerts"`
	QuietReviewedCorrect int                 `json:"quiet_reviewed_correct"`
	QuietReviewedMissed  int                 `json:"quiet_reviewed_missed"`
	QuietUnreviewed      int                 `json:"quiet_unreviewed"`
}

func pilotCommand(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "guide" {
		_, err := fmt.Fprintln(out, pilotGuide)
		return err
	}
	stateDir, err := state.ResolveDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	store, err := pilot.Open(stateDir)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("pilot", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if args[0] == "record" {
		work := flags.String("work", "", "retained Work ID")
		draft := flags.String("draft", "", "human draft judgment")
		corrections := flags.Int("corrections", -1, "observed corrections required")
		followup := flags.String("followup", "unreviewed", "follow-up judgment")
		recovery := flags.String("recovery", "unreviewed", "recovery judgment")
		reviewer := flags.String("reviewer", "self", "reviewer pseudonym")
		note := flags.String("note", "", "source-supported reviewer notes")
		setupSeconds := flags.Int("setup-seconds", -1, "observed setup time, or omit when unmeasured")
		resumeSeconds := flags.Int("resume-seconds", -1, "observed resume time, or omit when unmeasured")
		repairSeconds := flags.Int("repair-seconds", -1, "observed manual repair time, or omit when unmeasured")
		unassisted := flags.String("unassisted", "unreviewed", "true or false when observed")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if len(flags.Args()) != 0 {
			return errors.New("unexpected pilot review arguments")
		}
		var alone *bool
		if *unassisted != "unreviewed" {
			value, parseErr := strconv.ParseBool(*unassisted)
			if parseErr != nil {
				return errors.New("--unassisted must be true, false, or unreviewed")
			}
			alone = &value
		}
		optionalSeconds := func(value int) *int {
			if value < 0 {
				return nil
			}
			return &value
		}
		review, err := store.Save(pilot.Review{WorkID: *work, Reviewer: *reviewer, Draft: *draft, Corrections: *corrections, Followup: *followup, Recovery: *recovery, Notes: *note, SetupSeconds: optionalSeconds(*setupSeconds), ResumeSeconds: optionalSeconds(*resumeSeconds), RepairSeconds: optionalSeconds(*repairSeconds), Unassisted: alone})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Saved human review for %s (%s); %s, %d corrections.\n", review.WorkID, review.Reviewer, review.Draft, review.Corrections)
		return err
	}
	if args[0] != "report" {
		return errors.New("usage: gator pilot guide|record|report")
	}
	goalID := flags.String("goal", "", "filter one goal")
	asJSON := flags.Bool("json", false, "machine-readable report")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("unexpected pilot report arguments")
	}
	report, err := readPilotReport(stateDir, *goalID)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(report)
	}
	_, err = fmt.Fprintf(out, "Human reviews: %d across %d Work runs\nDrafts: %d usable, %d light edits, %d rewrite, %d failed; %d corrections\nFollow-ups: %d useful, %d unhelpful (others unreviewed)\nRecovery: %d succeeded, %d failed (others unreviewed)\nShown alerts: %d useful, %d noise, %d wrong, %d unrated\nHealth notices: %d (excluded from alert ratings)\nQuiet decisions: %d reviewed correct, %d reviewed missed, %d unreviewed\nObserved run time: %.1fs; %d reported model requests; %d reported tokens\n", len(report.Reviews), report.UniqueRuns, report.Usable, report.LightEdits, report.Rewrite, report.Failed, report.Corrections, report.UsefulFollowups, report.UnhelpfulFollowups, report.RecoverySuccesses, report.RecoveryFailures, report.Alerts.Useful, report.Alerts.Noise, report.Alerts.Wrong, report.Alerts.Unrated, report.Alerts.HealthNotices, report.QuietReviewedCorrect, report.QuietReviewedMissed, report.QuietUnreviewed, report.ElapsedSeconds, report.ModelRequests, report.Tokens)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(out, "Observed usability: setup %ds across %d observations; resume %ds across %d; repair %ds across %d\nCompleted without help: %d; needed help: %d (others unreviewed)\n", report.SetupSeconds, report.SetupObservations, report.ResumeSeconds, report.ResumeObservations, report.RepairSeconds, report.RepairObservations, report.UnassistedSuccesses, report.NeededHelp); err != nil {
		return err
	}
	if report.KnownCostUSD != nil {
		// Report only retained provider cost, never an invented estimate.
		_, err = fmt.Fprintf(out, "Reported cost: $%.4f\n", *report.KnownCostUSD)
	} else {
		_, err = fmt.Fprintln(out, "Cost unavailable; missing provider usage or pricing is not treated as zero.")
	}
	return err
}

func readPilotReport(stateDir, goalID string) (pilotReport, error) {
	store, err := pilot.Open(stateDir)
	if err != nil {
		return pilotReport{}, err
	}
	report, err := store.Report(goalID)
	if err != nil {
		return pilotReport{}, err
	}
	result := pilotReport{Report: report}
	goals := map[string]bool{}
	if goalID != "" {
		goals[goalID] = true
	}
	for _, r := range report.Reviews {
		if r.GoalID != "" {
			goals[r.GoalID] = true
		}
	}
	inboxStore, err := inbox.Open(stateDir)
	if err != nil {
		return result, err
	}
	for id := range goals {
		ratings, err := inboxStore.RatingsForGoal(id)
		if err != nil {
			return result, err
		}
		result.Alerts.HealthNotices += ratings.HealthNotices
		result.Alerts.Total += ratings.Total
		result.Alerts.Useful += ratings.Useful
		result.Alerts.Noise += ratings.Noise
		result.Alerts.Wrong += ratings.Wrong
		result.Alerts.Unrated += ratings.Unrated
	}
	jobStore, err := jobs.Open(stateDir)
	if err != nil {
		return result, err
	}
	definitions, err := jobStore.List()
	if err != nil {
		return result, err
	}
	for _, definition := range definitions {
		if !goals[definition.GoalID] || !definition.GoalWatch {
			continue
		}
		audit, err := jobStore.QuietWatchAudit(definition.ID, 0)
		if err != nil {
			return result, err
		}
		result.QuietReviewedCorrect += audit.Correct
		result.QuietReviewedMissed += audit.Missed
		result.QuietUnreviewed += audit.Unreviewed
	}
	return result, nil
}

func workPilotResult(workID string, args []string) (*uisnapshot.Result, error) {
	if len(args) > 0 && args[0] == "record" && !containsWord(args, "--work") && workID != "" {
		args = append(append([]string(nil), args...), "--work", workID)
	}
	var output strings.Builder
	if err := pilotCommand(args, &output); err != nil {
		return nil, err
	}
	return &uisnapshot.Result{Kind: "details", Title: "Project follow-up pilot", Summary: output.String()}, nil
}
