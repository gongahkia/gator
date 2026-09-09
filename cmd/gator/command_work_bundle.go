package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/gongahkia/gator/internal/artifact"
)

const maxWorkspaceBuckets = 4096

var workReferencePattern = regexp.MustCompile(`\A[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}\z`)

type workReviewOptions struct {
	reference string
	json      bool
	preview   bool
}

func parseWorkReviewOptions(arguments []string) (workReviewOptions, bool) {
	options := workReviewOptions{}
	for _, argument := range arguments {
		switch argument {
		case "--json":
			options.json = true
		case "--preview":
			options.preview = true
		default:
			if strings.HasPrefix(argument, "-") || options.reference != "" {
				return workReviewOptions{}, false
			}
			options.reference = argument
		}
	}
	return options, options.reference != ""
}

// resolveWorkBundle accepts an exact bundle/manifest path or a run ID from
// Gator's private workspace store. It reports found=false for a non-work path
// so legacy coding review can retain its established behavior.
func resolveWorkBundle(reference, stateDir string) (artifact.Bundle, bool, error) {
	if info, err := os.Stat(reference); err == nil {
		candidate := reference
		if !info.IsDir() && filepath.Base(reference) != "manifest.json" {
			return artifact.Bundle{}, false, nil
		}
		if filepath.Base(candidate) == "manifest.json" {
			candidate = filepath.Dir(candidate)
		}
		if _, err := os.Stat(filepath.Join(candidate, "manifest.json")); errors.Is(err, os.ErrNotExist) {
			return artifact.Bundle{}, false, nil
		} else if err != nil {
			return artifact.Bundle{}, true, fmt.Errorf("inspect work manifest: %w", err)
		}
		bundle, err := artifact.OpenBundle(candidate)
		return bundle, true, err
	} else if !errors.Is(err, os.ErrNotExist) {
		return artifact.Bundle{}, false, err
	}
	if !workReferencePattern.MatchString(reference) {
		return artifact.Bundle{}, false, nil
	}
	workspaceRoot := filepath.Join(stateDir, "gator", "workspaces")
	buckets, err := os.ReadDir(workspaceRoot)
	if errors.Is(err, os.ErrNotExist) {
		return artifact.Bundle{}, false, nil
	}
	if err != nil {
		return artifact.Bundle{}, false, fmt.Errorf("list retained workspaces: %w", err)
	}
	if len(buckets) > maxWorkspaceBuckets {
		return artifact.Bundle{}, false, errors.New("retained workspace index exceeds its safe scan limit")
	}
	var matches []string
	for _, bucket := range buckets {
		if !bucket.IsDir() || len(bucket.Name()) != 32 {
			continue
		}
		candidate := filepath.Join(workspaceRoot, bucket.Name(), reference)
		info, err := os.Lstat(filepath.Join(candidate, "manifest.json"))
		if err == nil && info.Mode().IsRegular() {
			matches = append(matches, candidate)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return artifact.Bundle{}, false, fmt.Errorf("inspect retained work %q: %w", reference, err)
		}
	}
	if len(matches) == 0 {
		return artifact.Bundle{}, false, nil
	}
	if len(matches) > 1 {
		return artifact.Bundle{}, true, fmt.Errorf("work ID %q is ambiguous across source folders; use its manifest path", reference)
	}
	bundle, err := artifact.OpenBundle(matches[0])
	return bundle, true, err
}

func reviewWorkBundle(out io.Writer, bundle artifact.Bundle, jsonOutput, includePreviews bool) error {
	if err := artifact.VerifyBundle(bundle); err != nil {
		return err
	}
	var previews []artifact.Preview
	var err error
	if includePreviews {
		previews, err = artifact.PreviewBundle(bundle)
		if err != nil {
			return err
		}
	}
	if jsonOutput {
		return json.NewEncoder(out).Encode(struct {
			Verified bool               `json:"verified"`
			Manifest artifact.Manifest  `json:"manifest"`
			Previews []artifact.Preview `json:"previews,omitempty"`
		}{Verified: true, Manifest: bundle.Manifest, Previews: previews})
	}
	manifest := bundle.Manifest
	if _, err := fmt.Fprintf(out, "Gator Work Review\n  run: %s\n  status: %s (verified)\n  objective: %s\n  source: %s (sha256:%s)\n  contract: sha256:%s\n", manifest.RunID, manifest.Status, terminalSafe(manifest.Objective), manifest.Source.Name, manifest.Source.IdentitySHA256[:12], manifest.ContractSHA256[:12]); err != nil {
		return err
	}
	if manifest.Failure != "" {
		if _, err := fmt.Fprintf(out, "  failure: %s\n", terminalSafe(manifest.Failure)); err != nil {
			return err
		}
	}
	if len(manifest.ConnectedSources) > 0 {
		if _, err := fmt.Fprintln(out, "\nConnected sources:"); err != nil {
			return err
		}
		for _, source := range manifest.ConnectedSources {
			if _, err := fmt.Fprintf(out, "  ✓ %s/%s  %d bytes  sha256:%s  %s\n", source.ConnectorID, source.Operation, source.Bytes, source.SHA256[:12], source.Resource); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(out, "\nArtifacts:"); err != nil {
		return err
	}
	if len(manifest.Artifacts) == 0 {
		if _, err := fmt.Fprintln(out, "  (analysis-only run; no artifacts)"); err != nil {
			return err
		}
	}
	for _, file := range manifest.Artifacts {
		if _, err := fmt.Fprintf(out, "  ✓ %s  %s  %d bytes  sha256:%s\n", file.Path, file.MediaType, file.Bytes, file.SHA256[:12]); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(out, "\nOutcome evidence:"); err != nil {
		return err
	}
	if len(manifest.Validations) == 0 {
		if _, err := fmt.Fprintln(out, "  (no artifact validations required)"); err != nil {
			return err
		}
	}
	for _, validation := range manifest.Validations {
		mark := "✓"
		if !validation.Passed {
			mark = "!"
		}
		diagnostic := ""
		if validation.Diagnostic != "" {
			diagnostic = " — " + terminalSafe(validation.Diagnostic)
		}
		if _, err := fmt.Fprintf(out, "  %s %s: %s%s\n", mark, validation.Path, validation.Kind, diagnostic); err != nil {
			return err
		}
	}
	if len(manifest.Actions) > 0 {
		if _, err := fmt.Fprintln(out, "\nExternal actions:"); err != nil {
			return err
		}
		for _, record := range manifest.Actions {
			if _, err := fmt.Fprintf(out, "  %s %s/%s → %s\n    payload sha256:%s\n    exact JSON (escaped): %s\n", record.Status, record.Proposal.ConnectorID, record.Proposal.Operation, terminalSafe(record.Proposal.Target), record.Proposal.PayloadSHA256, strconv.QuoteToASCII(record.Proposal.Preview)); err != nil {
				return err
			}
			if record.Error != "" {
				if _, err := fmt.Fprintf(out, "    error: %s\n", terminalSafe(record.Error)); err != nil {
					return err
				}
			}
		}
	}
	for _, preview := range previews {
		if _, err := fmt.Fprintf(out, "\nPreview: %s (%s)\n%s\n", preview.Path, terminalSafe(preview.Summary), preview.Content); err != nil {
			return err
		}
		if preview.Truncated {
			if _, err := fmt.Fprintln(out, "[… preview truncated …]"); err != nil {
				return err
			}
		}
	}
	return nil
}

func terminalSafe(value string) string {
	return strings.Map(func(character rune) rune {
		if character == '\r' {
			return '\n'
		}
		if unicode.IsControl(character) && character != '\n' && character != '\t' {
			return -1
		}
		return character
	}, value)
}
