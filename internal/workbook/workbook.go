// Package workbook owns Gator's semantic spreadsheet format and XLSX renderer.
package workbook

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

const Version = 1

type Spec struct {
	Version int     `json:"version"`
	Title   string  `json:"title,omitempty"`
	Theme   string  `json:"theme,omitempty"`
	Sheets  []Sheet `json:"sheets"`
}

type Sheet struct {
	Name       string   `json:"name"`
	Columns    []Column `json:"columns"`
	Rows       [][]Cell `json:"rows"`
	FreezeRows int      `json:"freeze_rows,omitempty"`
	Filter     bool     `json:"filter,omitempty"`
	Table      bool     `json:"table,omitempty"`
	Charts     []Chart  `json:"charts,omitempty"`
}

type Column struct {
	Header string  `json:"header"`
	Width  float64 `json:"width,omitempty"`
	Format string  `json:"format,omitempty"`
}

type Cell struct {
	Value   any    `json:"value,omitempty"`
	Formula string `json:"formula,omitempty"`
}

type Chart struct {
	Type       string `json:"type"`
	Title      string `json:"title,omitempty"`
	Anchor     string `json:"anchor"`
	Name       string `json:"name,omitempty"`
	Categories string `json:"categories"`
	Values     string `json:"values"`
}

type Preview struct {
	Title  string         `json:"title,omitempty"`
	Theme  string         `json:"theme"`
	Sheets []SheetPreview `json:"sheets"`
}

type SheetPreview struct {
	Name     string   `json:"name"`
	Rows     int      `json:"rows"`
	Columns  int      `json:"columns"`
	Formulas int      `json:"formulas"`
	Charts   []string `json:"charts,omitempty"`
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
	if s.Version != Version || len(s.Sheets) == 0 || len(s.Sheets) > 64 {
		return errors.New("workbook requires 1-64 sheets")
	}
	if s.Theme != "professional" && s.Theme != "minimal" && s.Theme != "report" {
		return fmt.Errorf("unsupported workbook theme %q", s.Theme)
	}
	seen := make(map[string]struct{}, len(s.Sheets))
	for sheetIndex, sheet := range s.Sheets {
		if strings.TrimSpace(sheet.Name) != sheet.Name || sheet.Name == "" || len([]rune(sheet.Name)) > 31 || strings.ContainsAny(sheet.Name, `[]:*?/\`) {
			return fmt.Errorf("sheet %d has an invalid name", sheetIndex+1)
		}
		if _, duplicate := seen[strings.ToLower(sheet.Name)]; duplicate {
			return fmt.Errorf("sheet name %q is repeated", sheet.Name)
		}
		seen[strings.ToLower(sheet.Name)] = struct{}{}
		if len(sheet.Columns) == 0 || len(sheet.Columns) > 256 || len(sheet.Rows) > 100_000 || sheet.FreezeRows < 0 || sheet.FreezeRows > len(sheet.Rows)+1 {
			return fmt.Errorf("sheet %q dimensions are invalid", sheet.Name)
		}
		for _, row := range sheet.Rows {
			if len(row) != len(sheet.Columns) {
				return fmt.Errorf("sheet %q contains a ragged row", sheet.Name)
			}
		}
		for _, column := range sheet.Columns {
			if strings.TrimSpace(column.Header) == "" || column.Width < 0 || column.Width > 255 || len(column.Format) > 128 {
				return fmt.Errorf("sheet %q has an invalid column", sheet.Name)
			}
		}
		for _, chart := range sheet.Charts {
			if chartType(chart.Type) == nil || strings.TrimSpace(chart.Anchor) == "" || strings.TrimSpace(chart.Values) == "" || strings.TrimSpace(chart.Categories) == "" {
				return fmt.Errorf("sheet %q has an invalid chart", sheet.Name)
			}
		}
	}
	return nil
}

func (s Spec) Preview() Preview {
	s = s.Normalize()
	preview := Preview{Title: s.Title, Theme: s.Theme}
	for _, sheet := range s.Sheets {
		item := SheetPreview{Name: sheet.Name, Rows: len(sheet.Rows), Columns: len(sheet.Columns)}
		for _, row := range sheet.Rows {
			for _, cell := range row {
				if cell.Formula != "" {
					item.Formulas++
				}
			}
		}
		for _, chart := range sheet.Charts {
			item.Charts = append(item.Charts, chart.Type+":"+chart.Title)
		}
		preview.Sheets = append(preview.Sheets, item)
	}
	return preview
}

// RenderXLSX renders a new workbook, or adds/replaces named sheets in an optional template.
func RenderXLSX(spec Spec, template io.Reader) ([]byte, Preview, error) {
	spec = spec.Normalize()
	if err := spec.Validate(); err != nil {
		return nil, Preview{}, err
	}
	var file *excelize.File
	var err error
	if template != nil {
		payload, readErr := io.ReadAll(io.LimitReader(template, 64*1024*1024+1))
		if readErr != nil || len(payload) > 64*1024*1024 {
			return nil, Preview{}, errors.New("XLSX template is unreadable or exceeds 64 MiB")
		}
		if unsafeTemplate(payload) {
			return nil, Preview{}, errors.New("macro-enabled or externally linked XLSX templates are not allowed")
		}
		file, err = excelize.OpenReader(bytes.NewReader(payload))
	} else {
		file = excelize.NewFile()
	}
	if err != nil {
		return nil, Preview{}, fmt.Errorf("open workbook template: %w", err)
	}
	defer file.Close()
	headerStyle, err := file.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"17365D"}, Pattern: 1}, Alignment: &excelize.Alignment{Vertical: "center"}})
	if err != nil {
		return nil, Preview{}, err
	}
	usedDefault := false
	for sheetIndex, sheet := range spec.Sheets {
		if index, err := file.GetSheetIndex(sheet.Name); err != nil || index == -1 {
			if sheetIndex == 0 && template == nil {
				if err := file.SetSheetName("Sheet1", sheet.Name); err != nil {
					return nil, Preview{}, err
				}
				usedDefault = true
			} else if _, err := file.NewSheet(sheet.Name); err != nil {
				return nil, Preview{}, err
			}
		}
		for columnIndex, column := range sheet.Columns {
			cell, _ := excelize.CoordinatesToCellName(columnIndex+1, 1)
			if err := file.SetCellValue(sheet.Name, cell, column.Header); err != nil {
				return nil, Preview{}, err
			}
			name, _ := excelize.ColumnNumberToName(columnIndex + 1)
			width := column.Width
			if width == 0 {
				width = 16
			}
			if err := file.SetColWidth(sheet.Name, name, name, width); err != nil {
				return nil, Preview{}, err
			}
			if column.Format != "" {
				format := column.Format
				style, err := file.NewStyle(&excelize.Style{CustomNumFmt: &format})
				if err != nil {
					return nil, Preview{}, err
				}
				end, _ := excelize.CoordinatesToCellName(columnIndex+1, len(sheet.Rows)+1)
				if err := file.SetCellStyle(sheet.Name, cell, end, style); err != nil {
					return nil, Preview{}, err
				}
			}
		}
		lastHeader, _ := excelize.CoordinatesToCellName(len(sheet.Columns), 1)
		if err := file.SetCellStyle(sheet.Name, "A1", lastHeader, headerStyle); err != nil {
			return nil, Preview{}, err
		}
		for rowIndex, row := range sheet.Rows {
			for columnIndex, cellValue := range row {
				cell, _ := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+2)
				if cellValue.Formula != "" {
					if err := file.SetCellFormula(sheet.Name, cell, cellValue.Formula); err != nil {
						return nil, Preview{}, err
					}
				} else if err := file.SetCellValue(sheet.Name, cell, cellValue.Value); err != nil {
					return nil, Preview{}, err
				}
			}
		}
		if sheet.FreezeRows > 0 {
			topLeft, _ := excelize.CoordinatesToCellName(1, sheet.FreezeRows+1)
			if err := file.SetPanes(sheet.Name, &excelize.Panes{Freeze: true, YSplit: sheet.FreezeRows, TopLeftCell: topLeft, ActivePane: "bottomLeft"}); err != nil {
				return nil, Preview{}, err
			}
		}
		lastCell, _ := excelize.CoordinatesToCellName(len(sheet.Columns), len(sheet.Rows)+1)
		if sheet.Table && len(sheet.Rows) > 0 {
			showRows := true
			if err := file.AddTable(sheet.Name, &excelize.Table{Range: "A1:" + lastCell, Name: safeTableName(sheet.Name), StyleName: "TableStyleMedium2", ShowRowStripes: &showRows}); err != nil {
				return nil, Preview{}, err
			}
		} else if sheet.Filter {
			if err := file.AutoFilter(sheet.Name, "A1:"+lastCell, nil); err != nil {
				return nil, Preview{}, err
			}
		}
		for _, chart := range sheet.Charts {
			kind := chartType(chart.Type)
			seriesName := chart.Name
			if seriesName == "" {
				seriesName = sheet.Name + "!$A$1"
			}
			if err := file.AddChart(sheet.Name, chart.Anchor, &excelize.Chart{Type: *kind, Series: []excelize.ChartSeries{{Name: seriesName, Categories: chart.Categories, Values: chart.Values}}, Title: excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: chart.Title}}}}); err != nil {
				return nil, Preview{}, err
			}
		}
	}
	_ = usedDefault
	var output bytes.Buffer
	if _, err := file.WriteTo(&output); err != nil {
		return nil, Preview{}, err
	}
	return output.Bytes(), spec.Preview(), nil
}

func unsafeTemplate(payload []byte) bool {
	archive, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return true
	}
	for _, file := range archive.File {
		lower := strings.ToLower(file.Name)
		if strings.Contains(lower, "vbaproject") || strings.Contains(lower, "externallinks/") || strings.HasSuffix(lower, ".bin") {
			return true
		}
		if strings.HasSuffix(lower, ".rels") {
			reader, err := file.Open()
			if err != nil {
				return true
			}
			contents, _ := io.ReadAll(io.LimitReader(reader, 2*1024*1024))
			_ = reader.Close()
			if bytes.Contains(contents, []byte(`TargetMode="External"`)) {
				return true
			}
		}
	}
	return false
}

func chartType(value string) *excelize.ChartType {
	types := map[string]excelize.ChartType{"bar": excelize.Bar, "column": excelize.Col, "line": excelize.Line, "pie": excelize.Pie}
	result, found := types[value]
	if !found {
		return nil
	}
	return &result
}

func safeTableName(value string) string {
	var result strings.Builder
	result.WriteString("Gator_")
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' {
			result.WriteRune(character)
		} else {
			result.WriteRune('_')
		}
	}
	return result.String()
}
