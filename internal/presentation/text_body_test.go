package presentation

import (
	"bytes"
	"strings"
	"testing"
)

func TestSlideBodyParagraphsRemainSeparateAndEditsReplaceWholeTextShapes(t *testing.T) {
	contents, _, err := RenderPPTX(Spec{Title: "Review", Slides: []Slide{{Title: "OLD TITLE", Body: []string{"OLD FIRST", "OLD SECOND"}, Notes: "OLD NOTES ONE\nOLD NOTES TWO"}}})
	if err != nil {
		t.Fatal(err)
	}
	deck, err := openDeck(contents)
	if err != nil {
		t.Fatal(err)
	}
	slide := string(deck.slides[0].contents)
	if strings.Contains(slide, "OLD FIRST\nOLD SECOND") {
		t.Fatal("body paragraphs flattened into one DrawingML text run")
	}
	slide = strings.Replace(slide, `<a:t>OLD TITLE</a:t>`, `<a:t>OLD </a:t></a:r><a:r><a:rPr i="1"/><a:t>TITLE</a:t>`, 1)
	slide = strings.Replace(slide, `<a:t>OLD FIRST</a:t>`, `<a:t>OLD </a:t></a:r><a:r><a:rPr b="1"/><a:t>FIRST</a:t>`, 1)
	caption := textShape(8, "Caption", "UNRELATED CAPTION", 0, 0, 1000000, 200000, "222222", 1200, false)
	footer := strings.Replace(textShape(9, "Footer", "UNRELATED FOOTER", 0, 6200000, 1000000, 200000, "222222", 1000, false), "<p:nvPr/>", `<p:nvPr><p:ph type="ftr"/></p:nvPr>`, 1)
	slide = strings.Replace(slide, `<p:sp><p:nvSpPr><p:cNvPr id="2"`, caption+`<p:sp><p:nvSpPr><p:cNvPr id="2"`, 1)
	slide = strings.Replace(slide, "</p:spTree>", footer+"</p:spTree>", 1)
	deck.slides[0].contents = []byte(slide)
	deck.files[deck.slides[0].path] = withContents(deck.files[deck.slides[0].path], []byte(slide))
	template, err := deck.write()
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), template...)
	inspection, err := InspectPPTX(template)
	if err != nil || inspection.Outline[0] != "1. OLD TITLE" {
		t.Fatalf("split title inspection: %#v, %v", inspection, err)
	}
	updated, preview, err := EditPPTX(template, []Edit{
		{Operation: "set_title", Slide: 0, Title: "New title"},
		{Operation: "set_body", Slide: 0, Body: []string{"New first", "New second"}},
		{Operation: "set_notes", Slide: 0, Notes: "New notes one\nNew notes two"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Outline[0] != "1. New title" {
		t.Fatalf("preview used an unrelated caption: %#v", preview)
	}
	if !bytes.Equal(template, before) {
		t.Fatal("source template changed")
	}
	result, err := openDeck(updated)
	if err != nil {
		t.Fatal(err)
	}
	visible := string(result.slides[0].contents)
	for _, old := range []string{"OLD ", ">TITLE<", ">FIRST<"} {
		if strings.Contains(visible, old) {
			t.Fatalf("superseded text survived: %q", old)
		}
	}
	for _, want := range []string{"New title", "New first", "New second", caption, footer} {
		if !strings.Contains(visible, want) {
			t.Fatalf("new content or unrelated shape lost: %q", want)
		}
	}
	for _, style := range []string{`sz="2800" b="1"`, `val="17365D"`, `sz="1800"`, `val="275D8C"`} {
		if !strings.Contains(visible, style) {
			t.Fatalf("original text styling lost: %s", style)
		}
	}
	notes := string(result.files[result.notePath(result.slides[0].path)].contents)
	if strings.Contains(notes, "OLD NOTES") || !strings.Contains(notes, "New notes one") || !strings.Contains(notes, "New notes two") {
		t.Fatalf("notes were not replaced completely: %s", notes)
	}
}

func TestShapeTextUsesPlaceholdersAndSupportsAlternateNamespacePrefixes(t *testing.T) {
	contents := []byte(slideXML(Slide{Title: "Old heading", Body: []string{"Old body"}}, "minimal"))
	contents = bytes.Replace(contents, []byte(`name="Title"/><p:cNvSpPr txBox="1"/><p:nvPr/>`), []byte(`name="External heading"/><p:cNvSpPr txBox="1"/><p:nvPr><p:ph type="ctrTitle"/></p:nvPr>`), 1)
	contents = bytes.ReplaceAll(contents, []byte("xmlns:a="), []byte("xmlns:d="))
	contents = bytes.ReplaceAll(contents, []byte("a:"), []byte("d:"))
	updated, err := replaceShapeText(contents, "title", []string{"New heading"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err = replaceShapeText(updated, "body", []string{"First line\nSecond line"})
	if err != nil {
		t.Fatal(err)
	}
	bodies, err := slideTextBodies(updated)
	if err != nil || len(bodies) != 2 {
		t.Fatalf("edited namespaces not readable: %d, %v", len(bodies), err)
	}
	if strings.Contains(string(updated), "Old heading") || strings.Contains(string(updated), "Old body") {
		t.Fatal("old text remains")
	}
	if !strings.Contains(string(updated), `xmlns:a="`+drawingNamespace+`"`) {
		t.Fatal("new DrawingML prefix is not bound")
	}
}

func TestShapeTextRejectsAmbiguousBodyWithoutChangingOtherShapes(t *testing.T) {
	contents := []byte(slideXML(Slide{Title: "Heading", Body: []string{"Left column"}}, "minimal"))
	second := textShape(7, "Body", "Right column", 5000000, 1600200, 5000000, 4000000, "222222", 1800, false)
	contents = bytes.Replace(contents, []byte("</p:spTree>"), []byte(second+"</p:spTree>"), 1)
	before := append([]byte(nil), contents...)
	if _, err := replaceShapeText(contents, "body", []string{"Replacement"}); err == nil {
		t.Fatal("ambiguous body replacement accepted")
	}
	if !bytes.Equal(contents, before) {
		t.Fatal("failed edit mutated input")
	}
}
