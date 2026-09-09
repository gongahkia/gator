package artifact

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxPreviewBytes = 16 * 1024

// Preview is a bounded, terminal-safe representation of a verified artifact.
// Kind lets richer clients choose their own renderer without trusting a model
// to supply HTML or terminal control sequences.
type Preview struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Summary   string `json:"summary"`
	Content   string `json:"content,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// PreviewBundle creates media-aware previews only after bundle verification.
func PreviewBundle(bundle Bundle) ([]Preview, error) {
	if err := VerifyBundle(bundle); err != nil {
		return nil, fmt.Errorf("verify artifact bundle before preview: %w", err)
	}
	previews := make([]Preview, 0, len(bundle.Manifest.Artifacts))
	for _, file := range bundle.Manifest.Artifacts {
		contents, err := bundle.Output.ReadRegularFile(filepath.FromSlash(file.Path), bundle.Manifest.Contract.MaxArtifactBytes)
		if err != nil {
			return nil, fmt.Errorf("read artifact %q for preview: %w", file.Path, err)
		}
		previews = append(previews, previewFile(file, contents))
	}
	return previews, nil
}

func previewFile(file File, contents []byte) Preview {
	preview := Preview{Path: file.Path, Kind: "binary", Summary: fmt.Sprintf("%s, %d bytes", file.MediaType, file.Bytes)}
	switch file.MediaType {
	case "application/pdf":
		pages := bytes.Count(contents, []byte("/Type /Page")) - bytes.Count(contents, []byte("/Type /Pages"))
		preview.Kind = "document"
		preview.Summary = fmt.Sprintf("PDF document, %d pages, %d bytes", max(1, pages), file.Bytes)
		return preview
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		preview.Kind = "document"
		preview.Summary = fmt.Sprintf("DOCX document, %d bytes", file.Bytes)
		if part := zipPart(contents, "word/document.xml"); len(part) > 0 {
			text := strings.NewReplacer("</w:p>", "\n", "<w:tab/>", "\t").Replace(string(part))
			var plain strings.Builder
			inside := false
			for _, character := range text {
				if character == '<' {
					inside = true
					continue
				}
				if character == '>' {
					inside = false
					continue
				}
				if !inside {
					plain.WriteRune(character)
				}
			}
			preview.Content, preview.Truncated = boundedSafeText(plain.String())
		}
		return preview
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		preview.Kind = "workbook"
		sheets := 0
		if archive, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents))); err == nil {
			for _, part := range archive.File {
				if strings.HasPrefix(part.Name, "xl/worksheets/sheet") && strings.HasSuffix(part.Name, ".xml") {
					sheets++
				}
			}
		}
		preview.Summary = fmt.Sprintf("XLSX workbook, %d sheets, %d bytes", sheets, file.Bytes)
		return preview
	}
	if !utf8.Valid(contents) || bytes.IndexByte(contents, 0) >= 0 {
		return preview
	}
	switch file.MediaType {
	case "application/json":
		var formatted bytes.Buffer
		if err := json.Indent(&formatted, contents, "", "  "); err == nil {
			preview.Kind = "json"
			preview.Summary = fmt.Sprintf("JSON document, %d bytes", file.Bytes)
			preview.Content, preview.Truncated = boundedSafeText(formatted.String())
			return preview
		}
	case "text/csv":
		rows, err := csv.NewReader(bytes.NewReader(contents)).ReadAll()
		if err == nil {
			columns := 0
			if len(rows) > 0 {
				columns = len(rows[0])
			}
			preview.Kind = "table"
			preview.Summary = fmt.Sprintf("CSV table, %d data rows × %d columns", max(0, len(rows)-1), columns)
			var sample bytes.Buffer
			writer := csv.NewWriter(&sample)
			limit := min(len(rows), 11)
			writer.WriteAll(rows[:limit])
			preview.Content, preview.Truncated = boundedSafeText(sample.String())
			preview.Truncated = preview.Truncated || len(rows) > limit
			return preview
		}
	case "text/markdown":
		preview.Kind = "markdown"
		preview.Summary = fmt.Sprintf("Markdown document, %d bytes", file.Bytes)
		preview.Content, preview.Truncated = boundedSafeText(string(contents))
		return preview
	}
	if strings.HasPrefix(file.MediaType, "text/") || file.MediaType == "application/yaml" {
		preview.Kind = "text"
		preview.Summary = fmt.Sprintf("Text document, %d bytes", file.Bytes)
		preview.Content, preview.Truncated = boundedSafeText(string(contents))
	}
	return preview
}

func zipPart(contents []byte, name string) []byte {
	archive, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		return nil
	}
	for _, part := range archive.File {
		if part.Name == name {
			reader, err := part.Open()
			if err != nil {
				return nil
			}
			defer reader.Close()
			data, _ := io.ReadAll(io.LimitReader(reader, maxPreviewBytes*4))
			return data
		}
	}
	return nil
}

func boundedSafeText(value string) (string, bool) {
	var result strings.Builder
	result.Grow(min(len(value), maxPreviewBytes))
	truncated := false
	for _, character := range value {
		if character == '\r' {
			character = '\n'
		}
		if unicode.IsControl(character) && character != '\n' && character != '\t' {
			continue
		}
		bytesNeeded := utf8.RuneLen(character)
		if result.Len()+bytesNeeded > maxPreviewBytes {
			truncated = true
			break
		}
		result.WriteRune(character)
	}
	return result.String(), truncated
}
