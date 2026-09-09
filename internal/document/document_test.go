package document

import (
	"archive/zip"
	"bytes"
	"testing"
)

func sample() Spec {
	return Spec{Version: 1, Title: "Quarterly update", Blocks: []Block{{Kind: "heading", Level: 1, Text: "Summary"}, {Kind: "paragraph", Text: "Revenue improved."}, {Kind: "table", Headers: []string{"Region", "Revenue"}, Rows: [][]string{{"APAC", "12"}}}}}
}

func TestRenderDOCXAndPDF(t *testing.T) {
	docx, preview, err := RenderDOCX(sample())
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil || len(archive.File) < 5 || len(preview.Outline) != 1 {
		t.Fatalf("docx files=%d preview=%#v err=%v", len(archive.File), preview, err)
	}
	pdf, preview, err := RenderPDF(sample())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) || preview.Pages < 1 {
		t.Fatalf("invalid PDF: bytes=%d preview=%#v", len(pdf), preview)
	}
}
