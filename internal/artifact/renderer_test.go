package artifact

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/document"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestSealBindsSemanticRendererEvidenceToArtifactBytes(t *testing.T) {
	spec := document.Spec{Title: "Board report", Blocks: []document.Block{{Kind: "paragraph", Text: "Evidence-backed summary."}}}.Normalize()
	contents, _, err := document.RenderDOCX(spec)
	if err != nil {
		t.Fatal(err)
	}
	outputDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(outputDirectory, "report.docx"), contents, 0o600); err != nil {
		t.Fatal(err)
	}
	output, _ := workspace.Open(outputDirectory)
	source, _ := workspace.Open(t.TempDir())
	evidence, err := NewRendererEvidence("report.docx", "gator.document.v1", spec, nil, contents)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	manifest, err := Seal(output, DefaultContract("report.docx"), SealOptions{RunID: "work-renderer", Objective: "Write report", Source: source, Renderers: []RendererEvidence{evidence}, StartedAt: now, FinishedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Renderers) != 1 || manifest.Renderers[0].SpecSHA256 == "" {
		t.Fatalf("renderers = %#v", manifest.Renderers)
	}
	evidence.ArtifactSHA256 = evidence.SpecSHA256
	if _, err := Seal(output, DefaultContract("report.docx"), SealOptions{RunID: "work-renderer-stale", Objective: "Write report", Source: source, Renderers: []RendererEvidence{evidence}, StartedAt: now, FinishedAt: now}); err == nil {
		t.Fatal("stale renderer evidence was accepted")
	}
}
