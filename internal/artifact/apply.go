package artifact

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/workspace"
)

// ApplyDisposition describes one preflighted target operation.
type ApplyDisposition string

const (
	ApplyCreate    ApplyDisposition = "create"
	ApplyReplace   ApplyDisposition = "replace"
	ApplyUnchanged ApplyDisposition = "unchanged"
	ApplyConflict  ApplyDisposition = "conflict"
)

// ApplyOperation binds a reviewed artifact to the target state observed in
// preflight. ExistingSHA256 is rechecked immediately before a replacement.
type ApplyOperation struct {
	Path           string           `json:"path"`
	Disposition    ApplyDisposition `json:"disposition"`
	ExistingSHA256 string           `json:"existing_sha256,omitempty"`
}

// ApplyPlan is a side-effect-free preview for one explicit target directory.
type ApplyPlan struct {
	Target     string           `json:"target"`
	Operations []ApplyOperation `json:"operations"`
}

// PlanApply verifies a completed bundle and compares every artifact with an
// existing target. A differing regular file is a conflict unless replace is
// explicitly true; symlinks and special files always fail closed.
func PlanApply(bundle Bundle, targetPath string, replace bool) (ApplyPlan, error) {
	if err := VerifyBundle(bundle); err != nil {
		return ApplyPlan{}, fmt.Errorf("verify artifact bundle before apply: %w", err)
	}
	if bundle.Manifest.Status != Completed {
		return ApplyPlan{}, errors.New("only a completed work bundle can be applied")
	}
	target, err := workspace.Open(targetPath)
	if err != nil {
		return ApplyPlan{}, fmt.Errorf("open apply target: %w", err)
	}
	if pathsOverlap(bundle.Path, target.Path()) {
		return ApplyPlan{}, errors.New("apply target must not overlap the retained artifact bundle")
	}
	plan := ApplyPlan{Target: target.Path(), Operations: make([]ApplyOperation, 0, len(bundle.Manifest.Artifacts))}
	for _, file := range bundle.Manifest.Artifacts {
		operation, err := planTarget(target, file, replace)
		if err != nil {
			return ApplyPlan{}, err
		}
		plan.Operations = append(plan.Operations, operation)
	}
	return plan, nil
}

// Apply executes a fully preflighted plan. Known conflicts cause no writes.
// Every target is rechecked against preflight before its atomic file replace.
func Apply(bundle Bundle, targetPath string, replace bool) (ApplyPlan, error) {
	plan, err := PlanApply(bundle, targetPath, replace)
	if err != nil {
		return plan, err
	}
	var conflicts []string
	for _, operation := range plan.Operations {
		if operation.Disposition == ApplyConflict {
			conflicts = append(conflicts, operation.Path)
		}
	}
	if len(conflicts) > 0 {
		return plan, fmt.Errorf("apply has conflicting targets: %s; review them or pass replace explicitly", strings.Join(conflicts, ", "))
	}
	if err := VerifyBundle(bundle); err != nil {
		return plan, fmt.Errorf("reverify artifact bundle before apply: %w", err)
	}
	target, err := workspace.Open(plan.Target)
	if err != nil {
		return plan, fmt.Errorf("reopen apply target: %w", err)
	}
	for index, operation := range plan.Operations {
		if operation.Disposition == ApplyUnchanged {
			continue
		}
		if err := verifyTargetState(target, operation); err != nil {
			return plan, fmt.Errorf("target changed after preflight for %q: %w", operation.Path, err)
		}
		file := bundle.Manifest.Artifacts[index]
		contents, err := bundle.Output.ReadRegularFile(filepath.FromSlash(file.Path), bundle.Manifest.Contract.MaxArtifactBytes)
		if err != nil {
			return plan, fmt.Errorf("read artifact %q: %w", file.Path, err)
		}
		if err := target.WriteRegularFileAtomic(filepath.FromSlash(file.Path), contents, bundle.Manifest.Contract.MaxArtifactBytes); err != nil {
			return plan, fmt.Errorf("apply artifact %q: %w", file.Path, err)
		}
	}
	return plan, nil
}

func planTarget(target workspace.Root, file File, replace bool) (ApplyOperation, error) {
	operation := ApplyOperation{Path: file.Path, Disposition: ApplyCreate}
	root, err := os.OpenRoot(target.Path())
	if err != nil {
		return ApplyOperation{}, fmt.Errorf("open apply target: %w", err)
	}
	defer root.Close()
	relative := filepath.FromSlash(file.Path)
	info, err := root.Lstat(relative)
	if errors.Is(err, fs.ErrNotExist) {
		return operation, nil
	}
	if err != nil {
		return ApplyOperation{}, fmt.Errorf("inspect apply target %q: %w", file.Path, err)
	}
	if !info.Mode().IsRegular() {
		return ApplyOperation{}, fmt.Errorf("apply target %q is not a regular file", file.Path)
	}
	digest, err := target.SHA256RegularFile(relative, maximumTotalBytes)
	if err != nil {
		return ApplyOperation{}, fmt.Errorf("hash apply target %q: %w", file.Path, err)
	}
	operation.ExistingSHA256 = digest
	if digest == file.SHA256 {
		operation.Disposition = ApplyUnchanged
	} else if replace {
		operation.Disposition = ApplyReplace
	} else {
		operation.Disposition = ApplyConflict
	}
	return operation, nil
}

func verifyTargetState(target workspace.Root, operation ApplyOperation) error {
	relative := filepath.FromSlash(operation.Path)
	root, err := os.OpenRoot(target.Path())
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat(relative)
	switch operation.Disposition {
	case ApplyCreate:
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		return errors.New("target now exists")
	case ApplyReplace:
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("target is no longer a regular file")
		}
		digest, err := target.SHA256RegularFile(relative, maximumTotalBytes)
		if err != nil {
			return err
		}
		if digest != operation.ExistingSHA256 {
			return errors.New("target contents changed")
		}
		return nil
	default:
		return fmt.Errorf("unexpected apply disposition %q", operation.Disposition)
	}
}

func pathsOverlap(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	for _, pair := range [][2]string{{left, right}, {right, left}} {
		relative, err := filepath.Rel(pair[0], pair[1])
		if err == nil && (relative == "." || relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
			return true
		}
	}
	return false
}
