package presentation

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

const drawingNamespace = "http://schemas.openxmlformats.org/drawingml/2006/main"
const slideNamespace = "http://schemas.openxmlformats.org/presentationml/2006/main"

type editableTextBody struct {
	name, placeholder        string
	start, end               int
	paragraphStyle, runStyle []byte
}

// Index whole shape text bodies, not individual runs: Office commonly splits a
// title or paragraph across several runs just to represent formatting changes.
func slideTextBodies(contents []byte) ([]editableTextBody, error) {
	decoder := xml.NewDecoder(bytes.NewReader(contents))
	var bodies []editableTextBody
	var current editableTextBody
	depth, shapeDepth, bodyDepth, paragraphDepth := 0, 0, 0, 0
	styleDepth, styleStart, runDepth, runStart := 0, 0, 0, 0
	firstParagraph := false
	for {
		before := int(decoder.InputOffset())
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch node := token.(type) {
		case xml.StartElement:
			depth++
			if node.Name.Space == slideNamespace && node.Name.Local == "sp" {
				if shapeDepth != 0 {
					return nil, errors.New("nested slide shape is unsupported")
				}
				shapeDepth = depth
				current = editableTextBody{start: -1, end: -1}
			}
			if shapeDepth == 0 {
				continue
			}
			if node.Name.Space == slideNamespace {
				switch node.Name.Local {
				case "cNvPr":
					for _, attr := range node.Attr {
						if attr.Name.Local == "name" {
							current.name = attr.Value
						}
					}
				case "ph":
					current.placeholder = "obj"
					for _, attr := range node.Attr {
						if attr.Name.Local == "type" {
							current.placeholder = attr.Value
						}
					}
				case "txBody":
					bodyDepth = depth
				}
			}
			if bodyDepth != 0 && node.Name.Space == drawingNamespace {
				if node.Name.Local == "p" && depth == bodyDepth+1 {
					paragraphDepth = depth
					firstParagraph = current.start < 0
					if firstParagraph {
						current.start = before
					}
				}
				if firstParagraph && node.Name.Local == "pPr" && depth == paragraphDepth+1 {
					styleDepth = depth
					styleStart = before
				}
				if firstParagraph && node.Name.Local == "rPr" && depth == paragraphDepth+2 && current.runStyle == nil {
					runDepth = depth
					runStart = before
				}
			}
		case xml.EndElement:
			after := int(decoder.InputOffset())
			if styleDepth != 0 && depth == styleDepth {
				current.paragraphStyle = contents[styleStart:after]
				styleDepth = 0
			}
			if runDepth != 0 && depth == runDepth {
				current.runStyle = contents[runStart:after]
				runDepth = 0
			}
			if paragraphDepth != 0 && depth == paragraphDepth {
				current.end = after
				paragraphDepth = 0
				firstParagraph = false
			}
			if bodyDepth != 0 && depth == bodyDepth {
				if current.start < 0 {
					current.start = before
					current.end = before
				}
				bodyDepth = 0
			}
			if shapeDepth != 0 && depth == shapeDepth {
				if current.start >= 0 {
					bodies = append(bodies, current)
				}
				shapeDepth = 0
			}
			depth--
		}
	}
	return bodies, nil
}

func (body editableTextBody) title() bool {
	return body.placeholder == "title" || body.placeholder == "ctrTitle" || (body.placeholder == "" && body.name == "Title")
}

func slideTitle(contents []byte) string {
	bodies, err := slideTextBodies(contents)
	if err != nil {
		return ""
	}
	var title *editableTextBody
	for index := range bodies {
		if bodies[index].title() {
			if title != nil {
				return ""
			}
			title = &bodies[index]
		}
	}
	if title == nil {
		return ""
	}
	decoder := xml.NewDecoder(io.LimitReader(bytes.NewReader(contents[title.start:title.end]), 32*1024))
	var value strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			return strings.TrimSpace(value.String())
		}
		switch node := token.(type) {
		case xml.StartElement:
			if node.Name.Local == "t" {
				var text string
				if decoder.DecodeElement(&text, &node) == nil {
					value.WriteString(text)
				}
			} else if node.Name.Local == "br" {
				value.WriteByte(' ')
			}
		case xml.EndElement:
			if node.Name.Local == "p" {
				value.WriteByte(' ')
			}
		}
	}
}

func (body editableTextBody) excluded() bool {
	switch body.placeholder {
	case "dt", "ftr", "hdr", "sldNum", "sldImg":
		return true
	}
	return false
}

func replaceShapeText(contents []byte, kind string, values []string) ([]byte, error) {
	bodies, err := slideTextBodies(contents)
	if err != nil {
		return nil, err
	}
	var candidates, fallback []editableTextBody
	for _, body := range bodies {
		if body.excluded() {
			continue
		}
		if kind == "title" {
			if body.title() {
				candidates = append(candidates, body)
			}
			continue
		}
		if body.title() {
			continue
		}
		fallback = append(fallback, body)
		if body.placeholder == "body" || (body.placeholder == "" && (body.name == "Body" || (kind == "notes" && body.name == "Notes Placeholder"))) {
			candidates = append(candidates, body)
		}
	}
	if len(candidates) == 0 && kind != "title" {
		candidates = fallback
	}
	if len(candidates) != 1 {
		return nil, errors.New("cannot identify one " + kind + " text shape; use precise replace_text edits for this slide")
	}
	body := candidates[0]
	paragraphs := textParagraphs(values, string(body.paragraphStyle), string(body.runStyle))
	// Imported decks may use a different prefix for DrawingML.
	paragraphs = strings.ReplaceAll(paragraphs, "<a:p>", `<a:p xmlns:a="`+drawingNamespace+`">`)
	output := append([]byte(nil), contents[:body.start]...)
	output = append(output, paragraphs...)
	return append(output, contents[body.end:]...), nil
}

func textParagraphs(values []string, paragraphStyle, runStyle string) string {
	var result strings.Builder
	for _, value := range values {
		value = strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
		for _, line := range strings.Split(value, "\n") {
			result.WriteString("<a:p>" + paragraphStyle + "<a:r>" + runStyle + "<a:t>" + xmlEscape(line) + "</a:t></a:r></a:p>")
		}
	}
	return result.String()
}
