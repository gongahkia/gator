package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/patch"
)

func exportPatch(arguments []string, out io.Writer) error {
	if options, eligible, err := parseWorkExportOptions(arguments); err != nil {
		return err
	} else if eligible {
		stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
		if err != nil {
			return err
		}
		bundle, found, err := resolveWorkBundle(options.reference, stateDir)
		if err != nil {
			return err
		}
		if found {
			return exportWorkBundle(out, bundle, options.destination, options.replace)
		}
		if options.destination != "" || options.replace {
			return fmt.Errorf("work bundle %q was not found", options.reference)
		}
	}
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

type workExportOptions struct {
	reference   string
	destination string
	replace     bool
}

func parseWorkExportOptions(arguments []string) (workExportOptions, bool, error) {
	options := workExportOptions{}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == "--replace":
			options.replace = true
		case strings.HasPrefix(argument, "--to="):
			options.destination = strings.TrimSpace(strings.TrimPrefix(argument, "--to="))
		case argument == "--to":
			if index+1 >= len(arguments) {
				return workExportOptions{}, false, errors.New("--to requires a destination path")
			}
			index++
			options.destination = strings.TrimSpace(arguments[index])
		case strings.HasPrefix(argument, "-"):
			return workExportOptions{}, false, nil
		default:
			if options.reference != "" {
				return workExportOptions{}, false, nil
			}
			options.reference = argument
		}
	}
	if options.destination == "" && options.replace {
		return workExportOptions{}, false, errors.New("--replace requires --to")
	}
	return options, options.reference != "", nil
}

func exportWorkBundle(out io.Writer, bundle artifact.Bundle, destination string, replace bool) error {
	if destination == "" {
		return artifact.WriteArchive(out, bundle)
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve export destination: %w", err)
	}
	if info, err := os.Stat(absolute); err == nil && info.IsDir() {
		absolute = filepath.Join(absolute, "gator-work-"+bundle.Manifest.RunID+".tar.gz")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect export destination: %w", err)
	}
	if pathInside(bundle.Path, absolute) {
		return errors.New("export destination must not be inside the retained work bundle")
	}
	if info, err := os.Lstat(absolute); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("export destination exists and is not a regular file")
		}
		if !replace {
			return errors.New("export destination already exists; pass --replace to overwrite it")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect export destination: %w", err)
	}
	parent := filepath.Dir(absolute)
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("not a directory")
		}
		return fmt.Errorf("open export destination directory: %w", err)
	}
	temporary, err := os.CreateTemp(parent, ".gator-work-export-*")
	if err != nil {
		return fmt.Errorf("create temporary work archive: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := artifact.WriteArchive(temporary, bundle); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync work archive: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close work archive: %w", err)
	}
	if err := os.Rename(temporaryPath, absolute); err != nil {
		return fmt.Errorf("publish work archive: %w", err)
	}
	_, err = fmt.Fprintf(out, "Exported verified work bundle to %s\n", absolute)
	return err
}

func pathInside(parent, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(candidate))
	return err == nil && (relative == "." || relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func applyPatch(arguments []string, out io.Writer) error {
	if options, eligible, err := parseWorkApplyOptions(arguments); err != nil {
		return err
	} else if eligible {
		stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
		if err != nil {
			return err
		}
		bundle, found, err := resolveWorkBundle(options.reference, stateDir)
		if err != nil {
			return err
		}
		if found {
			if options.destination == "" {
				return errors.New("applying a work bundle requires an explicit --to DIRECTORY")
			}
			return applyWorkBundle(out, bundle, options)
		}
		if options.destination != "" || options.replace || options.json {
			return fmt.Errorf("work bundle %q was not found", options.reference)
		}
	}
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

type workApplyOptions struct {
	reference   string
	destination string
	replace     bool
	check       bool
	json        bool
}

func parseWorkApplyOptions(arguments []string) (workApplyOptions, bool, error) {
	options := workApplyOptions{}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == "--replace":
			options.replace = true
		case argument == "--check":
			options.check = true
		case argument == "--json":
			options.json = true
		case strings.HasPrefix(argument, "--to="):
			options.destination = strings.TrimSpace(strings.TrimPrefix(argument, "--to="))
		case argument == "--to":
			if index+1 >= len(arguments) {
				return workApplyOptions{}, false, errors.New("--to requires a target directory")
			}
			index++
			options.destination = strings.TrimSpace(arguments[index])
		case strings.HasPrefix(argument, "-"):
			return workApplyOptions{}, false, nil
		default:
			if options.reference != "" {
				return workApplyOptions{}, false, nil
			}
			options.reference = argument
		}
	}
	if options.destination == "" && (options.replace || options.json) {
		return workApplyOptions{}, false, errors.New("--replace and --json require --to DIRECTORY for work apply")
	}
	return options, options.reference != "", nil
}

func applyWorkBundle(out io.Writer, bundle artifact.Bundle, options workApplyOptions) error {
	var plan artifact.ApplyPlan
	var applyErr error
	if options.check {
		plan, applyErr = artifact.PlanApply(bundle, options.destination, options.replace)
		if applyErr == nil {
			for _, operation := range plan.Operations {
				if operation.Disposition == artifact.ApplyConflict {
					applyErr = errors.New("apply preflight found conflicts; pass --replace only after reviewing them")
					break
				}
			}
		}
	} else {
		plan, applyErr = artifact.Apply(bundle, options.destination, options.replace)
	}
	if options.json {
		type response struct {
			Applied bool               `json:"applied"`
			Plan    artifact.ApplyPlan `json:"plan"`
			Error   string             `json:"error,omitempty"`
		}
		result := response{Applied: applyErr == nil && !options.check, Plan: plan}
		if applyErr != nil {
			result.Error = applyErr.Error()
		}
		if err := json.NewEncoder(out).Encode(result); err != nil {
			return err
		}
		return applyErr
	}
	heading := "Apply preflight"
	if !options.check && applyErr != nil {
		heading = "Apply blocked"
	} else if !options.check {
		heading = "Applied verified work bundle"
	}
	if _, err := fmt.Fprintf(out, "%s\n  target: %s\n", heading, plan.Target); err != nil {
		return err
	}
	for _, operation := range plan.Operations {
		if _, err := fmt.Fprintf(out, "  %s %s\n", operation.Disposition, operation.Path); err != nil {
			return err
		}
	}
	return applyErr
}
