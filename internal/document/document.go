// Package document validates and renders Gator's provider-independent document specification.
package document

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/signintech/gopdf"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

const Version = 1

type Spec struct {
	Version int     `json:"version"`
	Title   string  `json:"title"`
	Author  string  `json:"author,omitempty"`
	Subject string  `json:"subject,omitempty"`
	Theme   string  `json:"theme,omitempty"`
	Header  string  `json:"header,omitempty"`
	Footer  string  `json:"footer,omitempty"`
	Blocks  []Block `json:"blocks"`
}

type Block struct {
	Kind    string     `json:"kind"`
	Text    string     `json:"text,omitempty"`
	Level   int        `json:"level,omitempty"`
	Items   []string   `json:"items,omitempty"`
	Headers []string   `json:"headers,omitempty"`
	Rows    [][]string `json:"rows,omitempty"`
}

type Preview struct {
	Title    string   `json:"title"`
	Theme    string   `json:"theme"`
	Outline  []string `json:"outline"`
	Blocks   int      `json:"blocks"`
	Pages    int      `json:"pages,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

func (s Spec) Normalize() Spec {
	if s.Version == 0 {
		s.Version = Version
	}
	if s.Theme == "" {
		s.Theme = "professional"
	}
	return s
}

func (s Spec) Validate() error {
	s = s.Normalize()
	if s.Version != Version || strings.TrimSpace(s.Title) == "" || len(s.Title) > 512 || len(s.Blocks) == 0 || len(s.Blocks) > 2_000 {
		return errors.New("document requires a bounded title and 1-2000 blocks")
	}
	if s.Theme != "professional" && s.Theme != "minimal" && s.Theme != "report" {
		return fmt.Errorf("unsupported document theme %q", s.Theme)
	}
	for index, block := range s.Blocks {
		if err := block.validate(); err != nil {
			return fmt.Errorf("document block %d: %w", index+1, err)
		}
	}
	return nil
}

func (b Block) validate() error {
	textOK := strings.TrimSpace(b.Text) != "" && len(b.Text) <= 256*1024 && !strings.ContainsRune(b.Text, 0)
	switch b.Kind {
	case "heading":
		if !textOK || b.Level < 1 || b.Level > 3 {
			return errors.New("heading requires text and level 1-3")
		}
	case "paragraph", "callout":
		if !textOK {
			return errors.New("text block is empty or invalid")
		}
	case "bullet_list", "numbered_list":
		if len(b.Items) == 0 || len(b.Items) > 1_000 {
			return errors.New("list requires 1-1000 items")
		}
		for _, item := range b.Items {
			if strings.TrimSpace(item) == "" || len(item) > 64*1024 || strings.ContainsRune(item, 0) {
				return errors.New("list item is invalid")
			}
		}
	case "table":
		if len(b.Headers) == 0 || len(b.Headers) > 64 || len(b.Rows) > 10_000 {
			return errors.New("table requires 1-64 columns and at most 10000 rows")
		}
		for _, row := range b.Rows {
			if len(row) != len(b.Headers) {
				return errors.New("table contains a ragged row")
			}
		}
	case "page_break":
	default:
		return fmt.Errorf("unsupported kind %q", b.Kind)
	}
	return nil
}

func (s Spec) Preview() Preview {
	s = s.Normalize()
	preview := Preview{Title: s.Title, Theme: s.Theme, Blocks: len(s.Blocks)}
	for _, block := range s.Blocks {
		if block.Kind == "heading" {
			preview.Outline = append(preview.Outline, strings.Repeat("  ", block.Level-1)+block.Text)
		}
	}
	return preview
}

// RenderDOCX produces a deterministic, macro-free OOXML document.
func RenderDOCX(spec Spec) ([]byte, Preview, error) {
	spec = spec.Normalize()
	if err := spec.Validate(); err != nil {
		return nil, Preview{}, err
	}
	var document strings.Builder
	document.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	document.WriteString(paragraphXML(spec.Title, "Title", false))
	for _, block := range spec.Blocks {
		switch block.Kind {
		case "heading":
			document.WriteString(paragraphXML(block.Text, fmt.Sprintf("Heading%d", block.Level), false))
		case "paragraph":
			document.WriteString(paragraphXML(block.Text, "Normal", false))
		case "callout":
			document.WriteString(paragraphXML(block.Text, "Quote", false))
		case "bullet_list", "numbered_list":
			for index, item := range block.Items {
				prefix := "• "
				if block.Kind == "numbered_list" {
					prefix = fmt.Sprintf("%d. ", index+1)
				}
				document.WriteString(paragraphXML(prefix+item, "ListParagraph", false))
			}
		case "table":
			document.WriteString(tableXML(block.Headers, block.Rows))
		case "page_break":
			document.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
		}
	}
	document.WriteString(`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:body></w:document>`)
	styles := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:rPr><w:rFonts w:ascii="Aptos" w:hAnsi="Aptos"/><w:sz w:val="22"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:rPr><w:b/><w:color w:val="17365D"/><w:sz w:val="40"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:rPr><w:b/><w:color w:val="17365D"/><w:sz w:val="32"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:basedOn w:val="Normal"/><w:rPr><w:b/><w:color w:val="275D8C"/><w:sz w:val="28"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Heading3"><w:name w:val="heading 3"/><w:basedOn w:val="Normal"/><w:rPr><w:b/><w:sz w:val="24"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Quote"><w:name w:val="Quote"/><w:basedOn w:val="Normal"/><w:pPr><w:ind w:left="480"/></w:pPr><w:rPr><w:i/><w:color w:val="555555"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="ListParagraph"><w:name w:val="List Paragraph"/><w:basedOn w:val="Normal"/><w:pPr><w:ind w:left="360"/></w:pPr></w:style></w:styles>`
	files := map[string]string{
		"[Content_Types].xml":          `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/></Types>`,
		"_rels/.rels":                  `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":            document.String(),
		"word/styles.xml":              styles,
		"word/_rels/document.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`,
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/styles.xml", "word/_rels/document.xml.rels"} {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)}
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return nil, Preview{}, err
		}
		if _, err := io.WriteString(writer, strings.ReplaceAll(files[name], `\"`, `"`)); err != nil {
			return nil, Preview{}, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, Preview{}, err
	}
	return output.Bytes(), spec.Preview(), nil
}

// RenderDOCXTemplate fills explicit gator:title and gator:body placeholders or
// tagged content controls while preserving the rest of a safe macro-free DOCX.
func RenderDOCXTemplate(spec Spec, template io.Reader) ([]byte, Preview, error) {
	spec = spec.Normalize()
	if err := spec.Validate(); err != nil {
		return nil, Preview{}, err
	}
	payload, err := io.ReadAll(io.LimitReader(template, 64*1024*1024+1))
	if err != nil || len(payload) > 64*1024*1024 {
		return nil, Preview{}, errors.New("DOCX template is unreadable or exceeds 64 MiB")
	}
	archive, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, Preview{}, errors.New("DOCX template is not a valid archive")
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	found := false
	for _, file := range archive.File {
		lower := strings.ToLower(file.Name)
		if strings.Contains(lower, "vbaproject") || strings.HasSuffix(lower, ".bin") {
			return nil, Preview{}, errors.New("macro-enabled DOCX templates are not allowed")
		}
		reader, err := file.Open()
		if err != nil {
			return nil, Preview{}, err
		}
		contents, err := io.ReadAll(io.LimitReader(reader, 16*1024*1024+1))
		_ = reader.Close()
		if err != nil || len(contents) > 16*1024*1024 {
			return nil, Preview{}, errors.New("DOCX template part exceeds 16 MiB")
		}
		if strings.HasSuffix(lower, ".rels") && bytes.Contains(contents, []byte(`TargetMode="External"`)) {
			return nil, Preview{}, errors.New("DOCX templates with external relationships are not allowed")
		}
		if file.Name == "word/document.xml" {
			documentXML := string(contents)
			title := xmlTextRun(spec.Title)
			body := templateBodyXML(spec)
			var replaced bool
			documentXML, replaced = replaceMarker(documentXML, "gator:title", title)
			found = found || replaced
			documentXML, replaced = replaceMarker(documentXML, "gator:body", body)
			found = found || replaced
			contents = []byte(documentXML)
		}
		header := file.FileHeader
		header.SetMode(0o600)
		target, err := writer.CreateHeader(&header)
		if err != nil {
			return nil, Preview{}, err
		}
		if _, err := target.Write(contents); err != nil {
			return nil, Preview{}, err
		}
	}
	if !found {
		return nil, Preview{}, errors.New("DOCX template requires a gator:title or gator:body placeholder/content control")
	}
	if err := writer.Close(); err != nil {
		return nil, Preview{}, err
	}
	return output.Bytes(), spec.Preview(), nil
}

func replaceMarker(documentXML, marker, replacement string) (string, bool) {
	placeholder := "{{" + marker + "}}"
	if strings.Contains(documentXML, placeholder) {
		return strings.ReplaceAll(documentXML, placeholder, replacement), true
	}
	needle := `w:val="` + marker + `"`
	position := strings.Index(documentXML, needle)
	if position < 0 {
		return documentXML, false
	}
	start := strings.LastIndex(documentXML[:position], "<w:sdt>")
	endRelative := strings.Index(documentXML[position:], "</w:sdt>")
	if start < 0 || endRelative < 0 {
		return documentXML, false
	}
	end := position + endRelative + len("</w:sdt>")
	contentStartRelative := strings.Index(documentXML[start:end], "<w:sdtContent>")
	contentEndRelative := strings.Index(documentXML[start:end], "</w:sdtContent>")
	if contentStartRelative < 0 || contentEndRelative < 0 {
		return documentXML, false
	}
	contentStart := start + contentStartRelative + len("<w:sdtContent>")
	contentEnd := start + contentEndRelative
	return documentXML[:contentStart] + replacement + documentXML[contentEnd:], true
}

func xmlTextRun(value string) string {
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(value))
	return escaped.String()
}

func templateBodyXML(spec Spec) string {
	var result strings.Builder
	for _, block := range spec.Blocks {
		switch block.Kind {
		case "heading":
			result.WriteString(paragraphXML(block.Text, fmt.Sprintf("Heading%d", block.Level), false))
		case "paragraph":
			result.WriteString(paragraphXML(block.Text, "Normal", false))
		case "callout":
			result.WriteString(paragraphXML(block.Text, "Quote", false))
		case "bullet_list", "numbered_list":
			for index, item := range block.Items {
				prefix := "• "
				if block.Kind == "numbered_list" {
					prefix = fmt.Sprintf("%d. ", index+1)
				}
				result.WriteString(paragraphXML(prefix+item, "ListParagraph", false))
			}
		case "table":
			result.WriteString(tableXML(block.Headers, block.Rows))
		case "page_break":
			result.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
		}
	}
	return strings.ReplaceAll(result.String(), `\"`, `"`)
}

func paragraphXML(text, style string, bold bool) string {
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(text))
	run := ""
	if bold {
		run = "<w:rPr><w:b/></w:rPr>"
	}
	return `<w:p><w:pPr><w:pStyle w:val="` + style + `"/></w:pPr><w:r>` + run + `<w:t xml:space="preserve">` + escaped.String() + `</w:t></w:r></w:p>`
}

func tableXML(headers []string, rows [][]string) string {
	var result strings.Builder
	result.WriteString(`<w:tbl><w:tblPr><w:tblStyle w:val="TableGrid"/><w:tblW w:w="0" w:type="auto"/></w:tblPr>`)
	all := append([][]string{headers}, rows...)
	for rowIndex, row := range all {
		result.WriteString("<w:tr>")
		for _, cell := range row {
			result.WriteString("<w:tc><w:tcPr/><w:p><w:r>")
			if rowIndex == 0 {
				result.WriteString("<w:rPr><w:b/></w:rPr>")
			}
			var escaped bytes.Buffer
			_ = xml.EscapeText(&escaped, []byte(cell))
			result.WriteString(`<w:t xml:space="preserve">` + escaped.String() + "</w:t></w:r></w:p></w:tc>")
		}
		result.WriteString("</w:tr>")
	}
	result.WriteString("</w:tbl>")
	return result.String()
}

// RenderPDF lays out the semantic document with embedded Go fonts.
func RenderPDF(spec Spec) ([]byte, Preview, error) {
	spec = spec.Normalize()
	if err := spec.Validate(); err != nil {
		return nil, Preview{}, err
	}
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	if err := pdf.AddTTFFontData("regular", goregular.TTF); err != nil {
		return nil, Preview{}, err
	}
	if err := pdf.AddTTFFontData("bold", gobold.TTF); err != nil {
		return nil, Preview{}, err
	}
	const left, top, width, bottom = 54.0, 54.0, 487.0, 790.0
	y := top
	newPage := func() {
		pdf.AddPage()
		y = top
	}
	newPage()
	write := func(text, font string, size, lineHeight float64) error {
		if err := pdf.SetFont(font, "", size); err != nil {
			return err
		}
		lines := wrapPDF(pdf, text, width)
		for _, line := range lines {
			if y+lineHeight > bottom {
				newPage()
				if err := pdf.SetFont(font, "", size); err != nil {
					return err
				}
			}
			pdf.SetXY(left, y)
			if err := pdf.Cell(&gopdf.Rect{W: width, H: lineHeight}, line); err != nil {
				return err
			}
			y += lineHeight
		}
		y += lineHeight * .4
		return nil
	}
	if err := write(spec.Title, "bold", 24, 30); err != nil {
		return nil, Preview{}, err
	}
	for _, block := range spec.Blocks {
		switch block.Kind {
		case "heading":
			size := map[int]float64{1: 18, 2: 15, 3: 12}[block.Level]
			if err := write(block.Text, "bold", size, size*1.45); err != nil {
				return nil, Preview{}, err
			}
		case "paragraph", "callout":
			if err := write(block.Text, "regular", 10.5, 15); err != nil {
				return nil, Preview{}, err
			}
		case "bullet_list", "numbered_list":
			for index, item := range block.Items {
				prefix := "• "
				if block.Kind == "numbered_list" {
					prefix = fmt.Sprintf("%d. ", index+1)
				}
				if err := write(prefix+item, "regular", 10.5, 15); err != nil {
					return nil, Preview{}, err
				}
			}
		case "table":
			if err := write(strings.Join(block.Headers, "  |  "), "bold", 9, 13); err != nil {
				return nil, Preview{}, err
			}
			for _, row := range block.Rows {
				if err := write(strings.Join(row, "  |  "), "regular", 9, 13); err != nil {
					return nil, Preview{}, err
				}
			}
		case "page_break":
			newPage()
		}
	}
	var output bytes.Buffer
	if _, err := pdf.WriteTo(&output); err != nil {
		return nil, Preview{}, err
	}
	preview := spec.Preview()
	preview.Pages = pdf.GetNumberOfPages()
	return output.Bytes(), preview, nil
}

func wrapPDF(pdf *gopdf.GoPdf, value string, width float64) []string {
	var result []string
	for _, paragraph := range strings.Split(value, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line := words[0]
		for _, word := range words[1:] {
			candidate := line + " " + word
			measured, err := pdf.MeasureTextWidth(candidate)
			if err == nil && measured <= width {
				line = candidate
			} else {
				result = append(result, line)
				line = word
			}
		}
		result = append(result, line)
	}
	return result
}
