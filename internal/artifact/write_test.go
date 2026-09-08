package artifact

import (
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/workspace"
)

func TestWriteTextStagesPortableArtifact(t *testing.T) {
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	contract := DefaultContract("reports/summary.md")
	if err := WriteText(root, contract, "reports/summary.md", "# Summary\n"); err != nil {
		t.Fatal(err)
	}
	contents, err := root.ReadRegularFile("reports/summary.md", 1024)
	if err != nil || string(contents) != "# Summary\n" {
		t.Fatalf("contents = %q, %v", contents, err)
	}
}

func TestWriteTextRejectsUnportableOrInvalidContent(t *testing.T) {
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	contract := DefaultContract("report.md")
	contract.MaxArtifactBytes = 4
	contract.MaxTotalBytes = 4
	for _, test := range []struct {
		path    string
		content string
	}{
		{path: "../report.md", content: "ok"},
		{path: `folder\\report.md`, content: "ok"},
		{path: "report.md", content: "nul\x00byte"},
		{path: "report.md", content: "large"},
	} {
		if err := WriteText(root, contract, test.path, test.content); err == nil {
			t.Fatalf("WriteText(%q, %q) succeeded", test.path, test.content)
		}
	}
	if _, err := root.ReadRegularFile("report.md", 16); err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("invalid write created an artifact: %v", err)
	}
}
