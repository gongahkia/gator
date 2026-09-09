package document

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func sample() Spec {
	return Spec{Version: 1, Title: "Quarterly update", Blocks: []Block{{Kind: "heading", Level: 1, Text: "Summary"}, {Kind: "paragraph", Text: "Revenue improved."}, {Kind: "table", Headers: []string{"Region", "Revenue"}, Rows: [][]string{{"APAC", "12"}}}}}
}

func TestRenderDOCXTemplateFillsExplicitMarker(t *testing.T) {
	var template bytes.Buffer
	writer := zip.NewWriter(&template)
	part, _ := writer.Create("word/document.xml")
	_, _ = part.Write([]byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>{{gator:body}}</w:t></w:r></w:p></w:body></w:document>`))
	part, _ = writer.Create("[Content_Types].xml")
	_, _ = part.Write([]byte(`<Types/>`))
	_ = writer.Close()
	contents, _, err := RenderDOCXTemplate(sample(), bytes.NewReader(template.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	archive, _ := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	found := false
	for _, file := range archive.File {
		if file.Name == "word/document.xml" {
			reader, _ := file.Open()
			var value bytes.Buffer
			_, _ = value.ReadFrom(reader)
			_ = reader.Close()
			found = strings.Contains(value.String(), "Revenue improved.") && !strings.Contains(value.String(), "{{gator:body}}")
		}
	}
	if !found {
		t.Fatal("template marker was not replaced")
	}
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
