package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/delivery"
	"github.com/gongahkia/gator/internal/state"
)

func exportPatch(arguments []string, out io.Writer) error {
	options, eligible, err := parseWorkExportOptions(arguments)
	if err != nil {
		return err
	}
	if !eligible {
		return errors.New("usage: gator work export WORK_BUNDLE|WORK_ID [--to ARCHIVE] [--replace]")
	}
	stateDir, err := state.ResolveDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	bundle, found, err := resolveWorkBundle(options.reference, stateDir)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("work bundle %q was not found", options.reference)
	}
	return exportWorkBundle(out, bundle, options.destination, options.replace)
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
	options, eligible, err := parseWorkApplyOptions(arguments)
	if err != nil {
		return err
	}
	if !eligible {
		return errors.New("usage: gator work apply WORK_BUNDLE|WORK_ID --to DIRECTORY [--code-patch PATH] [--check] [--replace]")
	}
	if options.destination == "" {
		return errors.New("applying a work bundle requires an explicit --to DIRECTORY")
	}
	stateDir, err := state.ResolveDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	bundle, found, err := resolveWorkBundle(options.reference, stateDir)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("work bundle %q was not found", options.reference)
	}
	store, err := delivery.Open(stateDir)
	if err != nil {
		return err
	}
	return applyWorkBundle(out, bundle, store, options)
}

type workApplyOptions struct {
	codePatch   string
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
		case argument == "--code-patch":
			if index+1 >= len(arguments) {
				return workApplyOptions{}, false, errors.New("--code-patch requires one verified candidate patch path")
			}
			index++
			options.codePatch = arguments[index]
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

func applyWorkBundle(out io.Writer, bundle artifact.Bundle, store delivery.Store, options workApplyOptions) error {
	if options.codePatch != "" {
		candidateID := ""
		for _, candidate := range bundle.Manifest.Candidates {
			if candidate.PatchPath == options.codePatch && candidate.Status == "verified" {
				candidateID = candidate.ID
				break
			}
		}
		if candidateID == "" {
			return errors.New("select a verified code candidate retained by this bundle")
		}
		if options.check {
			candidate, err := delivery.PreviewCandidate(context.Background(), bundle, options.destination, candidateID)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(out, "Code candidate %s: preflight passed\nChanged paths: %s\n", candidate.ID, strings.Join(candidate.ChangedPaths, ", "))
			return err
		}
		record, err := store.DeliverCandidate(context.Background(), bundle, options.destination, candidateID)
		return writeDeliveryResult(out, record, err, options.json)
	}

	if options.check {
		plan, err := delivery.PreviewArtifacts(bundle, options.destination, options.replace)
		if err == nil {
			for _, operation := range plan.Operations {
				if operation.Disposition == artifact.ApplyConflict {
					err = errors.New("apply preflight found conflicts; pass --replace only after reviewing them")
					break
				}
			}
		}
		if options.json {
			type response struct {
				Applied bool               `json:"applied"`
				Plan    artifact.ApplyPlan `json:"plan"`
				Error   string             `json:"error,omitempty"`
			}
			result := response{Plan: plan}
			if err != nil {
				result.Error = err.Error()
			}
			if encodeErr := json.NewEncoder(out).Encode(result); encodeErr != nil {
				return encodeErr
			}
			return err
		}
		heading := "Apply preflight"
		if err != nil {
			heading = "Apply blocked"
		}
		if _, writeErr := fmt.Fprintf(out, "%s\n  target: %s\n", heading, plan.Target); writeErr != nil {
			return writeErr
		}
		for _, operation := range plan.Operations {
			if _, writeErr := fmt.Fprintf(out, "  %s %s\n", operation.Disposition, operation.Path); writeErr != nil {
				return writeErr
			}
		}
		return err
	}
	record, err := store.DeliverArtifacts(context.Background(), bundle, options.destination, options.replace)
	return writeDeliveryResult(out, record, err, options.json)
}

func writeDeliveryResult(out io.Writer, record delivery.Record, deliveryErr error, jsonOutput bool) error {
	if jsonOutput {
		result := struct {
			Delivery delivery.Record `json:"delivery"`
			Applied  bool            `json:"applied"`
			Error    string          `json:"error,omitempty"`
		}{Delivery: record, Applied: deliveryErr == nil}
		if deliveryErr != nil {
			result.Error = deliveryErr.Error()
		}
		if err := json.NewEncoder(out).Encode(result); err != nil {
			return err
		}
		return deliveryErr
	}
	heading := "Applied verified Work result"
	if deliveryErr != nil {
		heading = "Delivery incomplete"
	}
	if _, err := fmt.Fprintf(out, "%s\n  delivery: %s\n  target: %s\n", heading, record.ID, record.TargetPath); err != nil {
		return err
	}
	for _, effect := range record.Effects {
		if _, err := fmt.Fprintf(out, "  %s %s\n", effect.Status, effect.SourcePath); err != nil {
			return err
		}
	}
	return deliveryErr
}
