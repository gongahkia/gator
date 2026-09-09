package workbook

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestRenderXLSXWithTypedCellsAndFormula(t *testing.T) {
	spec := Spec{Version: 1, Title: "Revenue", Sheets: []Sheet{{Name: "Summary", Columns: []Column{{Header: "Region"}, {Header: "Revenue", Format: "$#,##0"}}, Rows: [][]Cell{{{{Value: "APAC"}, {Value: 12}}}, {{{Value: "Total"}, {Formula: "SUM(B2:B2)"}}}}, FreezeRows: 1, Table: true}}}
	contents, preview, err := RenderXLSX(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	if err != nil || len(archive.File) == 0 || preview.Sheets[0].Formulas != 1 {
		t.Fatalf("xlsx files=%d preview=%#v err=%v", len(archive.File), preview, err)
	}
}
