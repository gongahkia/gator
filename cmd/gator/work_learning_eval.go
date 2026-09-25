package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/gongahkia/gator/internal/eval"
)

const learningEvalUsage = `usage:
  gator work eval learning validate DATASET
  gator work eval learning run DATASET --report-dir DIR [--id ID] [--split development|held-out|all]
  gator work eval learning show EXPERIMENT_JSON

The default split is development. Held-out learning cases require an explicit
--split held-out or --split all selection.`

func workLearningEvalCommand(args []string, out io.Writer) error {
	if len(args) < 2 || isHelpArgument(args[0]) {
		_, err := fmt.Fprintln(out, learningEvalUsage)
		return err
	}
	action, path := args[0], args[1]
	switch action {
	case "validate":
		if len(args) != 2 {
			return errors.New("usage: gator work eval learning validate DATASET")
		}
		dataset, err := eval.LoadLearningDataset(path)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "%s: %d valid learning cases\n", dataset.ID, len(dataset.Cases))
		return err
	case "show":
		if len(args) != 2 {
			return errors.New("usage: gator work eval learning show EXPERIMENT_JSON")
		}
		report, err := eval.LoadLearningExperiment(path)
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, eval.LearningSummary(report))
		return err
	case "run":
		return runLearningEval(path, args[2:], out)
	default:
		return fmt.Errorf("unknown learning evaluation operation %q\n%s", action, learningEvalUsage)
	}
}

func runLearningEval(path string, args []string, out io.Writer) error {
	dataset, err := eval.LoadLearningDataset(path)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("eval learning run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	reportDir := flags.String("report-dir", "", "new output directory")
	id := flags.String("id", "learning-eval", "experiment ID")
	split := flags.String("split", eval.WorkSplitDevelopment, "dataset split: development (default), held-out, or all")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *reportDir == "" {
		return errors.New("--report-dir is required")
	}
	absolute, err := filepath.Abs(*reportDir)
	if err != nil {
		return err
	}
	report, err := eval.RunLearningExperiment(context.Background(), path, dataset, eval.LearningEvalOptions{ID: *id, Harness: commit, ReportDir: absolute, Split: *split})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s: %d/%d learning probes pass; split=%s; diagnostics %v. Report: %s\n", report.ID, report.Passed, report.Total, report.Split, report.Categories, filepath.Join(absolute, "experiment.json"))
	if err != nil {
		return err
	}
	if report.Passed != report.Total {
		return errors.New("learning evaluation has failing probes")
	}
	return nil
}
