package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestReviewCommandResolvesVerifiedWorkByID(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	work := createCLIWorkBundle(t, stateDir, "review-work")
	var output bytes.Buffer
	if err := reviewCommand([]string{work.ID, "--preview"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "status: completed (verified)") || !strings.Contains(output.String(), "Preview: report.md") || !strings.Contains(output.String(), "# CLI report") {
		t.Fatalf("review output = %q", output.String())
	}
}

func TestResolveWorkBundleAcceptsManifestPathAndRejectsTampering(t *testing.T) {
	stateDir := t.TempDir()
	work := createCLIWorkBundle(t, stateDir, "path-work")
	bundle, found, err := resolveWorkBundle(work.ManifestPath, stateDir)
	if err != nil || !found || bundle.Manifest.RunID != work.ID {
		t.Fatalf("resolve bundle = %#v, found=%v, err=%v", bundle, found, err)
	}
	if err := os.WriteFile(filepath.Join(work.Output.Path(), "report.md"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := reviewCommand([]string{work.ManifestPath}, &output); err == nil {
		t.Fatal("tampered work bundle was reviewed")
	}
}

func TestTerminalSafeRemovesControlSequences(t *testing.T) {
	if got := terminalSafe("hello\x1b[31m\rworld\x00"); strings.ContainsAny(got, "\x1b\x00\r") || !strings.Contains(got, "world") {
		t.Fatalf("safe terminal text = %q", got)
	}
}

func TestExportCommandWritesVerifiedArchiveWithoutOverwriting(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	work := createCLIWorkBundle(t, stateDir, "export-work")
	destination := filepath.Join(t.TempDir(), "bundle.tar.gz")
	var output bytes.Buffer
	if err := exportPatch([]string{work.ID, "--to", destination}, &output); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	tarReader := tar.NewReader(gzipReader)
	header, err := tarReader.Next()
	if err != nil || header.Name != "manifest.json" {
		t.Fatalf("first archive entry = %#v, %v", header, err)
	}
	if _, err := io.Copy(io.Discard, tarReader); err != nil {
		t.Fatal(err)
	}
	if err := exportPatch([]string{work.ID, "--to", destination}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("overwrite error = %v", err)
	}
}

func TestApplyCommandPreflightsConflictsAndRequiresReplace(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	work := createCLIWorkBundle(t, stateDir, "apply-work")
	target := t.TempDir()
	targetFile := filepath.Join(target, "report.md")
	if err := os.WriteFile(targetFile, []byte("keep me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := applyPatch([]string{work.ID, "--to", target, "--check"}, &output)
	if err == nil || !strings.Contains(output.String(), "conflict report.md") {
		t.Fatalf("preflight output = %q, err=%v", output.String(), err)
	}
	contents, readErr := os.ReadFile(targetFile)
	if readErr != nil || string(contents) != "keep me\n" {
		t.Fatalf("check changed target = %q, %v", contents, readErr)
	}
	output.Reset()
	if err := applyPatch([]string{work.ID, "--to", target, "--replace"}, &output); err != nil {
		t.Fatal(err)
	}
	contents, readErr = os.ReadFile(targetFile)
	if readErr != nil || string(contents) != "# CLI report\n" {
		t.Fatalf("applied target = %q, %v", contents, readErr)
	}
}

func createCLIWorkBundle(t *testing.T, stateDir, id string) workspace.Work {
	t.Helper()
	source := t.TempDir()
	work, err := workspace.CreateWork(source, stateDir, id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	contract := artifact.DefaultContract("report.md")
	if err := artifact.WriteText(work.Output, contract, "report.md", "# CLI report\n"); err != nil {
		t.Fatal(err)
	}
	manifest, err := artifact.Seal(work.Output, contract, artifact.SealOptions{
		RunID: id, Objective: "Prepare a CLI report", Source: work.Source,
		StartedAt: work.CreatedAt, FinishedAt: work.CreatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := artifact.WriteManifest(work.ManifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	return work
}
