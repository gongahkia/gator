package artifact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/workspace"
)

func TestOpenAndVerifyBundleWithoutOriginalSource(t *testing.T) {
	bundlePath, sourcePath := sealedBundleFixture(t)
	if err := os.RemoveAll(sourcePath); err != nil {
		t.Fatal(err)
	}
	bundle, err := OpenBundle(filepath.Join(bundlePath, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyBundle(bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.Manifest.RunID != "work-bundle" || bundle.Output.Path() != filepath.Join(bundle.Path, "output") {
		t.Fatalf("bundle = %#v", bundle)
	}
}

func TestVerifyBundleRejectsChangedOrExtraFiles(t *testing.T) {
	for _, mutate := range []func(string) error{
		func(bundlePath string) error {
			return os.WriteFile(filepath.Join(bundlePath, "output", "report.md"), []byte("changed\n"), 0o600)
		},
		func(bundlePath string) error {
			return os.WriteFile(filepath.Join(bundlePath, "output", "extra.txt"), []byte("extra\n"), 0o600)
		},
	} {
		bundlePath, _ := sealedBundleFixture(t)
		bundle, err := OpenBundle(bundlePath)
		if err != nil {
			t.Fatal(err)
		}
		if err := mutate(bundlePath); err != nil {
			t.Fatal(err)
		}
		if err := VerifyBundle(bundle); err == nil || !strings.Contains(err.Error(), "do not match") {
			t.Fatalf("verification error = %v", err)
		}
	}
}

func TestOpenBundleRejectsUnknownManifestFields(t *testing.T) {
	bundlePath, _ := sealedBundleFixture(t)
	manifestPath := filepath.Join(bundlePath, "manifest.json")
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	contents = []byte(strings.Replace(string(contents), `"version": 1`, `"version": 1, "unexpected": true`, 1))
	if err := os.WriteFile(manifestPath, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenBundle(bundlePath); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("open error = %v", err)
	}
}

func sealedBundleFixture(t *testing.T) (string, string) {
	t.Helper()
	bundlePath := t.TempDir()
	if err := os.Mkdir(filepath.Join(bundlePath, "output"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeArtifact(t, filepath.Join(bundlePath, "output"), "report.md", "sealed report\n")
	sourcePath := t.TempDir()
	source, err := workspace.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	output, err := workspace.Open(filepath.Join(bundlePath, "output"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	manifest, err := Seal(output, DefaultContract("report.md"), SealOptions{
		RunID: "work-bundle", Objective: "Build a bundle", Source: source,
		StartedAt: now, FinishedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(filepath.Join(bundlePath, "manifest.json"), manifest); err != nil {
		t.Fatal(err)
	}
	return bundlePath, sourcePath
}
