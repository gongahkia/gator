package document

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestDOCXRetainsMultilineTextInParagraphsAndTableCells(t *testing.T) {
	text := "First & second\r\nNext\tcolumn\rLast 🐊"
	contents, _, err := RenderDOCX(Spec{Title: "Structure", Blocks: []Block{{Kind: "paragraph", Text: text}, {Kind: "table", Headers: []string{"Label", "Details"}, Rows: [][]string{{"One", text}}}}})
	if err != nil {
		t.Fatal(err)
	}
	archive, _ := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	for _, part := range archive.File {
		if part.Name != "word/document.xml" {
			continue
		}
		reader, _ := part.Open()
		defer reader.Close()
		decoder := xml.NewDecoder(reader)
		var paragraphs []string
		var paragraph strings.Builder
		inText, gridColumns := false, 0
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			switch value := token.(type) {
			case xml.StartElement:
				switch value.Name.Local {
				case "p":
					paragraph.Reset()
				case "t":
					inText = true
				case "br":
					paragraph.WriteString("\n")
				case "tab":
					paragraph.WriteString("\t")
				case "gridCol":
					gridColumns++
				case "tr":
					if gridColumns != 2 {
						t.Fatal("table rows precede a complete column grid")
					}
				}
			case xml.CharData:
				if inText {
					paragraph.Write(value)
				}
			case xml.EndElement:
				if value.Name.Local == "t" {
					inText = false
				}
				if value.Name.Local == "p" {
					paragraphs = append(paragraphs, paragraph.String())
				}
			}
		}
		found := 0
		for _, value := range paragraphs {
			if value == "First & second\nNext\tcolumn\nLast 🐊" {
				found++
			}
		}
		if found != 2 {
			t.Fatalf("paragraph/cell line breaks lost: %#v", paragraphs)
		}
	}
}
