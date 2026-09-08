package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/workspace"
)

var runIDPattern = regexp.MustCompile(`\A[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}\z`)

// SealOptions supplies trusted run identity and action outcomes. Artifact
// evidence is always collected directly from outputRoot by Seal.
type SealOptions struct {
	RunID      string
	Objective  string
	Source     workspace.Root
	Actions    []action.Record
	StartedAt  time.Time
	FinishedAt time.Time
}

// Seal creates a manifest from fresh filesystem inspection. A failed
// validation produces a valid failed manifest rather than hiding the evidence
// behind an error.
func Seal(outputRoot workspace.Root, contract Contract, options SealOptions) (Manifest, error) {
	if !runIDPattern.MatchString(options.RunID) {
		return Manifest{}, fmt.Errorf("invalid work run id %q", options.RunID)
	}
	objective := strings.TrimSpace(options.Objective)
	if objective == "" || len(objective) > 64*1024 || strings.ContainsRune(objective, 0) {
		return Manifest{}, errors.New("work objective is missing or invalid")
	}
	if options.Source.Path() == "" {
		return Manifest{}, errors.New("work source is required")
	}
	if options.StartedAt.IsZero() || options.FinishedAt.IsZero() || options.FinishedAt.Before(options.StartedAt) {
		return Manifest{}, errors.New("work manifest timestamps are invalid")
	}
	for index, record := range options.Actions {
		if err := record.Validate(); err != nil {
			return Manifest{}, fmt.Errorf("action record %d: %w", index+1, err)
		}
	}
	switch contract.Normalize().ExternalActions {
	case action.Forbid:
		if len(options.Actions) > 0 {
			return Manifest{}, errors.New("artifact contract forbids external action proposals")
		}
	case action.Propose:
		for _, record := range options.Actions {
			if record.Status == action.Executed {
				return Manifest{}, errors.New("artifact contract permits external action drafts but not execution")
			}
		}
	}
	digest, err := contract.Digest()
	if err != nil {
		return Manifest{}, err
	}
	inspection, err := Inspect(outputRoot, contract)
	if err != nil {
		return Manifest{}, err
	}
	status := Completed
	if !inspection.Passed {
		status = Failed
	}
	sourceDigest := sha256.Sum256([]byte(options.Source.Path()))
	manifest := Manifest{
		Version:        ManifestVersion,
		RunID:          options.RunID,
		Workflow:       "work",
		Objective:      objective,
		ContractSHA256: digest,
		Source: Source{
			Kind:           "directory",
			Name:           filepath.Base(options.Source.Path()),
			IdentitySHA256: hex.EncodeToString(sourceDigest[:]),
		},
		Artifacts:   inspection.Files,
		Validations: inspection.Validations,
		Actions:     append([]action.Record(nil), options.Actions...),
		Status:      status,
		StartedAt:   options.StartedAt,
		FinishedAt:  options.FinishedAt,
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// WriteManifest atomically publishes a private manifest outside the output
// root. The caller owns the private run directory and supplies its exact path.
func WriteManifest(path string, manifest Manifest) error {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return errors.New("manifest path must be absolute")
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode artifact manifest: %w", err)
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".manifest-*")
	if err != nil {
		return fmt.Errorf("create artifact manifest: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set artifact manifest permissions: %w", err)
	}
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write artifact manifest: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync artifact manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close artifact manifest: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish artifact manifest: %w", err)
	}
	return nil
}
