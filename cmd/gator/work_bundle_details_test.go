package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/workapp"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestWorkBundleDetailsPreviewIsVerifiedAndRetained(t *testing.T) {
	work := createCLIWorkBundle(t, t.TempDir(), "work-details")
	summary, err := summarizeWorkBundle(work.Path)
	if err != nil || summary.RunID != "work-details" || summary.FinishedAt == "" || len(summary.Artifacts) != 1 || summary.Artifacts[0].Preview != "# CLI report\n" || summary.Artifacts[0].PreviewKind != "markdown" {
		t.Fatalf("summary = %#v, %v", summary, err)
	}
	if err := os.WriteFile(filepath.Join(work.Path, "output", "report.md"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := summarizeWorkBundle(work.Path); err == nil {
		t.Fatal("tampered bytes were exposed as a verified preview")
	}
	if summary.Artifacts[0].Preview != "# CLI report\n" {
		t.Fatal("retained snapshot changed with the live file")
	}
}

func TestWorkBundleDetailsBoundsAndDeduplicatesProvenance(t *testing.T) {
	var bundle artifact.Bundle
	for index := 0; index < 40; index++ {
		bundle.Manifest.Evidence = append(bundle.Manifest.Evidence, artifact.Evidence{Locator: "recorded-source"})
		bundle.Manifest.ConnectedSources = append(bundle.Manifest.ConnectedSources, connector.Provenance{ConnectorID: "drive", Operation: "fetch"})
	}
	var summary workapp.BundleSummary
	projectWorkBundleDetails(bundle, &summary)
	if len(summary.Sources) != 16 || len(summary.Connections) != 1 || len(summary.Connections[0].Operations) != 1 {
		t.Fatalf("provenance projection = %#v", summary)
	}
	text := strings.Repeat("界", 2000)
	preview, truncated := boundedDetailPreview(text, 4096)
	if !truncated || !utf8.ValidString(preview) || len(preview) > 4096 || len(preview) < 4093 {
		t.Fatalf("UTF-8 preview bounds = %d, %t", len(preview), truncated)
	}
}

func TestWorkBundleDetailsBoundsPreviewReadsAndMatchesSelectedPaths(t *testing.T) {
	work, err := workspace.CreateWork(t.TempDir(), t.TempDir(), "work-bounded-previews", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"large.txt", "one.md", "two.md", "three.md", "four.md", "five.md", "six.md", "seven.md", "eight.md", "nine.md"}
	contract := artifact.DefaultContract(paths...)
	for _, path := range paths {
		content := strings.Repeat("界", 2000)
		if path == "large.txt" {
			content = strings.Repeat("x", 1024*1024+1)
		}
		if err := artifact.WriteText(work.Output, contract, path, content); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := artifact.Seal(work.Output, contract, artifact.SealOptions{RunID: "work-bounded-previews", Objective: "Review retained outputs", Source: work.Source, StartedAt: work.CreatedAt, FinishedAt: work.CreatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if err := artifact.WriteManifest(work.ManifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	summary, err := summarizeWorkBundle(work.Path)
	if err != nil {
		t.Fatal(err)
	}
	total, previews := 0, 0
	for _, file := range summary.Artifacts {
		if file.Path == "large.txt" && (file.Preview != "" || file.Bytes != 1024*1024+1 || !file.Valid) {
			t.Fatal("large file metadata or preview read budget was lost")
		}
		if file.Preview != "" {
			previews++
			if !file.Truncated || !utf8.ValidString(file.Preview) || file.Preview != strings.Repeat("界", 1365) {
				t.Fatalf("selected preview was assigned to the wrong path or truncated incorrectly: %#v", file)
			}
		}
		total += len(file.Preview)
	}
	if len(summary.Artifacts) != 10 || previews != 8 || total > 32*1024 {
		t.Fatalf("preview envelope: artifacts=%d previews=%d bytes=%d", len(summary.Artifacts), previews, total)
	}
}
