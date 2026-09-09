package workbook

import (
	"archive/zip"
	"bytes"
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
