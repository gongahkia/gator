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
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workspace"
)

const PDFMediaType = "application/pdf"

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
	resolved, err := root.ResolveFile(path)
	if err != nil {
		return nil, fmt.Errorf("resolve attachment @%s: %w", path, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("inspect attachment @%s: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Size() > int64(maxBytes) {
		return nil, fmt.Errorf("attachment @%s must be a regular file no larger than %d MiB", path, maxBytes/(1024*1024))
	}
	contents, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read attachment @%s: %w", path, err)
	}
	return contents, nil
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
	reader, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		return nil, fmt.Errorf("open document archive: %w", err)
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
	reader, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		return "", fmt.Errorf("open spreadsheet archive: %w", err)
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
