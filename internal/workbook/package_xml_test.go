package workbook

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestWorkbookNormalizationPreservesPartsAndChartContent(t *testing.T) {
	font := `<s:styleSheet xmlns:s="` + sheetNS + `"><s:fonts><s:font><s:name val="Calibri"/><s:color rgb="FF123456"/><s:sz val="11"/><s:b/></s:font></s:fonts></s:styleSheet>`
	chart := `<c:chartSpace xmlns:c="` + chartNS + `"><c:chart><c:plotArea><c:barChart><c:ser><c:idx val="0"/><c:order val="0"/><c:dLbls/><c:invertIfNegative val="0"/><c:cat><c:strRef><c:f>Summary!$A$2:$A$3</c:f></c:strRef></c:cat><c:val><c:numRef><c:f>Summary!$B$2:$B$3</c:f></c:numRef></c:val><c:smooth val="0"/></c:ser></c:barChart></c:plotArea></c:chart></c:chartSpace>`
	var input bytes.Buffer
	w := zip.NewWriter(&input)
	parts := map[string]string{"xl/styles.xml": font, "xl/charts/chart1.xml": chart, "xl/worksheets/sheet1.xml": `<sheet><formula>SUM(B2:B3)</formula></sheet>`, "xl/media/logo.bin": "untouched bytes\x00"}
	for name, data := range parts {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := normalizePackageXML(input.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(result), int64(len(result)))
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range archive.File {
		reader, _ := part.Open()
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		switch part.Name {
		case "xl/styles.xml":
			if !bytes.Contains(data, []byte(`<s:b/><s:sz val="11"/><s:color rgb="FF123456"/><s:name val="Calibri"/>`)) {
				t.Fatalf("font ordering: %s", data)
			}
		case "xl/charts/chart1.xml":
			if bytes.Contains(data, []byte("smooth")) || !bytes.Contains(data, []byte(`<c:invertIfNegative val="0"/><c:dLbls/>`)) {
				t.Fatalf("chart sequence: %s", data)
			}
			for _, reference := range []string{"Summary!$A$2:$A$3", "Summary!$B$2:$B$3"} {
				if !bytes.Contains(data, []byte(reference)) {
					t.Fatal("lost chart reference", reference)
				}
			}
		default:
			if string(data) != parts[part.Name] {
				t.Fatal("changed unrelated part", part.Name)
			}
		}
		if normalized, err := normalizeSequences(data); part.Name != "xl/media/logo.bin" && (err != nil || !bytes.Equal(data, normalized)) {
			t.Fatalf("normalization is not stable: %s, %v", part.Name, err)
		}
	}
}

func TestWorkbookNormalizationPreservesUnknownContentAndRejectsSemanticLoss(t *testing.T) {
	for _, inner := range []string{`<f:font><f:name val="X"/><!-- keep this --><f:b/></f:font>`, `<f:font><f:name val="X"/><f:future/></f:font>`} {
		data := []byte(`<f:fonts xmlns:f="` + sheetNS + `">` + inner + `</f:fonts>`)
		result, err := normalizeSequences(data)
		if err != nil || !bytes.Equal(data, result) {
			t.Fatalf("changed unfamiliar content: %s, %v", result, err)
		}
	}
	data := []byte(`<barChart xmlns="` + chartNS + `"><ser><idx val="0"/><order val="0"/><smooth val="1"/></ser></barChart>`)
	if _, err := normalizeSequences(data); err == nil || !strings.Contains(err.Error(), "non-default") {
		t.Fatalf("silently removed a meaningful property: %v", err)
	}
}
