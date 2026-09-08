package artifact

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/gongahkia/gator/internal/workspace"
)

const maxManifestBytes = 8 * 1024 * 1024

// Bundle is a portable run directory containing manifest.json and output/.
// Opening a bundle does not require the original source directory to remain
// available; source identity in the manifest is provenance, not authority.
type Bundle struct {
	Path     string
	Output   workspace.Root
	Manifest Manifest
}

// OpenBundle strictly loads a retained or exported artifact bundle. Call
// VerifyBundle before presenting, exporting, or applying its contents.
func OpenBundle(path string) (Bundle, error) {
	if filepath.Base(path) == "manifest.json" {
		path = filepath.Dir(path)
	}
	root, err := workspace.Open(path)
	if err != nil {
		return Bundle{}, fmt.Errorf("open artifact bundle: %w", err)
	}
	contents, err := root.ReadRegularFile("manifest.json", maxManifestBytes)
	if err != nil {
		return Bundle{}, fmt.Errorf("read artifact manifest: %w", err)
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Bundle{}, fmt.Errorf("decode artifact manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return Bundle{}, errors.New("artifact manifest contains more than one value")
		}
		return Bundle{}, fmt.Errorf("decode artifact manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Bundle{}, err
	}
	outputPath, err := root.ResolveFile("output")
	if err != nil {
		return Bundle{}, fmt.Errorf("resolve artifact output: %w", err)
	}
	output, err := workspace.Open(outputPath)
	if err != nil {
		return Bundle{}, fmt.Errorf("open artifact output: %w", err)
	}
	return Bundle{Path: root.Path(), Output: output, Manifest: manifest}, nil
}

// VerifyBundle re-runs the embedded contract and requires every observed
// digest and validation result to match the sealed evidence exactly.
func VerifyBundle(bundle Bundle) error {
	if err := bundle.Manifest.Validate(); err != nil {
		return err
	}
	inspection, err := Inspect(bundle.Output, bundle.Manifest.Contract)
	if err != nil {
		return fmt.Errorf("inspect artifact bundle: %w", err)
	}
	if !equalFiles(inspection.Files, bundle.Manifest.Artifacts) {
		return errors.New("artifact files do not match the sealed manifest")
	}
	if !equalValidations(inspection.Validations, bundle.Manifest.Validations) {
		return errors.New("artifact validation evidence does not match the sealed manifest")
	}
	expected := Completed
	if !inspection.Passed || bundle.Manifest.Failure != "" {
		expected = Failed
	}
	if bundle.Manifest.Status != expected {
		return fmt.Errorf("artifact status %q does not match verified evidence", bundle.Manifest.Status)
	}
	return nil
}

func equalFiles(left, right []File) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalValidations(left, right []ValidationResult) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
