package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/model"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

func resumeTask(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("resume", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	maxSteps := flags.Int("max-steps", 0, "maximum model turns for this continuation")
	allowExternalCLI := flags.Bool("allow-external-cli", false, "allow a retained vendor CLI harness to run with its own permission model")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) < 2 {
		return errors.New("resume requires a run record path and a continuation task")
	}
	statePath := flags.Arg(0)
	continuation := strings.TrimSpace(strings.Join(flags.Args()[1:], " "))
	if continuation == "" {
		return errors.New("resume task is required")
	}
	session, err := journal.LoadSession(statePath)
	if err != nil {
		return err
	}
	provider, err := model.ParseProvider(session.Provider)
	if err != nil {
		return fmt.Errorf("load retained provider: %w", err)
	}
	modelName := model.EffectiveModel(provider, session.Model)
	if model.IsHarness(provider) && !*allowExternalCLI {
		return errors.New("resume of an external CLI harness requires --allow-external-cli")
	}
	executor, err := newExecutor(string(provider), modelName, session.BaseURL)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Gator resume\n  provider: %s\n  model: %s\n  task: %s\n", provider, displayModel(modelName), continuation); err != nil {
		return err
	}
	printer := eventPrinter{out: out}
	outcome, err := executor.Resume(context.Background(), session, statePath, continuation, gatorrun.Request{
		MaxSteps:         *maxSteps,
		OnEvent:          printer.Print,
		AllowExternalCLI: *allowExternalCLI,
	})
	if outcome.Worktree.Path != "" {
		if _, writeErr := fmt.Fprintf(out, "\nReview worktree: %s\n", outcome.Worktree.Path); writeErr != nil && err == nil {
			err = writeErr
		}
	}
	if outcome.StatePath != "" {
		if _, writeErr := fmt.Fprintf(out, "Run record: %s\n", outcome.StatePath); writeErr != nil && err == nil {
			err = writeErr
		}
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "\n%s\n", outcome.Result.FinalText)
	return err
}
