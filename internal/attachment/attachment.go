// Package attachment loads bounded developer-supplied document context.
package attachment

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workspace"
)

const PDFMediaType = "application/pdf"

const (
	// MaxInputs is the maximum number of developer-supplied images and
	// documents that a single prompt may include.
	MaxInputs = 4
	// MaxImageBytes and MaxDocumentBytes bound one input before it reaches a
	// provider. Extracted Office text is bounded by MaxDocumentBytes too.
	MaxImageBytes    = 4 * 1024 * 1024
	MaxDocumentBytes = 4 * 1024 * 1024
	// MaxTotalBytes bounds the provider-visible bytes across all inputs.
	MaxTotalBytes = 8 * 1024 * 1024

	maxArchiveEntries     = 4_096
	maxSpreadsheetSheets  = 128
	maxSpreadsheetRows    = 100_000
	maxSpreadsheetStrings = 100_000
)

// InputKind identifies the explicit developer-selected type of one prompt
// attachment. Keeping image and document flags separate avoids treating an
// arbitrary binary file as model-visible input.
type InputKind uint8

const (
	ImageInput InputKind = iota
	DocumentInput
)

// Input is a repository-relative, developer-selected image or document.
type Input struct {
	Path string
	Kind InputKind
}

// IsImage reports whether a path has one of Gator's supported visual-input
// extensions. LoadImage verifies the actual media type before returning bytes.
func IsImage(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".webp":
		return true
	default:
		return false
	}
}

// LoadImage resolves one repository-local visual attachment. Only PNG, JPEG,
// and WebP are accepted because all Gator-native image adapters have a stable
// encoding for those formats.
func LoadImage(root workspace.Root, path string, maxBytes int) (agent.Image, error) {
	if !IsImage(path) {
		return agent.Image{}, fmt.Errorf("image @%s must be PNG, JPEG, or WebP", path)
	}
	if maxBytes < 1 {
		return agent.Image{}, errors.New("image size limit must be positive")
	}
	contents, err := readRegularFile(root, path, maxBytes)
	if err != nil {
		return agent.Image{}, err
	}
	mediaType := http.DetectContentType(contents)
	if !supportedImageMediaType(mediaType) {
		return agent.Image{}, fmt.Errorf("image @%s must be PNG, JPEG, or WebP", path)
	}
	return agent.Image{Name: path, MediaType: mediaType, Data: contents}, nil
}

// LoadInputs loads a bounded, explicit collection of repository-local visual
// and document attachments. It is shared by terminal and automation callers so
// they use the same byte, type, duplicate, and workspace-boundary rules.
func LoadInputs(root workspace.Root, inputs []Input) ([]agent.Image, []agent.Attachment, error) {
	if len(inputs) > MaxInputs {
		return nil, nil, fmt.Errorf("attach at most %d files per task", MaxInputs)
	}
	images := make([]agent.Image, 0, len(inputs))
	attachments := make([]agent.Attachment, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	totalBytes := 0
	for _, input := range inputs {
		if strings.TrimSpace(input.Path) == "" {
			return nil, nil, errors.New("attachment path must not be empty")
		}
		cleanPath := filepath.Clean(input.Path)
		if _, duplicate := seen[cleanPath]; duplicate {
			return nil, nil, fmt.Errorf("attachment %q was supplied more than once", input.Path)
		}
		seen[cleanPath] = struct{}{}

		switch input.Kind {
		case ImageInput:
			image, err := LoadImage(root, input.Path, MaxImageBytes)
			if err != nil {
				return nil, nil, err
			}
			if totalBytes+len(image.Data) > MaxTotalBytes {
				return nil, nil, fmt.Errorf("attached files exceed the %d MiB total limit", MaxTotalBytes/(1024*1024))
			}
			images = append(images, image)
			totalBytes += len(image.Data)
		case DocumentInput:
			remaining := MaxTotalBytes - totalBytes
			if remaining < 1 {
				return nil, nil, fmt.Errorf("attached files exceed the %d MiB total limit", MaxTotalBytes/(1024*1024))
			}
			loaded, supported, err := Load(root, input.Path, min(MaxDocumentBytes, remaining))
			if err != nil {
				return nil, nil, err
			}
			if !supported {
				return nil, nil, fmt.Errorf("attachment @%s must be a supported document or UTF-8 text file", input.Path)
			}
			if totalBytes+len(loaded.Data) > MaxTotalBytes {
				return nil, nil, fmt.Errorf("attached files exceed the %d MiB total limit", MaxTotalBytes/(1024*1024))
			}
			attachments = append(attachments, loaded)
			totalBytes += len(loaded.Data)
		default:
			return nil, nil, fmt.Errorf("attachment @%s has an unsupported input kind", input.Path)
		}
	}
	if err := ValidateLoaded(images, attachments); err != nil {
		return nil, nil, err
	}
	return images, attachments, nil
}

// ValidateLoaded enforces the prompt-input boundary for callers that already
// have in-memory provider inputs. File-loading callers pass through this same
// check before returning, so CLI and programmatic Work requests cannot drift.
func ValidateLoaded(images []agent.Image, attachments []agent.Attachment) error {
	if len(images)+len(attachments) > MaxInputs {
		return fmt.Errorf("attach at most %d files per task", MaxInputs)
	}
	seen := make(map[string]struct{}, len(images)+len(attachments))
	totalBytes := 0
	validate := func(name, mediaType string, data []byte, maxBytes int) error {
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsRune(name, 0) {
			return errors.New("attachment name is missing or invalid")
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("attachment %q was supplied more than once", name)
		}
		seen[name] = struct{}{}
		if len(data) == 0 || len(data) > maxBytes {
			return fmt.Errorf("attachment %q has an invalid size", name)
		}
		totalBytes += len(data)
		if totalBytes > MaxTotalBytes {
			return fmt.Errorf("attached files exceed the %d MiB total limit", MaxTotalBytes/(1024*1024))
		}
		return nil
	}
	for _, image := range images {
		if err := validate(image.Name, image.MediaType, image.Data, MaxImageBytes); err != nil {
			return err
		}
		detected := http.DetectContentType(image.Data)
		if !supportedImageMediaType(image.MediaType) || detected != image.MediaType {
			return fmt.Errorf("attachment %q has unsupported or mismatched media type %q", image.Name, image.MediaType)
		}
	}
	for _, item := range attachments {
		if err := validate(item.Name, item.MediaType, item.Data, MaxDocumentBytes); err != nil {
			return err
		}
		switch item.MediaType {
		case PDFMediaType:
			if !bytes.HasPrefix(item.Data, []byte("%PDF-")) {
				return fmt.Errorf("attachment %q is not a PDF", item.Name)
			}
		case "text/plain":
			if !utf8.Valid(item.Data) {
				return fmt.Errorf("attachment %q is not valid UTF-8 text", item.Name)
			}
		default:
			return fmt.Errorf("attachment %q has unsupported media type %q", item.Name, item.MediaType)
		}
	}
	return nil
}

// IsSupported reports whether a path can become an explicit attachment. Other
// @ references retain their existing inspect-in-workspace behavior.
func IsSupported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf", ".docx", ".odt", ".xlsx", ".txt", ".md", ".markdown", ".rst", ".csv", ".tsv", ".json", ".jsonl", ".yaml", ".yml", ".toml", ".xml", ".html", ".htm", ".log", ".ini", ".cfg", ".conf":
		return true
	default:
		return false
	}
}

// Load resolves a repository-local attachment and returns a bounded provider
// representation. PDFs retain their bytes for native document APIs. Text is
// copied verbatim, while office documents are converted to plain text locally.
func Load(root workspace.Root, path string, maxBytes int) (agent.Attachment, bool, error) {
	if !IsSupported(path) {
		return agent.Attachment{}, false, nil
	}
	if maxBytes < 1 {
		return agent.Attachment{}, true, errors.New("attachment size limit must be positive")
	}
	contents, err := readRegularFile(root, path, maxBytes)
	if err != nil {
		return agent.Attachment{}, true, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	attachment := agent.Attachment{Name: path}
	switch ext {
	case ".pdf":
		if !bytes.HasPrefix(contents, []byte("%PDF-")) {
			return agent.Attachment{}, true, fmt.Errorf("attachment @%s is not a PDF", path)
		}
		attachment.MediaType = PDFMediaType
		attachment.Data = contents
	case ".docx":
		text, err := extractDOCX(contents, maxBytes)
		if err != nil {
			return agent.Attachment{}, true, fmt.Errorf("extract @%s: %w", path, err)
		}
		attachment.MediaType = "text/plain"
		attachment.Data = []byte(text)
	case ".odt":
		text, err := extractODT(contents, maxBytes)
		if err != nil {
			return agent.Attachment{}, true, fmt.Errorf("extract @%s: %w", path, err)
		}
		attachment.MediaType = "text/plain"
		attachment.Data = []byte(text)
	case ".xlsx":
		text, err := extractXLSX(contents, maxBytes)
		if err != nil {
			return agent.Attachment{}, true, fmt.Errorf("extract @%s: %w", path, err)
		}
		attachment.MediaType = "text/plain"
		attachment.Data = []byte(text)
	default:
		if !utf8.Valid(contents) || bytes.IndexByte(contents, 0) >= 0 {
			return agent.Attachment{}, true, fmt.Errorf("attachment @%s is not UTF-8 text", path)
		}
		attachment.MediaType = "text/plain"
		attachment.Data = contents
	}
	if len(attachment.Data) == 0 {
		return agent.Attachment{}, true, fmt.Errorf("attachment @%s is empty", path)
	}
	if len(attachment.Data) > maxBytes {
		return agent.Attachment{}, true, fmt.Errorf("attachment @%s exceeds the %d MiB limit after extraction", path, maxBytes/(1024*1024))
	}
	return attachment, true, nil
}

func readRegularFile(root workspace.Root, path string, maxBytes int) ([]byte, error) {
	contents, err := root.ReadRegularFile(path, int64(maxBytes))
	if err != nil {
		return nil, fmt.Errorf("read attachment @%s: %w", path, err)
	}
	return contents, nil
}

func supportedImageMediaType(mediaType string) bool {
	switch mediaType {
	case "image/png", "image/jpeg", "image/webp":
		return true
	default:
		return false
	}
}

func extractDOCX(contents []byte, maxBytes int) (string, error) {
	entry, err := zipEntry(contents, "word/document.xml", maxBytes)
	if err != nil {
		return "", err
	}
	return wordXMLText(entry, maxBytes)
}

func extractODT(contents []byte, maxBytes int) (string, error) {
	entry, err := zipEntry(contents, "content.xml", maxBytes)
	if err != nil {
		return "", err
	}
	return documentXMLText(entry, maxBytes)
}

func zipEntry(contents []byte, name string, maxBytes int) ([]byte, error) {
	reader, err := documentArchive(contents, maxBytes)
	if err != nil {
		return nil, err
	}
	for _, entry := range reader.File {
		if entry.Name != name {
			continue
		}
		if entry.UncompressedSize64 > uint64(maxBytes) {
			return nil, errors.New("document text exceeds the extraction limit")
		}
		stream, err := entry.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", name, err)
		}
		defer stream.Close()
		data, err := io.ReadAll(io.LimitReader(stream, int64(maxBytes)+1))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		if len(data) > maxBytes {
			return nil, errors.New("document text exceeds the extraction limit")
		}
		return data, nil
	}
	return nil, fmt.Errorf("document archive does not contain %s", name)
}

func wordXMLText(data []byte, maxBytes int) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var text strings.Builder
	inText := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("decode document XML: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "t", "instrText":
				inText++
			case "tab":
				text.WriteByte('\t')
			case "br", "cr", "p":
				appendLineBreak(&text)
			}
		case xml.EndElement:
			switch value.Name.Local {
			case "t", "instrText":
				if inText > 0 {
					inText--
				}
			case "p":
				appendLineBreak(&text)
			}
		case xml.CharData:
			if inText > 0 {
				text.Write([]byte(value))
			}
		}
		if text.Len() > maxBytes {
			return "", errors.New("document text exceeds the extraction limit")
		}
	}
	return strings.TrimSpace(text.String()), nil
}

func documentXMLText(data []byte, maxBytes int) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var text strings.Builder
	paragraphDepth := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("decode document XML: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "p", "h":
				if paragraphDepth == 0 {
					appendLineBreak(&text)
				}
				paragraphDepth++
			case "tab":
				if paragraphDepth > 0 {
					text.WriteByte('\t')
				}
			}
		case xml.EndElement:
			if (value.Name.Local == "p" || value.Name.Local == "h") && paragraphDepth > 0 {
				paragraphDepth--
				if paragraphDepth == 0 {
					appendLineBreak(&text)
				}
			}
		case xml.CharData:
			if paragraphDepth > 0 {
				text.Write([]byte(value))
			}
		}
		if text.Len() > maxBytes {
			return "", errors.New("document text exceeds the extraction limit")
		}
	}
	return strings.TrimSpace(text.String()), nil
}

func extractXLSX(contents []byte, maxBytes int) (string, error) {
	reader, err := documentArchive(contents, maxBytes)
	if err != nil {
		return "", err
	}
	entries := make(map[string]*zip.File, len(reader.File))
	var sheets []string
	for _, entry := range reader.File {
		entries[entry.Name] = entry
		if strings.HasPrefix(entry.Name, "xl/worksheets/") && strings.HasSuffix(entry.Name, ".xml") {
			sheets = append(sheets, entry.Name)
		}
	}
	if len(sheets) == 0 {
		return "", errors.New("spreadsheet archive contains no worksheets")
	}
	if len(sheets) > maxSpreadsheetSheets {
		return "", fmt.Errorf("spreadsheet has more than %d worksheets", maxSpreadsheetSheets)
	}
	shared := []string(nil)
	if entry := entries["xl/sharedStrings.xml"]; entry != nil {
		data, err := readZipFile(entry, maxBytes)
		if err != nil {
			return "", err
		}
		shared, err = sharedStrings(data, maxBytes)
		if err != nil {
			return "", err
		}
	}
	sort.Strings(sheets)
	var text strings.Builder
	for _, name := range sheets {
		data, err := readZipFile(entries[name], maxBytes)
		if err != nil {
			return "", err
		}
		if text.Len() > 0 {
			text.WriteByte('\n')
		}
		fmt.Fprintf(&text, "[sheet %s]\n", strings.TrimSuffix(filepath.Base(name), ".xml"))
		remaining := maxBytes - text.Len()
		if remaining < 1 {
			return "", errors.New("spreadsheet text exceeds the extraction limit")
		}
		rows, err := worksheetCSV(data, shared, remaining)
		if err != nil {
			return "", err
		}
		text.WriteString(rows)
		if text.Len() > maxBytes {
			return "", errors.New("spreadsheet text exceeds the extraction limit")
		}
	}
	return strings.TrimSpace(text.String()), nil
}

func documentArchive(contents []byte, maxBytes int) (*zip.Reader, error) {
	reader, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		return nil, fmt.Errorf("open document archive: %w", err)
	}
	if len(reader.File) > maxArchiveEntries {
		return nil, fmt.Errorf("document archive has more than %d entries", maxArchiveEntries)
	}
	maxTotalBytes := uint64(maxBytes) * 4
	var totalBytes uint64
	seen := make(map[string]struct{}, len(reader.File))
	for _, entry := range reader.File {
		if !safeArchivePath(entry.Name) {
			return nil, fmt.Errorf("document archive contains unsafe entry %q", entry.Name)
		}
		if _, duplicate := seen[entry.Name]; duplicate {
			return nil, fmt.Errorf("document archive contains duplicate entry %q", entry.Name)
		}
		seen[entry.Name] = struct{}{}
		if entry.UncompressedSize64 > uint64(maxBytes) || totalBytes > maxTotalBytes-entry.UncompressedSize64 {
			return nil, errors.New("document archive exceeds the extraction limit")
		}
		totalBytes += entry.UncompressedSize64
	}
	return reader, nil
}

func safeArchivePath(name string) bool {
	return name != "" && !strings.Contains(name, "\\") && filepath.IsLocal(name)
}

func readZipFile(entry *zip.File, maxBytes int) ([]byte, error) {
	if entry.UncompressedSize64 > uint64(maxBytes) {
		return nil, errors.New("document text exceeds the extraction limit")
	}
	stream, err := entry.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", entry.Name, err)
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, int64(maxBytes)+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", entry.Name, err)
	}
	if len(data) > maxBytes {
		return nil, errors.New("document text exceeds the extraction limit")
	}
	return data, nil
}

func sharedStrings(data []byte, maxBytes int) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var values []string
	var current strings.Builder
	inItem, inText := false, 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode shared strings: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "si":
				if len(values) == maxSpreadsheetStrings {
					return nil, fmt.Errorf("spreadsheet has more than %d shared strings", maxSpreadsheetStrings)
				}
				inItem = true
				current.Reset()
			case "t":
				if inItem {
					inText++
				}
			}
		case xml.EndElement:
			switch value.Name.Local {
			case "t":
				if inText > 0 {
					inText--
				}
			case "si":
				if inItem {
					values = append(values, current.String())
					inItem = false
				}
			}
		case xml.CharData:
			if inText > 0 {
				current.Write([]byte(value))
			}
		}
		if current.Len() > maxBytes {
			return nil, errors.New("spreadsheet text exceeds the extraction limit")
		}
	}
	return values, nil
}

func worksheetCSV(data []byte, shared []string, maxBytes int) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var output strings.Builder
	row := []string(nil)
	rowCount := 0
	cellType, value := "", ""
	inCell, inValue, inInline := false, 0, 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("decode worksheet: %w", err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			switch token.Name.Local {
			case "row":
				row = nil
			case "c":
				inCell, cellType, value = true, "", ""
				for _, attribute := range token.Attr {
					if attribute.Name.Local == "t" {
						cellType = attribute.Value
					}
				}
			case "v":
				if inCell {
					inValue++
				}
			case "t":
				if inCell && cellType == "inlineStr" {
					inInline++
				}
			}
		case xml.EndElement:
			switch token.Name.Local {
			case "v":
				if inValue > 0 {
					inValue--
				}
			case "t":
				if inInline > 0 {
					inInline--
				}
			case "c":
				if inCell {
					if cellType == "s" {
						index := -1
						if _, err := fmt.Sscan(value, &index); err != nil || index < 0 || index >= len(shared) {
							return "", errors.New("spreadsheet has an invalid shared-string reference")
						}
						value = shared[index]
					}
					row = append(row, value)
					inCell = false
				}
			case "row":
				rowCount++
				if rowCount > maxSpreadsheetRows {
					return "", fmt.Errorf("spreadsheet has more than %d rows per worksheet", maxSpreadsheetRows)
				}
				if err := writeCSVRow(&output, row); err != nil {
					return "", err
				}
			}
		case xml.CharData:
			if inValue > 0 || inInline > 0 {
				value += string(token)
			}
		}
		if output.Len()+len(value) > maxBytes {
			return "", errors.New("spreadsheet text exceeds the extraction limit")
		}
	}
	return output.String(), nil
}

func writeCSVRow(output *strings.Builder, row []string) error {
	if len(row) == 0 {
		return nil
	}
	var line bytes.Buffer
	writer := csv.NewWriter(&line)
	if err := writer.Write(row); err != nil {
		return fmt.Errorf("encode worksheet row: %w", err)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("encode worksheet row: %w", err)
	}
	output.Write(line.Bytes())
	return nil
}

func appendLineBreak(text *strings.Builder) {
	if text.Len() > 0 && !strings.HasSuffix(text.String(), "\n") {
		text.WriteByte('\n')
	}
}
