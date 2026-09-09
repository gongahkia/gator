package artifact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/workspace"
)

func TestPreviewBundleReturnsMediaAwareSafeText(t *testing.T) {
	bundlePath, _ := sealedBundleFixture(t)
	bundle, err := OpenBundle(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundlePath, "output", "report.md"), []byte("# Report\n\x1b[31mnot red\x1b[0m\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Reseal the intentionally control-bearing artifact so the preview test is
	// about rendering safety rather than bundle-tamper detection.
	manifest, err := Seal(bundle.Output, bundle.Manifest.Contract, SealOptions{
		RunID: bundle.Manifest.RunID, Objective: bundle.Manifest.Objective,
		Source: mustOpenRoot(t, t.TempDir()), StartedAt: bundle.Manifest.StartedAt, FinishedAt: bundle.Manifest.FinishedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	bundle.Manifest = manifest
	previews, err := PreviewBundle(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(previews) != 1 || previews[0].Kind != "markdown" || strings.ContainsRune(previews[0].Content, '\x1b') || !strings.Contains(previews[0].Content, "not red") {
		t.Fatalf("previews = %#v", previews)
	}
}

func TestPreviewFileSummarizesCSVAndBoundsText(t *testing.T) {
	csvPreview := previewFile(File{Path: "data.csv", MediaType: "text/csv", Bytes: 24}, []byte("name,value\na,1\nb,2\n"))
	if csvPreview.Kind != "table" || !strings.Contains(csvPreview.Summary, "2 data rows × 2 columns") {
		t.Fatalf("CSV preview = %#v", csvPreview)
	}
	long := strings.Repeat("界", maxPreviewBytes)
	textPreview := previewFile(File{Path: "long.txt", MediaType: "text/plain", Bytes: int64(len(long))}, []byte(long))
	if !textPreview.Truncated || len(textPreview.Content) > maxPreviewBytes || !strings.HasSuffix(textPreview.Content, "界") {
		t.Fatalf("bounded preview bytes=%d truncated=%v", len(textPreview.Content), textPreview.Truncated)
	}
}

func mustOpenRoot(t *testing.T, path string) workspace.Root {
	t.Helper()
	root, err := workspace.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return root
}
