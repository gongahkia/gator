package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/worktui"
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
	var summary worktui.BundleSummary
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
