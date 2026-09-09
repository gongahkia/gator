package artifact

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyCreatesArtifactsInExplicitTarget(t *testing.T) {
	bundlePath, _ := sealedBundleFixture(t)
	bundle, err := OpenBundle(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	plan, err := Apply(bundle, target, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) != 1 || plan.Operations[0].Disposition != ApplyCreate {
		t.Fatalf("plan = %#v", plan)
	}
	contents, err := os.ReadFile(filepath.Join(target, "report.md"))
	if err != nil || string(contents) != "sealed report\n" {
		t.Fatalf("applied report = %q, %v", contents, err)
	}
	second, err := PlanApply(bundle, target, false)
	if err != nil || second.Operations[0].Disposition != ApplyUnchanged {
		t.Fatalf("second plan = %#v, %v", second, err)
	}
}

func TestApplyRefusesConflictsBeforeWriting(t *testing.T) {
	bundlePath, _ := sealedBundleFixture(t)
	bundle, err := OpenBundle(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	path := filepath.Join(target, "report.md")
	if err := os.WriteFile(path, []byte("developer version\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := Apply(bundle, target, false)
	if err == nil || len(plan.Operations) != 1 || plan.Operations[0].Disposition != ApplyConflict {
		t.Fatalf("conflict plan = %#v, %v", plan, err)
	}
	contents, readErr := os.ReadFile(path)
	if readErr != nil || string(contents) != "developer version\n" {
		t.Fatalf("conflicting target changed = %q, %v", contents, readErr)
	}
	plan, err = Apply(bundle, target, true)
	if err != nil || plan.Operations[0].Disposition != ApplyReplace {
		t.Fatalf("replacement plan = %#v, %v", plan, err)
	}
	contents, readErr = os.ReadFile(path)
	if readErr != nil || string(contents) != "sealed report\n" {
		t.Fatalf("replacement contents = %q, %v", contents, readErr)
	}
}

func TestPlanApplyRejectsSymlinkAndBundleOverlap(t *testing.T) {
	bundlePath, _ := sealedBundleFixture(t)
	bundle, err := OpenBundle(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PlanApply(bundle, filepath.Join(bundlePath, "output"), false); err == nil {
		t.Fatal("overlapping apply target was accepted")
	}
	target := t.TempDir()
	external := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(external, []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(target, "report.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanApply(bundle, target, true); err == nil {
		t.Fatal("symlink apply target was accepted")
	}
	contents, err := os.ReadFile(external)
	if err != nil || string(contents) != "outside\n" {
		t.Fatalf("external file changed = %q, %v", contents, err)
	}
}

func TestPlanApplyRejectsFailedBundle(t *testing.T) {
	bundlePath, _ := sealedBundleFixture(t)
	bundle, err := OpenBundle(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	bundle.Manifest.Status = Failed
	bundle.Manifest.Failure = "execution failed"
	if _, err := PlanApply(bundle, t.TempDir(), false); err == nil {
		t.Fatal("failed bundle was accepted")
	}
}
