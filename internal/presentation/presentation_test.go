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

func TestNewDeckRejectsNotesOnlyContent(t *testing.T) {
	_, _, err := RenderPPTX(Spec{Title: "Daily brief", Slides: []Slide{{Title: "Results", Notes: "Facts hidden from slide"}}})
	if err == nil || !strings.Contains(err.Error(), "visible body text") {
		t.Fatalf("notes-only deck was accepted: %v", err)
	}
}

func TestEditPPTXReplacesTextAcrossSlides(t *testing.T) {
	contents, _, err := RenderPPTX(Spec{Title: "Monthly review", Slides: []Slide{
		{Title: "Review", Body: []string{"September 2026 draft"}},
		{Title: "Health", Body: []string{"Availability: 99.2%", "Open priority-one tickets: 2"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	updated, _, err := EditPPTX(contents, []Edit{
		{Operation: "replace_text", Find: "September 2026 draft", Replace: "October 2026 update"},
		{Operation: "replace_text", Find: "99.2%", Replace: "99.7%"},
		{Operation: "replace_text", Find: "Open priority-one tickets: 2", Replace: "Open priority-one tickets: 1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	slides, err := SlideTexts(updated)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(slides, "\n")
	for _, expected := range []string{"October 2026 update", "99.7%", "priority-one tickets: 1"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("edited deck lacks %q: %s", expected, joined)
		}
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
