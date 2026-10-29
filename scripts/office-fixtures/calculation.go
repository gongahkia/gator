package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gongahkia/gator/internal/workbook"
	"github.com/xuri/excelize/v2"
)

type calculationCase struct {
	Formulas map[string]map[string]any `json:"formulas"`
	Values   map[string]map[string]any `json:"values,omitempty"`
}

func calculationFixtures(directory string) error {
	expectations := map[string]calculationCase{}
	for _, theme := range []string{"professional", "minimal", "report"} {
		expectations[theme+"-new.xlsx"] = calculationCase{Formulas: map[string]map[string]any{"Summary": {"B4": 200}}}
		expectations[theme+"-edited.xlsx"] = calculationCase{Formulas: map[string]map[string]any{"Summary": {"B4": 230}}}
	}
	expectations["host-roundtrip.xlsx"] = calculationCase{Formulas: map[string]map[string]any{"Summary": {"B4": 230}}}
	streamed := map[string]any{}
	for index := 0; index < 10_001; index++ {
		streamed[fmt.Sprintf("B%d", index+2)] = index * 2
	}
	expectations["streamed.xlsx"] = calculationCase{Formulas: map[string]map[string]any{"Data": streamed}}

	data := workbook.Sheet{Name: "Sales & costs", Columns: []workbook.Column{{Header: "Region"}, {Header: "Units"}, {Header: "Price", Format: "0.00"}, {Header: "Revenue", Format: "0.00"}}, Rows: [][]workbook.Cell{
		{{Value: "APAC"}, {Value: 3}, {Value: 12.5}, {Formula: "B2*C2"}},
		{{Value: "EMEA"}, {Value: 2}, {Value: 7.25}, {Formula: "B3*C3"}},
		{{Value: "APAC"}, {Value: -1}, {Value: 5}, {Formula: "B4*C4"}},
		{{Value: "AMER"}, {Value: 0}, {Value: 9}, {Formula: "B5*C5"}},
	}}
	summary := workbook.Sheet{Name: "Analysis", Columns: []workbook.Column{{Header: "Measure"}, {Header: "Result"}}, Rows: [][]workbook.Cell{
		{{Value: "Total revenue"}, {Formula: "SUM('Sales & costs'!D2:D5)"}},
		{{Value: "APAC revenue"}, {Formula: `SUMIF('Sales & costs'!A2:A5,"APAC",'Sales & costs'!D2:D5)`}},
		{{Value: "Positive APAC revenue"}, {Formula: `SUMIFS('Sales & costs'!D2:D5,'Sales & costs'!A2:A5,"APAC",'Sales & costs'!B2:B5,">0")`}},
		{{Value: "APAC rows"}, {Formula: `COUNTIF('Sales & costs'!A2:A5,"APAC")`}},
		{{Value: "Rounded mean"}, {Formula: "ROUND(AVERAGE('Sales & costs'!D2:D5),2)"}},
		{{Value: "Status"}, {Formula: `IF(B2>0,"ready","review")`}},
		{{Value: "Safe division"}, {Formula: "IFERROR(B2/0,0)"}},
		{{Value: "Lookup"}, {Formula: `VLOOKUP("EMEA",'Sales & costs'!A2:D5,4,FALSE)`}},
		{{Value: "Positive total"}, {Formula: "B2>0"}},
		{{Value: "Visible error"}, {Formula: "1/0"}},
		{{Value: "Literal formula-looking text"}, {Value: "=1+1"}},
	}}
	book, _, err := workbook.RenderXLSX(workbook.Spec{Sheets: []workbook.Sheet{data, summary}}, nil)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(directory, "calculation.xlsx"), book, 0600); err != nil {
		return err
	}
	expectations["calculation.xlsx"] = calculationCase{
		Formulas: map[string]map[string]any{
			"Sales & costs": {"D2": 37.5, "D3": 14.5, "D4": -5, "D5": 0},
			"Analysis":      {"B2": 47, "B3": 32.5, "B4": 37.5, "B5": 2, "B6": 11.75, "B7": "ready", "B8": 0, "B9": 14.5, "B10": true, "B11": map[string]string{"error": "#DIV/0!"}},
		}, Values: map[string]map[string]any{"Analysis": {"B12": "=1+1"}},
	}

	// Seed deliberately stale values and manual calculation in a template.
	// Rendering changes Summary and must invalidate the unedited dependent sheet.
	template := excelize.NewFile()
	defer template.Close()
	if err := template.SetSheetName("Sheet1", "Summary"); err != nil {
		return err
	}
	if err := template.SetCellValue("Summary", "B4", 999); err != nil {
		return err
	}
	if _, err := template.NewSheet("Dashboard"); err != nil {
		return err
	}
	if err := template.SetCellValue("Dashboard", "A1", 1998); err != nil {
		return err
	}
	if err := template.SetCellFormula("Dashboard", "A1", "Summary!B4*2"); err != nil {
		return err
	}
	manual, disabled := "manual", false
	if err := template.SetCalcProps(&excelize.CalcPropsOptions{CalcMode: &manual, FullCalcOnLoad: &disabled}); err != nil {
		return err
	}
	seed, err := template.WriteToBuffer()
	if err != nil {
		return err
	}
	filled, _, err := workbook.RenderXLSX(workbook.Spec{Sheets: []workbook.Sheet{{Name: "Summary", Columns: []workbook.Column{{Header: "Region"}, {Header: "Amount"}}, Rows: [][]workbook.Cell{
		{{Value: "APAC"}, {Value: 120.1}}, {{Value: "EMEA"}, {Value: 79.9}}, {{Value: "Total"}, {Formula: "SUM(B2:B3)"}},
	}}}}, bytes.NewReader(seed.Bytes()))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(directory, "stale-template.xlsx"), filled, 0600); err != nil {
		return err
	}
	expectations["stale-template.xlsx"] = calculationCase{Formulas: map[string]map[string]any{"Summary": {"B4": 200}, "Dashboard": {"A1": 400}}}
	payload, err := json.MarshalIndent(expectations, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "calculation-expectations.json"), append(payload, '\n'), 0600)
}
