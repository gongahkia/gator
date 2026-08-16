package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/patch"
)

func exportPatch(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("export", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) != 1 {
		return errors.New("export requires one run record path")
	}
	session, err := journal.LoadSession(flags.Arg(0))
	if err != nil {
		return err
	}
	exported, err := patch.Export(context.Background(), session.WorktreePath, session.BaseCommit)
	if err != nil {
		return err
	}
	_, err = out.Write(exported)
	return err
}

func applyPatch(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("apply", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	checkOnly := flags.Bool("check", false, "verify that the patch applies without modifying this checkout")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) != 1 {
		return errors.New("apply requires one run record path")
	}
	session, err := journal.LoadSession(flags.Arg(0))
	if err != nil {
		return err
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	target, err := gitRepositoryRoot(workingDirectory)
	if err != nil {
		return errors.New("apply must start inside the target Git checkout")
	}
	var result patch.Result
	if *checkOnly {
		result, err = patch.Check(context.Background(), session.WorktreePath, session.BaseCommit, target)
	} else {
		result, err = patch.Apply(context.Background(), session.WorktreePath, session.BaseCommit, target)
	}
	if err != nil {
		return err
	}
	if *checkOnly {
		_, err = fmt.Fprintf(out, "Patch is compatible with this clean checkout (%d bytes).\n", result.Bytes)
	} else {
		_, err = fmt.Fprintf(out, "Applied retained patch to this checkout (%d bytes). Review and commit the resulting changes.\n", result.Bytes)
	}
	return err
}
