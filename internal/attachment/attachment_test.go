package attachment

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestLoadPDFAndTextAttachments(t *testing.T) {
	repository := t.TempDir()
	if err := os.WriteFile(filepath.Join(repository, "report.pdf"), []byte("%PDF-1.7\ncontent"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "notes.md"), []byte("# Notes\nimportant detail"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := workspace.Open(repository)
	if err != nil {
		t.Fatal(err)
	}
	pdf, supported, err := Load(root, "report.pdf", 1024)
	if err != nil || !supported || pdf.MediaType != PDFMediaType || string(pdf.Data) != "%PDF-1.7\ncontent" {
		t.Fatalf("PDF attachment = %#v, supported = %v, err = %v", pdf, supported, err)
	}
	text, supported, err := Load(root, "notes.md", 1024)
	if err != nil || !supported || text.MediaType != "text/plain" || string(text.Data) != "# Notes\nimportant detail" {
		t.Fatalf("text attachment = %#v, supported = %v, err = %v", text, supported, err)
	}
}

func TestLoadInputsSharesImageAndDocumentLimits(t *testing.T) {
	repository := t.TempDir()
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00}
	if err := os.WriteFile(filepath.Join(repository, "screen.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "notes.md"), []byte("# Notes\nimportant detail"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := workspace.Open(repository)
	if err != nil {
		t.Fatal(err)
	}
	images, documents, err := LoadInputs(root, []Input{
		{Path: "screen.png", Kind: ImageInput},
		{Path: "notes.md", Kind: DocumentInput},
	})
	if err != nil {
		t.Fatalf("load inputs: %v", err)
	}
	if len(images) != 1 || images[0].Name != "screen.png" || string(images[0].Data) != string(png) {
		t.Fatalf("images = %#v", images)
	}
	if len(documents) != 1 || documents[0].Name != "notes.md" || string(documents[0].Data) != "# Notes\nimportant detail" {
		t.Fatalf("documents = %#v", documents)
	}
	if _, _, err := LoadInputs(root, []Input{{Path: "screen.png", Kind: ImageInput}, {Path: "screen.png", Kind: ImageInput}}); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("duplicate attachment error = %v", err)
	}
	if _, _, err := LoadInputs(root, []Input{{Path: "notes.md", Kind: ImageInput}}); err == nil || !strings.Contains(err.Error(), "must be PNG") {
		t.Fatalf("wrong image type error = %v", err)
	}
	if _, _, err := LoadInputs(root, []Input{{Path: "../outside.png", Kind: ImageInput}}); err == nil || !strings.Contains(err.Error(), "escapes the workspace") {
		t.Fatalf("escaping input error = %v", err)
	}
}

func TestValidateLoadedEnforcesProgrammaticInputBoundary(t *testing.T) {
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	if err := ValidateLoaded(
		[]agent.Image{{Name: "screen.png", MediaType: "image/png", Data: png}},
		[]agent.Attachment{{Name: "notes.txt", MediaType: "text/plain", Data: []byte("notes")}},
	); err != nil {
		t.Fatalf("valid loaded inputs: %v", err)
	}
	if err := ValidateLoaded(
		[]agent.Image{{Name: "same", MediaType: "image/png", Data: png}},
		[]agent.Attachment{{Name: "same", MediaType: "text/plain", Data: []byte("notes")}},
	); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("duplicate loaded input error = %v", err)
	}
	if err := ValidateLoaded([]agent.Image{{Name: "fake.png", MediaType: "image/png", Data: []byte("not an image")}}, nil); err == nil || !strings.Contains(err.Error(), "mismatched") {
		t.Fatalf("mismatched image error = %v", err)
	}
	tooMany := make([]agent.Attachment, MaxInputs+1)
	if err := ValidateLoaded(nil, tooMany); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("input count error = %v", err)
	}
}

func TestLoadExtractsDOCXODTAndXLSX(t *testing.T) {
	repository := t.TempDir()
	writeArchive(t, filepath.Join(repository, "report.docx"), map[string]string{
		"word/document.xml": `<w:document xmlns:w="urn:test"><w:body><w:p><w:r><w:t>first paragraph</w:t></w:r></w:p><w:p><w:r><w:t>second paragraph</w:t></w:r></w:p></w:body></w:document>`,
	})
	writeArchive(t, filepath.Join(repository, "report.odt"), map[string]string{
		"content.xml": `<office:document-content xmlns:office="urn:office" xmlns:text="urn:text"><office:body><office:text><text:p>first paragraph</text:p><text:p>second paragraph</text:p></office:text></office:body></office:document-content>`,
	})
	writeArchive(t, filepath.Join(repository, "table.xlsx"), map[string]string{
		"xl/sharedStrings.xml":     `<sst><si><t>name</t></si><si><t>Ada</t></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row><c t="s"><v>0</v></c><c><v>7</v></c></row><row><c t="s"><v>1</v></c><c t="inlineStr"><is><t>ok</t></is></c></row></sheetData></worksheet>`,
	})
	root, err := workspace.Open(repository)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		path string
		want string
	}{
		{path: "report.docx", want: "first paragraph\nsecond paragraph"},
		{path: "report.odt", want: "first paragraph\nsecond paragraph"},
		{path: "table.xlsx", want: "[sheet sheet1]\nname,7\nAda,ok"},
	} {
		attachment, supported, err := Load(root, check.path, 4096)
		if err != nil || !supported || attachment.MediaType != "text/plain" || !strings.Contains(string(attachment.Data), check.want) {
			t.Fatalf("load %s = %#v, supported = %v, err = %v", check.path, attachment, supported, err)
		}
	}
}

func TestLoadRejectsInvalidOrOversizedSupportedAttachments(t *testing.T) {
	repository := t.TempDir()
	if err := os.WriteFile(filepath.Join(repository, "broken.pdf"), []byte("not a PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "large.txt"), bytes.Repeat([]byte("x"), 32), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := workspace.Open(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, supported, err := Load(root, "broken.pdf", 1024); !supported || err == nil || !strings.Contains(err.Error(), "not a PDF") {
		t.Fatalf("broken PDF supported = %v, err = %v", supported, err)
	}
	if _, supported, err := Load(root, "large.txt", 16); !supported || err == nil || !strings.Contains(err.Error(), "no larger") {
		t.Fatalf("large text supported = %v, err = %v", supported, err)
	}
	if _, supported, err := Load(root, "unknown.bin", 1024); supported || err != nil {
		t.Fatalf("unknown attachment supported = %v, err = %v", supported, err)
	}
}

func TestOfficeArchiveRejectsUnsafeAndDuplicateEntries(t *testing.T) {
	repository := t.TempDir()
	writeArchive(t, filepath.Join(repository, "unsafe.docx"), map[string]string{
		"../word/document.xml": "outside",
		"word/document.xml":    "inside",
	})
	root, err := workspace.Open(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, supported, err := Load(root, "unsafe.docx", 4096); !supported || err == nil || !strings.Contains(err.Error(), "unsafe entry") {
		t.Fatalf("unsafe archive supported = %v, err = %v", supported, err)
	}

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for range 2 {
		entry, err := archive.Create("word/document.xml")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("duplicate")); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := documentArchive(buffer.Bytes(), 4096); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate archive error = %v", err)
	}
}

func writeArchive(t *testing.T, path string, files map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for name, contents := range files {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
