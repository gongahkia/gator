package attachment

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pdf "github.com/Detective-XH/gopdf"
)

const (
	// MaxPDFPages prevents a valid but hostile page tree from monopolizing a
	// Work turn. Large documents should be split before ingestion.
	MaxPDFPages = 2_000
	// MaxPDFExtractedBytes bounds provider-visible page text and page markers.
	MaxPDFExtractedBytes = 512 * 1024
	maxPDFWarnings       = 128
)

// PDFExtraction is a bounded, page-addressable representation of a PDF text
// layer. RequiresOCR is evidence that some pages contain images but no usable
// text; Gator does not pretend that native extraction is OCR.
type PDFExtraction struct {
	PageCount   int          `json:"page_count"`
	Pages       []PDFPage    `json:"pages"`
	Warnings    []PDFWarning `json:"warnings,omitempty"`
	RequiresOCR bool         `json:"requires_ocr,omitempty"`
}

type PDFPage struct {
	Page          int      `json:"page"`
	Text          string   `json:"text,omitempty"`
	HasText       bool     `json:"has_text"`
	ImageCount    int      `json:"image_count,omitempty"`
	ImageCoverage float64  `json:"image_coverage,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

type PDFWarning struct {
	Page    int    `json:"page,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// PlainText renders page-addressable extraction for a text-only model while
// preserving explicit OCR/review warnings.
func (e PDFExtraction) PlainText(maxBytes int) (string, error) {
	var text strings.Builder
	fmt.Fprintf(&text, "[PDF: %d page(s)", e.PageCount)
	if e.RequiresOCR {
		text.WriteString("; one or more pages require OCR/review")
	}
	text.WriteString("]\n")
	for _, page := range e.Pages {
		fmt.Fprintf(&text, "\n[PDF page %d]\n", page.Page)
		if page.Text == "" {
			text.WriteString("[No extractable text")
			if page.ImageCount > 0 {
				text.WriteString("; image content requires OCR")
			}
			text.WriteString("]\n")
		} else {
			text.WriteString(page.Text)
			text.WriteByte('\n')
		}
		if text.Len() > maxBytes {
			return "", fmt.Errorf("PDF text exceeds the %d KiB extraction limit", maxBytes/1024)
		}
	}
	return strings.TrimSpace(text.String()), nil
}

// ExtractPDFText parses a bounded in-memory PDF without network, rendering, or
// subprocesses. Text is returned per physical 1-based page so callers can cite
// page locations. Image-only and sparse pages are flagged for OCR/review.
func ExtractPDFText(ctx context.Context, contents []byte, maxBytes int) (result PDFExtraction, err error) {
	if len(contents) == 0 || !strings.HasPrefix(string(contents[:min(len(contents), 5)]), "%PDF-") {
		return PDFExtraction{}, errors.New("document is not a PDF")
	}
	if maxBytes < 1 {
		return PDFExtraction{}, errors.New("PDF extraction limit must be positive")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			result = PDFExtraction{}
			err = fmt.Errorf("parse PDF: %v", recovered)
		}
	}()
	reader, err := pdf.OpenBytes(contents)
	if err != nil {
		return PDFExtraction{}, fmt.Errorf("open PDF: %w", err)
	}
	result.PageCount = reader.NumPage()
	if result.PageCount < 1 {
		return PDFExtraction{}, errors.New("PDF contains no pages")
	}
	if result.PageCount > MaxPDFPages {
		return PDFExtraction{}, fmt.Errorf("PDF has %d pages; maximum is %d", result.PageCount, MaxPDFPages)
	}
	extractedBytes := 0
	result.Pages = make([]PDFPage, 0, result.PageCount)
	for number := 1; number <= result.PageCount; number++ {
		if err := ctx.Err(); err != nil {
			return PDFExtraction{}, err
		}
		page := reader.Page(number)
		text, err := page.GetPlainText(nil)
		if err != nil {
			return PDFExtraction{}, fmt.Errorf("extract PDF page %d: %w", number, err)
		}
		text = strings.TrimSpace(text)
		summary, err := page.ExtractionSummary()
		if err != nil {
			return PDFExtraction{}, fmt.Errorf("inspect PDF page %d: %w", number, err)
		}
		item := PDFPage{
			Page: number, Text: text, HasText: text != "",
			ImageCount: summary.ImageCount, ImageCoverage: summary.ImageCoverage,
		}
		for _, warning := range summary.Warnings {
			item.Warnings = append(item.Warnings, string(warning.Code))
			if warning.Code == pdf.WarningImageOnlyPage || warning.Code == pdf.WarningSparseText {
				result.RequiresOCR = true
			}
		}
		extractedBytes += len(text) + 32
		if extractedBytes > maxBytes {
			return PDFExtraction{}, fmt.Errorf("PDF text exceeds the %d KiB extraction limit", maxBytes/1024)
		}
		result.Pages = append(result.Pages, item)
	}
	for _, warning := range reader.Warnings() {
		if len(result.Warnings) == maxPDFWarnings {
			result.Warnings = append(result.Warnings, PDFWarning{Code: "warnings_truncated", Message: "additional PDF extraction warnings omitted"})
			break
		}
		result.Warnings = append(result.Warnings, PDFWarning{
			Page: warning.Page, Code: string(warning.Code),
			Message: warning.Message, Detail: warning.Detail,
		})
		if warning.Code == pdf.WarningImageOnlyPage || warning.Code == pdf.WarningSparseText {
			result.RequiresOCR = true
		}
	}
	return result, nil
}
