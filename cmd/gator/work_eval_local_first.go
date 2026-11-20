package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/gongahkia/gator/internal/eval"
	"github.com/gongahkia/gator/internal/workrun"
)

// This command intentionally has no provider factory or credential lookup.
// Cloud/specialist arms are reference decision probes, never paid execution.
func workLocalFirstEvalCommand(args []string, out io.Writer) error {
	if len(args) < 2 || (args[0] != "validate" && args[0] != "run") {
		return errors.New("usage: gator work eval local-first validate|run PROFILE [--architecture A|B|C|D --split development|held-out --report-dir NEW_DIR]")
	}
	profilePath, err := filepath.Abs(args[1])
	if err != nil {
		return err
	}
	p, err := eval.LoadLocalFirstProfile(profilePath)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("local-first", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	arm := flags.String("architecture", "A", "A local-only, B local with specialist proposals, C direct-cloud reference, D explicit cloud selection reference")
	split := flags.String("split", eval.WorkSplitDevelopment, "development or held-out; all is an explicit diagnostic")
	directory := flags.String("report-dir", "", "new private report directory")
	maxRequests := flags.Int("max-model-requests", p.MaxRequests, "total scripted request limit, at most profile cap")
	wall := flags.Int("timeout-seconds", p.WallSeconds, "total wall limit, at most profile cap")
	attempts := flags.Int("attempts", 1, "repeat trials, at most two")
	live := flags.Bool("live", false, "unsupported in this deterministic profile")
	if err := flags.Parse(args[2:]); err != nil {
		return errors.New("invalid local-first flags; no operation performed")
	}
	if *live || flags.NArg() != 0 || *maxRequests < 1 || *maxRequests > p.MaxRequests || *wall < 1 || *wall > p.WallSeconds || *attempts < 1 || *attempts > 2 {
		return errors.New("local-first is fixture-only and requires bounded requests, wall time and attempts")
	}
	common := ""
	for _, architecture := range []string{"A", "B", "C", "D"} {
		_, _, hash, err := eval.LoadLocalFirstArm(profilePath, p, architecture)
		if err != nil {
			return err
		}
		if common != "" && hash != common {
			return errors.New("architecture arms do not have identical tasks, source fixtures and budgets")
		}
		common = hash
	}
	path, dataset, _, err := eval.LoadLocalFirstArm(profilePath, p, *arm)
	if err != nil {
		return err
	}
	if args[0] == "validate" {
		_, err := fmt.Fprintln(out, eval.LocalFirstProfileSummary(p))
		return err
	}
	if *directory == "" {
		return errors.New("--report-dir is required")
	}
	absolute, err := filepath.Abs(*directory)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*wall)*time.Second)
	defer cancel()
	report, runErr := eval.RunWorkExperiment(ctx, path, dataset, eval.WorkEvalOptions{
		ID: "local-first-" + *arm, Harness: commit, Provider: "fixture", Model: "scripted-reference", ReportDir: absolute,
		Attempts: *attempts, MaxRequests: *maxRequests, Split: *split, Delegation: false,
	}, func(c eval.WorkCase, state string) (workrun.Service, error) {
		return scriptedWorkService(c, state), nil
	})
	if runErr != nil {
		return runErr
	}
	// Retain effective limits, not only the larger reviewed profile caps.
	p.MaxRequests = *maxRequests
	p.WallSeconds = *wall
	if err := eval.WriteLocalFirstSummary(absolute, common, *arm, p, report); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "%s: %d/%d fixture trials pass; split=%s; no live models or specialists invoked. Model quality NOT TESTED. Report: %s\n", report.ID, report.Passed, report.Total, report.Split, filepath.Join(absolute, "experiment.json")); err != nil {
		return err
	}
	if report.Passed != report.Total {
		return errors.New("local-first fixture evaluation has failing trials")
	}
	return nil
}
