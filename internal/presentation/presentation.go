// Package presentation owns Gator's bounded PPTX creation, inspection, and
// conservative editing surface. It deliberately edits only well-understood
// PresentationML parts; every other package part (including VBA, media,
// embedded objects, and unsupported animation data) is copied through.
package presentation

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const Version = 1

const (
	maxArchiveBytes   = 64 * 1024 * 1024
	maxPartBytes      = 32 * 1024 * 1024
	maxArchiveEntries = 4096
	maxSlides         = 512
)

const presentationContentType = "application/vnd.openxmlformats-officedocument.presentationml.presentation"

// Spec is a constrained, model-safe presentation description. Media is not
// accepted here because models must not name arbitrary local paths; a template
// can retain existing media exactly, while later media import can use an
// explicit developer-selected source boundary.
type Spec struct {
	Version int     `json:"version"`
	Title   string  `json:"title"`
	Theme   string  `json:"theme,omitempty"`
	Slides  []Slide `json:"slides"`
}

type Slide struct {
	Title      string   `json:"title"`
	Body       []string `json:"body,omitempty"`
	Notes      string   `json:"notes,omitempty"`
	Transition string   `json:"transition,omitempty"`
}

// Edit is an intentionally small operation set for an existing deck. Indices
// are zero-based and refer to the deck's current order at each operation.
type Edit struct {
	Operation  string   `json:"operation"`
	Slide      int      `json:"slide,omitempty"`
	To         int      `json:"to,omitempty"`
	Find       string   `json:"find,omitempty"`
	Replace    string   `json:"replace,omitempty"`
	Title      string   `json:"title,omitempty"`
	Body       []string `json:"body,omitempty"`
	Notes      string   `json:"notes,omitempty"`
	Transition string   `json:"transition,omitempty"`
}

// Preview is a bounded text projection suitable for artifact evidence and a
// terminal UI. It never renders provider-supplied HTML.
type Preview struct {
	Title    string   `json:"title"`
	Theme    string   `json:"theme,omitempty"`
	Slides   int      `json:"slides"`
	Outline  []string `json:"outline"`
	Notes    int      `json:"notes,omitempty"`
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
	if s.Version != Version || invalidText(s.Title, 512) || len(s.Slides) == 0 || len(s.Slides) > maxSlides {
		return errors.New("presentation requires a bounded title and 1-512 slides")
	}
	if s.Theme != "professional" && s.Theme != "minimal" && s.Theme != "report" {
		return fmt.Errorf("unsupported presentation theme %q", s.Theme)
	}
	for index, slide := range s.Slides {
		if err := slide.validate(); err != nil {
			return fmt.Errorf("slide %d: %w", index+1, err)
		}
	}
	return nil
}

func (s Slide) validate() error {
	if invalidText(s.Title, 4096) || len(s.Body) > 256 || invalidTextOptional(s.Notes, 256*1024) || !validTransition(s.Transition) {
		return errors.New("slide title, body, notes, or transition is invalid")
	}
	for _, paragraph := range s.Body {
		if invalidText(paragraph, 64*1024) {
			return errors.New("slide body paragraph is invalid")
		}
	}
	return nil
}

func (s Spec) Preview() Preview {
	s = s.Normalize()
	preview := Preview{Title: s.Title, Theme: s.Theme, Slides: len(s.Slides), Outline: make([]string, 0, len(s.Slides))}
	for index, slide := range s.Slides {
		preview.Outline = append(preview.Outline, fmt.Sprintf("%d. %s", index+1, slide.Title))
		if slide.Notes != "" {
			preview.Notes++
		}
	}
	return preview
}

// RenderPPTX creates a deterministic, macro-free 16:9 PPTX. Notes are
// emitted using PresentationML notes parts, and the theme affects the trusted
// text styling only. It produces no external relationships or executable code.
func RenderPPTX(spec Spec) ([]byte, Preview, error) {
	spec = spec.Normalize()
	if err := spec.Validate(); err != nil {
		return nil, Preview{}, err
	}
	files := map[string][]byte{}
	files["[Content_Types].xml"] = []byte(contentTypes(spec))
	files["_rels/.rels"] = []byte(rootRelationships())
	files["ppt/presentation.xml"] = []byte(presentationXML(spec))
	files["ppt/_rels/presentation.xml.rels"] = []byte(presentationRelationships(spec))
	files["ppt/theme/theme1.xml"] = []byte(themeXML(spec.Theme))
	files["ppt/notesMasters/notesMaster1.xml"] = []byte(notesMasterXML())
	for index, slide := range spec.Slides {
		number := index + 1
		files[fmt.Sprintf("ppt/slides/slide%d.xml", number)] = []byte(slideXML(slide, spec.Theme))
		files[fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", number)] = []byte(slideRelationships(number, slide.Notes != ""))
		if slide.Notes != "" {
			files[fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", number)] = []byte(notesSlideXML(slide.Notes))
			files[fmt.Sprintf("ppt/notesSlides/_rels/notesSlide%d.xml.rels", number)] = []byte(notesRelationships(number))
		}
	}
	return writeFiles(files), spec.Preview(), nil
}

// InspectPPTX extracts only bounded visible slide and speaker-note text. It
// does not execute content, resolve external relationships, or inspect macros.
func InspectPPTX(contents []byte) (Preview, error) {
	deck, err := openDeck(contents)
	if err != nil {
		return Preview{}, err
	}
	preview := Preview{Slides: len(deck.slides), Outline: make([]string, 0, len(deck.slides))}
	if core := deck.files["docProps/core.xml"]; len(core.contents) > 0 {
		preview.Title = firstText(core.contents)
	}
	for index, slide := range deck.slides {
		texts := xmlText(slide.contents, 32*1024)
		title := "Untitled slide"
		if len(texts) > 0 && strings.TrimSpace(texts[0]) != "" {
			title = texts[0]
		}
		preview.Outline = append(preview.Outline, fmt.Sprintf("%d. %s", index+1, title))
		if notePath := deck.notePath(slide.path); notePath != "" {
			if len(xmlText(deck.files[notePath].contents, 32*1024)) > 0 {
				preview.Notes++
			}
		}
	}
	if len(deck.files["ppt/vbaProject.bin"].contents) > 0 {
		preview.Warnings = append(preview.Warnings, "Contains a VBA project; Gator preserved it without reading, executing, or editing it.")
	}
	if deck.hasUnsupportedAnimations() {
		preview.Warnings = append(preview.Warnings, "Contains animation data; unsupported animation details are preserved unchanged.")
	}
	return preview, nil
}

// EditPPTX applies bounded safe operations to a PPTX. Untouched package parts
// are copied as-is at the part-content level. In particular, vbaProject.bin,
// media, embedded files, charts, and unsupported animation nodes are never
// interpreted or regenerated by this package.
func EditPPTX(contents []byte, edits []Edit) ([]byte, Preview, error) {
	deck, err := openDeck(contents)
	if err != nil {
		return nil, Preview{}, err
	}
	if len(edits) == 0 || len(edits) > 256 {
		return nil, Preview{}, errors.New("presentation editing requires 1-256 operations")
	}
	for index, edit := range edits {
		if err := deck.apply(edit); err != nil {
			return nil, Preview{}, fmt.Errorf("presentation edit %d: %w", index+1, err)
		}
	}
	output, err := deck.write()
	if err != nil {
		return nil, Preview{}, err
	}
	preview, err := InspectPPTX(output)
	if err != nil {
		return nil, Preview{}, err
	}
	return output, preview, nil
}

type archivePart struct {
	contents []byte
	header   zip.FileHeader
}

type deck struct {
	files  map[string]archivePart
	slides []slideRef
}

type slideRef struct {
	id       uint32
	relID    string
	path     string
	contents []byte
}

func openDeck(contents []byte) (*deck, error) {
	if len(contents) == 0 || len(contents) > maxArchiveBytes {
		return nil, errors.New("PPTX is empty or exceeds 64 MiB")
	}
	archive, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		return nil, errors.New("PPTX is not a valid OOXML archive")
	}
	if len(archive.File) > maxArchiveEntries {
		return nil, errors.New("PPTX contains too many archive entries")
	}
	result := &deck{files: make(map[string]archivePart, len(archive.File))}
	var total uint64
	for _, file := range archive.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if unsafePartName(file.Name) || file.UncompressedSize64 > maxPartBytes {
			return nil, errors.New("PPTX contains an unsafe or oversized part")
		}
		total += file.UncompressedSize64
		if total > maxArchiveBytes*2 {
			return nil, errors.New("PPTX expanded contents exceed the safety limit")
		}
		reader, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(reader, maxPartBytes+1))
		_ = reader.Close()
		if readErr != nil || len(data) > maxPartBytes {
			return nil, errors.New("PPTX part cannot be safely read")
		}
		if _, duplicate := result.files[file.Name]; duplicate {
			return nil, errors.New("PPTX contains duplicate package parts")
		}
		result.files[file.Name] = archivePart{contents: data, header: file.FileHeader}
	}
	if _, ok := result.files["[Content_Types].xml"]; !ok {
		return nil, errors.New("PPTX is missing [Content_Types].xml")
	}
	if _, ok := result.files["ppt/presentation.xml"]; !ok {
		return nil, errors.New("PPTX is missing ppt/presentation.xml")
	}
	rels, ok := result.files["ppt/_rels/presentation.xml.rels"]
	if !ok {
		return nil, errors.New("PPTX is missing presentation relationships")
	}
	ordered, err := parseSlideRefs(result.files["ppt/presentation.xml"].contents, rels.contents)
	if err != nil {
		return nil, err
	}
	if len(ordered) == 0 || len(ordered) > maxSlides {
		return nil, errors.New("PPTX must contain 1-512 slides")
	}
	for index := range ordered {
		part, exists := result.files[ordered[index].path]
		if !exists {
			return nil, fmt.Errorf("PPTX is missing slide part %q", ordered[index].path)
		}
		ordered[index].contents = part.contents
	}
	result.slides = ordered
	return result, nil
}

func (d *deck) apply(edit Edit) error {
	switch edit.Operation {
	case "replace_text":
		if invalidText(edit.Find, 16*1024) || invalidTextOptional(edit.Replace, 64*1024) {
			return errors.New("replace_text requires bounded find and replace text")
		}
		changed := false
		for index := range d.slides {
			next, didChange := replaceTextRuns(d.slides[index].contents, edit.Find, edit.Replace, false)
			d.slides[index].contents = next
			d.files[d.slides[index].path] = withContents(d.files[d.slides[index].path], next)
			changed = changed || didChange
			if note := d.notePath(d.slides[index].path); note != "" {
				next, didChange = replaceTextRuns(d.files[note].contents, edit.Find, edit.Replace, false)
				d.files[note] = withContents(d.files[note], next)
				changed = changed || didChange
			}
		}
		if !changed {
			return errors.New("replace_text did not find the requested text")
		}
		return nil
	case "set_title", "set_body", "set_transition", "set_notes":
		if edit.Slide < 0 || edit.Slide >= len(d.slides) {
			return errors.New("slide index is outside the current deck")
		}
		return d.applySlide(edit)
	case "add_slide":
		slide := Slide{Title: edit.Title, Body: edit.Body, Notes: edit.Notes, Transition: edit.Transition}
		if err := slide.validate(); err != nil {
			return err
		}
		return d.addSlide(slide)
	case "delete_slide":
		if len(d.slides) == 1 {
			return errors.New("a deck must retain at least one slide")
		}
		if edit.Slide < 0 || edit.Slide >= len(d.slides) {
			return errors.New("slide index is outside the current deck")
		}
		return d.deleteSlide(edit.Slide)
	case "move_slide":
		if edit.Slide < 0 || edit.Slide >= len(d.slides) || edit.To < 0 || edit.To >= len(d.slides) {
			return errors.New("slide move index is outside the current deck")
		}
		value := d.slides[edit.Slide]
		d.slides = append(d.slides[:edit.Slide], d.slides[edit.Slide+1:]...)
		d.slides = append(d.slides, slideRef{})
		copy(d.slides[edit.To+1:], d.slides[edit.To:])
		d.slides[edit.To] = value
		return nil
	default:
		return fmt.Errorf("unsupported presentation edit operation %q", edit.Operation)
	}
}

func (d *deck) applySlide(edit Edit) error {
	slide := &d.slides[edit.Slide]
	switch edit.Operation {
	case "set_title":
		if invalidText(edit.Title, 4096) {
			return errors.New("set_title requires a bounded title")
		}
		next, changed := replaceFirstTextRun(slide.contents, edit.Title)
		if !changed {
			return errors.New("slide has no editable text run for its title")
		}
		slide.contents = next
		d.files[slide.path] = withContents(d.files[slide.path], next)
	case "set_body":
		if len(edit.Body) == 0 || len(edit.Body) > 256 {
			return errors.New("set_body requires 1-256 paragraphs")
		}
		for _, paragraph := range edit.Body {
			if invalidText(paragraph, 64*1024) {
				return errors.New("set_body contains invalid text")
			}
		}
		next, changed := replaceBodyText(slide.contents, edit.Body)
		if !changed {
			return errors.New("slide has no editable text run for its body")
		}
		slide.contents = next
		d.files[slide.path] = withContents(d.files[slide.path], next)
	case "set_transition":
		if !validTransition(edit.Transition) {
			return errors.New("set_transition has an unsupported transition")
		}
		next := setTransition(slide.contents, edit.Transition)
		slide.contents = next
		d.files[slide.path] = withContents(d.files[slide.path], next)
	case "set_notes":
		if invalidTextOptional(edit.Notes, 256*1024) {
			return errors.New("set_notes contains invalid text")
		}
		note := d.notePath(slide.path)
		if note == "" {
			return errors.New("set_notes requires an existing notes slide; use add_slide to add new notes")
		}
		next, changed := replaceFirstTextRun(d.files[note].contents, edit.Notes)
		if !changed {
			return errors.New("notes slide has no editable text run")
		}
		d.files[note] = withContents(d.files[note], next)
	}
	return nil
}

func (d *deck) addSlide(value Slide) error {
	if len(d.slides) >= maxSlides {
		return errors.New("PPTX cannot contain more than 512 slides")
	}
	maxNumber, maxID, maxRel := 0, uint32(255), 0
	for _, slide := range d.slides {
		maxID = max(maxID, slide.id)
		base := path.Base(slide.path)
		if number, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(base, "slide"), ".xml")); err == nil {
			maxNumber = max(maxNumber, number)
		}
	}
	for _, relation := range parseRelationships(d.files["ppt/_rels/presentation.xml.rels"].contents) {
		if strings.HasPrefix(relation.ID, "rId") {
			if number, err := strconv.Atoi(strings.TrimPrefix(relation.ID, "rId")); err == nil {
				maxRel = max(maxRel, number)
			}
		}
	}
	number, id, relID := maxNumber+1, maxID+1, fmt.Sprintf("rId%d", maxRel+1)
	partPath := fmt.Sprintf("ppt/slides/slide%d.xml", number)
	for {
		if _, exists := d.files[partPath]; !exists {
			break
		}
		number++
		partPath = fmt.Sprintf("ppt/slides/slide%d.xml", number)
	}
	contents := []byte(slideXML(value, "professional"))
	d.files[partPath] = newPart(partPath, contents)
	d.files[fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", number)] = newPart("", []byte(slideRelationships(number, value.Notes != "")))
	if value.Notes != "" {
		if _, exists := d.files["ppt/notesMasters/notesMaster1.xml"]; !exists {
			return errors.New("adding notes requires a deck with a notes master; preserve existing notes or use a Gator-created deck")
		}
		d.files[fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", number)] = newPart("", []byte(notesSlideXML(value.Notes)))
		d.files[fmt.Sprintf("ppt/notesSlides/_rels/notesSlide%d.xml.rels", number)] = newPart("", []byte(notesRelationships(number)))
		d.ensureContentOverride(fmt.Sprintf("/ppt/notesSlides/notesSlide%d.xml", number), "application/vnd.openxmlformats-officedocument.presentationml.notesSlide+xml")
	}
	d.ensureContentOverride("/"+partPath, "application/vnd.openxmlformats-officedocument.presentationml.slide+xml")
	d.addPresentationRelationship(relID, "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide", fmt.Sprintf("slides/slide%d.xml", number))
	d.slides = append(d.slides, slideRef{id: id, relID: relID, path: partPath, contents: contents})
	return nil
}

func (d *deck) deleteSlide(index int) error {
	slide := d.slides[index]
	note := d.notePath(slide.path)
	delete(d.files, slide.path)
	base := strings.TrimSuffix(path.Base(slide.path), ".xml")
	delete(d.files, "ppt/slides/_rels/"+base+".xml.rels")
	if note != "" {
		delete(d.files, note)
		delete(d.files, path.Join(path.Dir(note), "_rels", path.Base(note)+".rels"))
	}
	d.removePresentationRelationship(slide.relID)
	d.slides = append(d.slides[:index], d.slides[index+1:]...)
	return nil
}

func (d *deck) notePath(slidePath string) string {
	relsPath := path.Join(path.Dir(slidePath), "_rels", path.Base(slidePath)+".rels")
	rels, exists := d.files[relsPath]
	if !exists {
		return ""
	}
	for _, relation := range parseRelationships(rels.contents) {
		if strings.HasSuffix(relation.Type, "/notesSlide") && relation.TargetMode == "" {
			candidate := resolveTarget(slidePath, relation.Target)
			if _, exists := d.files[candidate]; exists {
				return candidate
			}
		}
	}
	return ""
}

func (d *deck) hasUnsupportedAnimations() bool {
	for _, slide := range d.slides {
		if bytes.Contains(slide.contents, []byte("<p:timing")) || bytes.Contains(slide.contents, []byte("<p:extLst")) {
			return true
		}
	}
	return false
}

func (d *deck) ensureContentOverride(partName, contentType string) {
	part := d.files["[Content_Types].xml"]
	if bytes.Contains(part.contents, []byte(`PartName="`+partName+`"`)) {
		return
	}
	part.contents = insertBefore(part.contents, "</Types>", `<Override PartName="`+partName+`" ContentType="`+contentType+`"/>`)
	d.files["[Content_Types].xml"] = part
}

func (d *deck) addPresentationRelationship(id, typ, target string) {
	part := d.files["ppt/_rels/presentation.xml.rels"]
	part.contents = insertBefore(part.contents, "</Relationships>", `<Relationship Id="`+xmlEscape(id)+`" Type="`+xmlEscape(typ)+`" Target="`+xmlEscape(target)+`"/>`)
	d.files["ppt/_rels/presentation.xml.rels"] = part
}

func (d *deck) removePresentationRelationship(id string) {
	part := d.files["ppt/_rels/presentation.xml.rels"]
	part.contents = removeRelationship(part.contents, id)
	d.files["ppt/_rels/presentation.xml.rels"] = part
}

func (d *deck) write() ([]byte, error) {
	presentation := d.files["ppt/presentation.xml"]
	presentation.contents = replaceSlideList(presentation.contents, d.slides)
	d.files["ppt/presentation.xml"] = presentation
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	names := make([]string, 0, len(d.files))
	for name := range d.files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		part := d.files[name]
		header := part.header
		header.Name = name
		if header.Method != zip.Store && header.Method != zip.Deflate {
			header.Method = zip.Deflate
		}
		header.SetMode(0o600)
		if header.Modified.IsZero() {
			header.Modified = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		entry, err := writer.CreateHeader(&header)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(part.contents); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

type relationship struct {
	ID         string
	Type       string
	Target     string
	TargetMode string
}

func parseSlideRefs(presentation, relationshipXML []byte) ([]slideRef, error) {
	relTargets := map[string]string{}
	for _, relation := range parseRelationships(relationshipXML) {
		if strings.HasSuffix(relation.Type, "/slide") && relation.TargetMode == "" {
			relTargets[relation.ID] = resolveTarget("ppt/presentation.xml", relation.Target)
		}
	}
	decoder := xml.NewDecoder(bytes.NewReader(presentation))
	var result []slideRef
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("PPTX presentation XML is invalid")
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "sldId" {
			continue
		}
		var numeric string
		var relID string
		for _, attribute := range start.Attr {
			if attribute.Name.Local != "id" {
				continue
			}
			if strings.Contains(attribute.Name.Space, "relationships") {
				relID = attribute.Value
			} else {
				numeric = attribute.Value
			}
		}
		id, conversionErr := strconv.ParseUint(numeric, 10, 32)
		if conversionErr != nil || relID == "" || relTargets[relID] == "" {
			return nil, errors.New("PPTX slide relationships are invalid")
		}
		result = append(result, slideRef{id: uint32(id), relID: relID, path: relTargets[relID]})
	}
	return result, nil
}

func parseRelationships(contents []byte) []relationship {
	decoder := xml.NewDecoder(bytes.NewReader(contents))
	var result []relationship
	for {
		token, err := decoder.Token()
		if err != nil {
			return result
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Relationship" {
			continue
		}
		var value relationship
		for _, attribute := range start.Attr {
			switch attribute.Name.Local {
			case "Id":
				value.ID = attribute.Value
			case "Type":
				value.Type = attribute.Value
			case "Target":
				value.Target = attribute.Value
			case "TargetMode":
				value.TargetMode = attribute.Value
			}
		}
		if value.ID != "" {
			result = append(result, value)
		}
	}
}

func replaceSlideList(contents []byte, slides []slideRef) []byte {
	start := bytes.Index(contents, []byte("<p:sldIdLst"))
	if start < 0 {
		return contents
	}
	openEndRelative := bytes.IndexByte(contents[start:], '>')
	endRelative := bytes.Index(contents[start:], []byte("</p:sldIdLst>"))
	if openEndRelative < 0 || endRelative < 0 {
		return contents
	}
	end := start + endRelative + len("</p:sldIdLst>")
	var list strings.Builder
	list.WriteString("<p:sldIdLst>")
	for _, slide := range slides {
		list.WriteString(`<p:sldId id="`)
		list.WriteString(strconv.FormatUint(uint64(slide.id), 10))
		list.WriteString(`" r:id="`)
		list.WriteString(xmlEscape(slide.relID))
		list.WriteString(`"/>`)
	}
	list.WriteString("</p:sldIdLst>")
	_ = openEndRelative // retained to make malformed open tags fail closed above.
	return append(append([]byte(nil), contents[:start]...), append([]byte(list.String()), contents[end:]...)...)
}

func replaceFirstTextRun(contents []byte, replacement string) ([]byte, bool) {
	return replaceTextRuns(contents, "", replacement, true)
}

func replaceBodyText(contents []byte, values []string) ([]byte, bool) {
	matches := textRunPattern.FindAllIndex(contents, -1)
	if len(matches) < 2 {
		return contents, false
	}
	// Keep the title run (the first visible run) and write the requested body
	// into the next run. Existing further body shapes remain untouched because
	// their layout may encode unsupported semantics.
	start, end := matches[1][0], matches[1][1]
	openEnd := bytes.IndexByte(contents[start:end], '>')
	if openEnd < 0 {
		return contents, false
	}
	value := xmlEscape(strings.Join(values, "\n"))
	replacement := append(append([]byte(nil), contents[start:start+openEnd+1]...), append([]byte(value), contents[end-len("</a:t>"):end]...)...)
	return append(append([]byte(nil), contents[:start]...), append(replacement, contents[end:]...)...), true
}

var textRunPattern = regexp.MustCompile(`(?s)<a:t(?:\s[^>]*)?>.*?</a:t>`)

func replaceTextRuns(contents []byte, find, replacement string, firstOnly bool) ([]byte, bool) {
	matches := textRunPattern.FindAllIndex(contents, -1)
	if len(matches) == 0 {
		return contents, false
	}
	var output bytes.Buffer
	position := 0
	changed := false
	for _, match := range matches {
		output.Write(contents[position:match[0]])
		run := contents[match[0]:match[1]]
		openEnd := bytes.IndexByte(run, '>')
		if openEnd < 0 {
			return contents, false
		}
		text := decodeXMLText(run[openEnd+1 : len(run)-len("</a:t>")])
		if (firstOnly && !changed) || (!firstOnly && strings.Contains(text, find)) {
			if firstOnly {
				text = replacement
			} else {
				text = strings.ReplaceAll(text, find, replacement)
			}
			output.Write(run[:openEnd+1])
			output.WriteString(xmlEscape(text))
			output.WriteString("</a:t>")
			changed = true
		} else {
			output.Write(run)
		}
		position = match[1]
	}
	output.Write(contents[position:])
	return output.Bytes(), changed
}

func setTransition(contents []byte, transition string) []byte {
	transitionPattern := regexp.MustCompile(`(?s)<p:transition(?:\s[^>]*)?(?:/>|>.*?</p:transition>)`)
	contents = transitionPattern.ReplaceAll(contents, nil)
	if transition == "none" {
		return contents
	}
	value := `<p:transition advClick="1"><p:` + transition + `/></p:transition>`
	return insertBefore(contents, "</p:sld>", value)
}

func removeRelationship(contents []byte, id string) []byte {
	for position := 0; ; {
		startRelative := bytes.Index(contents[position:], []byte("<Relationship"))
		if startRelative < 0 {
			return contents
		}
		start := position + startRelative
		endRelative := bytes.IndexByte(contents[start:], '>')
		if endRelative < 0 {
			return contents
		}
		end := start + endRelative + 1
		candidate := contents[start:end]
		if bytes.Contains(candidate, []byte(`Id="`+id+`"`)) || bytes.Contains(candidate, []byte(`Id='`+id+`'`)) {
			return append(append([]byte(nil), contents[:start]...), contents[end:]...)
		}
		position = end
	}
}

func insertBefore(contents []byte, closing, value string) []byte {
	position := bytes.LastIndex(contents, []byte(closing))
	if position < 0 {
		return contents
	}
	return append(append([]byte(nil), contents[:position]...), append([]byte(value), contents[position:]...)...)
}

func withContents(part archivePart, contents []byte) archivePart {
	part.contents = contents
	return part
}

func newPart(name string, contents []byte) archivePart {
	return archivePart{contents: contents, header: zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)}}
}

func unsafePartName(name string) bool {
	return name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || path.Clean(name) != name || strings.HasPrefix(name, "../") || strings.ContainsRune(name, 0)
}

func resolveTarget(source, target string) string {
	if strings.HasPrefix(target, "/") || strings.Contains(target, "\\") {
		return ""
	}
	value := path.Clean(path.Join(path.Dir(source), target))
	if value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return ""
	}
	return value
}

func invalidText(value string, limit int) bool {
	return strings.TrimSpace(value) == "" || invalidTextOptional(value, limit)
}
func invalidTextOptional(value string, limit int) bool {
	return len(value) > limit || strings.ContainsRune(value, 0)
}
func validTransition(value string) bool {
	return value == "" || value == "none" || value == "fade" || value == "push" || value == "wipe"
}

func xmlEscape(value string) string {
	var output bytes.Buffer
	_ = xml.EscapeText(&output, []byte(value))
	return output.String()
}

func decodeXMLText(value []byte) string {
	var output strings.Builder
	decoder := xml.NewDecoder(bytes.NewReader(append([]byte("<x>"), append(value, []byte("</x>")...)...)))
	for {
		token, err := decoder.Token()
		if err != nil {
			return output.String()
		}
		if character, ok := token.(xml.CharData); ok {
			output.Write([]byte(character))
		}
	}
}

func xmlText(contents []byte, limit int) []string {
	decoder := xml.NewDecoder(bytes.NewReader(contents))
	var values []string
	for {
		token, err := decoder.Token()
		if err != nil || len(values) >= limit {
			return values
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "t" {
			continue
		}
		var value string
		if err := decoder.DecodeElement(&value, &start); err == nil && strings.TrimSpace(value) != "" {
			values = append(values, value)
		}
	}
}

func firstText(contents []byte) string {
	values := xmlText(contents, 1)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func writeFiles(files map[string][]byte) []byte {
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)}
		entry, err := archive.CreateHeader(header)
		if err != nil {
			panic(err) // bytes.Buffer writer and package-controlled names cannot fail here.
		}
		if _, err := entry.Write(files[name]); err != nil {
			panic(err)
		}
	}
	if err := archive.Close(); err != nil {
		panic(err)
	}
	return output.Bytes()
}

func contentTypes(spec Spec) string {
	var value strings.Builder
	value.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/><Override PartName="/ppt/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/><Override PartName="/ppt/notesMasters/notesMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.notesMaster+xml"/>`)
	for index, slide := range spec.Slides {
		number := index + 1
		value.WriteString(fmt.Sprintf(`<Override PartName="/ppt/slides/slide%d.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`, number))
		if slide.Notes != "" {
			value.WriteString(fmt.Sprintf(`<Override PartName="/ppt/notesSlides/notesSlide%d.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.notesSlide+xml"/>`, number))
		}
	}
	value.WriteString(`</Types>`)
	return value.String()
}

func rootRelationships() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/></Relationships>`
}

func presentationXML(spec Spec) string {
	var slides strings.Builder
	for index := range spec.Slides {
		slides.WriteString(fmt.Sprintf(`<p:sldId id="%d" r:id="rId%d"/>`, 256+index, index+1))
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><p:presentation xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:sldIdLst>` + slides.String() + `</p:sldIdLst><p:notesMasterIdLst><p:notesMasterId r:id="rId` + strconv.Itoa(len(spec.Slides)+1) + `"/></p:notesMasterIdLst><p:sldSz cx="12192000" cy="6858000" type="screen16x9"/><p:notesSz cx="6858000" cy="9144000"/></p:presentation>`
}

func presentationRelationships(spec Spec) string {
	var value strings.Builder
	value.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for index := range spec.Slides {
		value.WriteString(fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide%d.xml"/>`, index+1, index+1))
	}
	next := len(spec.Slides) + 1
	value.WriteString(fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesMaster" Target="notesMasters/notesMaster1.xml"/><Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="theme/theme1.xml"/>`, next, next+1))
	value.WriteString(`</Relationships>`)
	return value.String()
}

func slideRelationships(number int, notes bool) string {
	value := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`
	if notes {
		value += fmt.Sprintf(`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide" Target="../notesSlides/notesSlide%d.xml"/>`, number)
	}
	return value + `</Relationships>`
}

func notesRelationships(number int) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="../slides/slide%d.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesMaster" Target="../notesMasters/notesMaster1.xml"/></Relationships>`, number)
}

func slideXML(slide Slide, theme string) string {
	primary, accent := "17365D", "275D8C"
	if theme == "minimal" {
		primary, accent = "222222", "555555"
	}
	if theme == "report" {
		primary, accent = "1F4E79", "5B9BD5"
	}
	body := strings.Join(slide.Body, "\n")
	value := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` + textShape(2, "Title", slide.Title, 762000, 457200, 10668000, 914400, primary, 2800, true) + textShape(3, "Body", body, 762000, 1600200, 10668000, 4572000, accent, 1800, false) + `</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>`
	if slide.Transition != "" && slide.Transition != "none" {
		value += `<p:transition advClick="1"><p:` + slide.Transition + `/></p:transition>`
	}
	return value + `</p:sld>`
}

func notesSlideXML(notes string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><p:notes xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld name=""><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` + textShape(2, "Notes Placeholder", notes, 0, 0, 6858000, 9144000, "222222", 1400, false) + `</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:notes>`
}

func notesMasterXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><p:notesMaster xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld name=""><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld><p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/><p:hf dt="0" hdr="0" ftr="0" sldNum="0"/><p:txStyles/></p:notesMaster>`
}

func textShape(id int, name, value string, x, y, cx, cy int, color string, size int, bold bool) string {
	boldValue := ""
	if bold {
		boldValue = ` b="1"`
	}
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/><a:ln><a:noFill/></a:ln></p:spPr><p:txBody><a:bodyPr wrap="square"/><a:lstStyle/><a:p><a:r><a:rPr lang="en-US" sz="%d"%s><a:solidFill><a:srgbClr val="%s"/></a:solidFill></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody></p:sp>`, id, xmlEscape(name), x, y, cx, cy, size, boldValue, color, xmlEscape(value))
}

func themeXML(theme string) string {
	primary, accent := "17365D", "275D8C"
	if theme == "minimal" {
		primary, accent = "222222", "555555"
	}
	if theme == "report" {
		primary, accent = "1F4E79", "5B9BD5"
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="Gator"><a:themeElements><a:clrScheme name="Gator"><a:dk1><a:srgbClr val="000000"/></a:dk1><a:lt1><a:srgbClr val="FFFFFF"/></a:lt1><a:dk2><a:srgbClr val="222222"/></a:dk2><a:lt2><a:srgbClr val="F5F7FA"/></a:lt2><a:accent1><a:srgbClr val="` + primary + `"/></a:accent1><a:accent2><a:srgbClr val="` + accent + `"/></a:accent2><a:accent3><a:srgbClr val="70AD47"/></a:accent3><a:accent4><a:srgbClr val="FFC000"/></a:accent4><a:accent5><a:srgbClr val="ED7D31"/></a:accent5><a:accent6><a:srgbClr val="A5A5A5"/></a:accent6><a:hlink><a:srgbClr val="0563C1"/></a:hlink><a:folHlink><a:srgbClr val="954F72"/></a:folHlink></a:clrScheme><a:fontScheme name="Gator"><a:majorFont><a:latin typeface="Aptos Display"/></a:majorFont><a:minorFont><a:latin typeface="Aptos"/></a:minorFont></a:fontScheme><a:fmtScheme name="Gator"><a:fillStyleLst/><a:lnStyleLst/><a:effectStyleLst/><a:bgFillStyleLst/></a:fmtScheme></a:themeElements></a:theme>`
}
