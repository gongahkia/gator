package presentation

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestRenderInspectAndEditPPTX(t *testing.T) {
	contents, preview, err := RenderPPTX(Spec{
		Title:  "Quarterly review",
		Slides: []Slide{{Title: "Results", Body: []string{"Revenue grew", "Margin improved"}, Notes: "Mention the new region.", Transition: "fade"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Slides != 1 || preview.Notes != 1 {
		t.Fatalf("render preview = %#v", preview)
	}
	inspected, err := InspectPPTX(contents)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Slides != 1 || !strings.Contains(inspected.Outline[0], "Results") || inspected.Notes != 1 {
		t.Fatalf("inspection = %#v", inspected)
	}
	edited, after, err := EditPPTX(contents, []Edit{
		{Operation: "set_title", Slide: 0, Title: "Updated results"},
		{Operation: "set_notes", Slide: 0, Notes: "Updated speaker note."},
		{Operation: "add_slide", Title: "Next steps", Body: []string{"Ship"}, Transition: "wipe"},
		{Operation: "move_slide", Slide: 1, To: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if after.Slides != 2 || !strings.Contains(after.Outline[1], "Updated results") {
		t.Fatalf("edited preview = %#v", after)
	}
	if _, err := openDeck(edited); err != nil {
		t.Fatalf("edited deck failed its safe parser: %v", err)
	}
}

func TestEditPPTXPreservesVBAAndUnknownParts(t *testing.T) {
	contents, _, err := RenderPPTX(Spec{Title: "Title", Slides: []Slide{{Title: "One", Body: []string{"body"}}}})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		t.Fatal(err)
	}
	var template bytes.Buffer
	writer := zip.NewWriter(&template)
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		header := file.FileHeader
		entry, err := writer.CreateHeader(&header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range map[string]string{"ppt/vbaProject.bin": "macro-bytes", "ppt/media/image1.bin": "image-bytes", "ppt/diagrams/unknown.xml": "<unknown/>"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	edited, _, err := EditPPTX(template.Bytes(), []Edit{{Operation: "set_title", Slide: 0, Title: "Two"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := zip.NewReader(bytes.NewReader(edited), int64(len(edited)))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"ppt/vbaProject.bin": "macro-bytes", "ppt/media/image1.bin": "image-bytes", "ppt/diagrams/unknown.xml": "<unknown/>"}
	for _, file := range result.File {
		expected, ok := want[file.Name]
		if !ok {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		actual, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if string(actual) != expected {
			t.Fatalf("part %s = %q, want %q", file.Name, actual, expected)
		}
		delete(want, file.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing untouched parts: %#v", want)
	}
}
