package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/model"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

// forkTask creates an independent branch from the selected retained turn. It
// deliberately uses the same target-selection language as resume, so users do
// not need to remember separate session identifiers or state-file paths.
func forkTask(arguments []string, out io.Writer) error {
	return branchTask("fork", arguments, out)
}

func cloneTask(arguments []string, out io.Writer) error {
	return branchTask("clone", arguments, out)
}

func branchTask(operation string, arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet(operation, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	last := flags.Bool("last", false, "fork the most recent retained thread for this repository")
	all := flags.Bool("all", false, "include retained threads from other repositories")
	maxSteps := flags.Int("max-steps", 0, "maximum model turns for this fork")
	compact := flags.Bool("compact", false, "summarize older retained context before branching")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	arguments = flags.Args()
	var target, instruction string
	if *last {
		instruction = strings.TrimSpace(strings.Join(arguments, " "))
	} else if len(arguments) > 0 {
		target = arguments[0]
		instruction = strings.TrimSpace(strings.Join(arguments[1:], " "))
	}
	if target == "" && !*last {
		return fmt.Errorf("%s requires a retained thread ID or --last", operation)
	}
	if target != "" && looksLikeRunRecordPath(target) {
		if *all {
			return errors.New("--all cannot be combined with a run record path")
		}
		return branchState(operation, target, instruction, *maxSteps, *compact, out)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	repository, repositoryErr := gitRepositoryRoot(workingDirectory)
	if repositoryErr != nil && !*all {
		return errors.New("fork by thread requires starting inside a Git checkout")
	}
	if repositoryErr != nil {
		repository = workingDirectory
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	selected, err := resolveResumeThread(stateDir, repository, target, *last, *all)
	if err != nil {
		return err
	}
	return branchState(operation, selected.HeadStatePath, instruction, *maxSteps, *compact, out)
}

func forkState(statePath, instruction string, maxSteps int, out io.Writer) error {
	return branchState("fork", statePath, instruction, maxSteps, false, out)
}

func branchState(operation, statePath, instruction string, maxSteps int, compact bool, out io.Writer) error {
	session, err := journal.LoadSession(statePath)
	if err != nil {
		return err
	}
	if _, err := journal.LoadSnapshot(statePath); err != nil {
		return fmt.Errorf("retained turn cannot be forked: %w", err)
	}
	if instruction == "" {
		return interactiveWithOptions(interactiveOptions{RepositoryPath: session.Repository, ForkStatePath: statePath})
	}
	provider, err := model.ParseProvider(session.Provider)
	if err != nil {
		return fmt.Errorf("load retained provider: %w", err)
	}
	modelName := model.EffectiveModel(provider, session.Model)
	executor, err := newExecutor(string(provider), modelName, session.BaseURL)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Gator %s\n  source: %s\n  provider: %s\n  model: %s\n  task: %s\n", operation, session.ThreadID, provider, displayModel(modelName), instruction); err != nil {
		return err
	}
	printer := eventPrinter{out: out}
	request := gatorrun.Request{MaxSteps: maxSteps, ForceCompaction: compact, OnEvent: printer.Print}
	var outcome gatorrun.Outcome
	if operation == "clone" {
		outcome, err = executor.Clone(context.Background(), session, statePath, instruction, request)
	} else {
		outcome, err = executor.Fork(context.Background(), session, statePath, instruction, request)
	}
	if outcome.Worktree.Path != "" {
		if _, writeErr := fmt.Fprintf(out, "\n%s worktree: %s\n", operation, outcome.Worktree.Path); writeErr != nil && err == nil {
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
