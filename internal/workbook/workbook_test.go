package workbook

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strconv"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestRenderXLSXWithTypedCellsAndFormula(t *testing.T) {
	spec := Spec{Version: 1, Title: "Revenue", Sheets: []Sheet{{Name: "Summary", Columns: []Column{{Header: "Region"}, {Header: "Revenue", Format: "$#,##0"}}, Rows: [][]Cell{{{Value: "APAC"}, {Value: 12}}, {{Value: "Total"}, {Formula: "SUM(B2:B2)"}}}, FreezeRows: 1, Table: true}}}
	contents, preview, err := RenderXLSX(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	if err != nil || len(archive.File) == 0 || preview.Sheets[0].Formulas != 1 {
		t.Fatalf("xlsx files=%d preview=%#v err=%v", len(archive.File), preview, err)
	}
}

func TestRenderXLSXStreamsLargeNewSheet(t *testing.T) {
	rows := make([][]Cell, 10_001)
	for index := range rows {
		rows[index] = []Cell{{Value: index}}
	}
	contents, preview, err := RenderXLSX(Spec{Sheets: []Sheet{{Name: "Data", Columns: []Column{{Header: "Index", Format: "0"}}, Rows: rows, FreezeRows: 1, Filter: true}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Sheets[0].Streamed {
		t.Fatalf("preview = %#v", preview)
	}
	file, err := excelize.OpenReader(bytes.NewReader(contents))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	value, err := file.GetCellValue("Data", "A10002")
	if err != nil || value != "10000" {
		t.Fatalf("last streamed value = %q, %v", value, err)
	}
}

func TestTemplateFormulaDoesNotReuseOldValueAndRequiresRecalculation(t *testing.T) {
	template := excelize.NewFile()
	defer template.Close()
	if err := template.SetSheetName("Sheet1", "Summary"); err != nil {
		t.Fatal(err)
	}
	if err := template.SetCellValue("Summary", "B4", 999); err != nil {
		t.Fatal(err)
	}
	if _, err := template.NewSheet("Dashboard"); err != nil {
		t.Fatal(err)
	}
	if err := template.SetCellValue("Dashboard", "A1", 1998); err != nil {
		t.Fatal(err)
	}
	if err := template.SetCellFormula("Dashboard", "A1", "Summary!B4*2"); err != nil {
		t.Fatal(err)
	}
	manual, disabled := "manual", false
	if err := template.SetCalcProps(&excelize.CalcPropsOptions{CalcMode: &manual, FullCalcOnLoad: &disabled}); err != nil {
		t.Fatal(err)
	}
	seed, err := template.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	spec := Spec{Sheets: []Sheet{{Name: "Summary", Columns: []Column{{Header: "Region"}, {Header: "Amount"}}, Rows: [][]Cell{
		{{Value: "APAC"}, {Value: 120.1}}, {{Value: "EMEA"}, {Value: 79.9}}, {{Value: "Total"}, {Formula: "SUM(B2:B3)"}},
	}}}}
	contents, preview, err := RenderXLSX(spec, bytes.NewReader(seed.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	file, err := excelize.OpenReader(bytes.NewReader(contents))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if len(preview.Warnings) == 0 {
		t.Fatal("preview did not disclose uncalculated results")
	}
	value, err := file.GetCellValue("Summary", "B4", excelize.Options{RawCellValue: true})
	if err != nil || value != "" {
		t.Fatalf("unrecalculated total must not reuse the template's 999: %q, %v", value, err)
	}
	calculated, err := file.CalcCellValue("Summary", "B4")
	if err != nil || calculated != "200" {
		t.Fatalf("independent expected total 200, got %q, %v", calculated, err)
	}
	dependent, err := file.GetCellValue("Dashboard", "A1", excelize.Options{RawCellValue: true})
	if err != nil || dependent != "" {
		t.Fatalf("dependent template cache survived: %q, %v", dependent, err)
	}
	dependent, err = file.CalcCellValue("Dashboard", "A1")
	if err != nil || dependent != "400" {
		t.Fatalf("independent expected dashboard 400, got %q, %v", dependent, err)
	}
	props, err := file.GetCalcProps()
	if err != nil || props.CalcMode == nil || *props.CalcMode != "auto" || props.FullCalcOnLoad == nil || !*props.FullCalcOnLoad || props.ForceFullCalc == nil || !*props.ForceFullCalc {
		t.Fatalf("derived workbook must request a full recalculation: %#v, %v", props, err)
	}
}

func TestFormulaIgnoresUnverifiedCachedValueInBothWriters(t *testing.T) {
	for _, count := range []int{2, 10_001} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			rows := make([][]Cell, count)
			for index := range rows {
				rows[index] = []Cell{{Value: 1}}
			}
			rows[count-1] = []Cell{{Value: 999, Formula: fmt.Sprintf("SUM(A2:A%d)", count)}}
			contents, _, err := RenderXLSX(Spec{Sheets: []Sheet{{Name: "Data", Columns: []Column{{Header: "Amount"}}, Rows: rows}}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			file, err := excelize.OpenReader(bytes.NewReader(contents))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			cell := fmt.Sprintf("A%d", count+1)
			value, err := file.GetCellValue("Data", cell, excelize.Options{RawCellValue: true})
			if err != nil || value != "" {
				t.Fatalf("unverified cache leaked: %q, %v", value, err)
			}
			calculated, err := file.CalcCellValue("Data", cell)
			if err != nil || calculated != strconv.Itoa(count-1) {
				t.Fatalf("expected %d from actual inputs, got %q, %v", count-1, calculated, err)
			}
		})
	}
}
