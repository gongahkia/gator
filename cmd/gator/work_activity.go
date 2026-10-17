package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/gongahkia/gator/internal/workrun"
)

func workActivityCommand(stateDir string, arguments []string, out io.Writer) error {
	if len(arguments) > 1 || len(arguments) == 1 && arguments[0] != "--json" {
		return errors.New("usage: gator work activity [--json]")
	}
	activity, err := workrun.ReadActivity(stateDir)
	if err != nil {
		return err
	}
	if len(arguments) == 1 {
		return json.NewEncoder(out).Encode(activity)
	}
	if len(activity.Runs) == 0 {
		_, err := fmt.Fprintln(out, "No running Work records.")
		return err
	}
	if _, err := fmt.Fprintln(out, "Current Work activity (stale records require inspection):"); err != nil {
		return err
	}
	for _, run := range activity.Runs {
		status := "running (unconfirmed)"
		switch run.LiveStatus {
		case "active":
			status = "running (active)"
		case "stopped":
			status = "running (stale)"
		}
		if _, err := fmt.Fprintf(out, "\n%s  %s  %s\n  started: %s\n", run.ID, status, strconv.QuoteToASCII(run.Objective), run.StartedAt.Format(time.RFC3339)); err != nil {
			return err
		}
		if len(run.Tasks) == 0 {
			if _, err := fmt.Fprintln(out, "  specialists: none recorded"); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintln(out, "  specialists:"); err != nil {
				return err
			}
			for _, task := range run.Tasks {
				checkpoint := ""
				if run.LiveStatus != "active" && (task.Status == "queued" || task.Status == "running") {
					checkpoint = " (checkpoint only)"
				}
				if _, err := fmt.Fprintf(out, "    %s %s%s %s: %s\n", task.ID, strconv.QuoteToASCII(task.Status), checkpoint, strconv.QuoteToASCII(task.Role), strconv.QuoteToASCII(task.Summary)); err != nil {
					return err
				}
			}
		}
		if len(run.PendingDecisions) == 0 {
			if _, err := fmt.Fprintln(out, "  pending decisions: none recorded"); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintln(out, "  pending decisions:"); err != nil {
				return err
			}
			for _, decision := range run.PendingDecisions {
				status := "recorded; check liveness before answering"
				if run.LiveStatus == "active" && decision.LiveStatus == "active" {
					status = "active"
				}
				if _, err := fmt.Fprintf(out, "    #%d %s (%s)\n", decision.ID, strconv.QuoteToASCII(decision.Kind), status); err != nil {
					return err
				}
			}
		}
		if run.LiveStatus == "active" {
			if _, err := fmt.Fprintf(out, "  controls: gator work steer %s TEXT | gator work stop %s\n", run.ID, run.ID); err != nil {
				return err
			}
			if len(run.Tasks) > 0 {
				if _, err := fmt.Fprintf(out, "            gator work cancel-task %s TASK_ID\n", run.ID); err != nil {
					return err
				}
			}
			if len(run.PendingDecisions) > 0 {
				if _, err := fmt.Fprintf(out, "            gator work answer %s DECISION_ID\n", run.ID); err != nil {
					return err
				}
			}
		} else if _, err := fmt.Fprintf(out, "  inspect: gator work show %s | gator work tasks %s\n", run.ID, run.ID); err != nil {
			return err
		}
	}
	return nil
}
