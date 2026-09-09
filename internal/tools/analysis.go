package tools

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/attachment"
	"github.com/gongahkia/gator/internal/workbook"
	"github.com/gongahkia/gator/internal/workspace"
	"github.com/xuri/excelize/v2"
)

type TableSource struct {
	Path  string `json:"path"`
	Sheet string `json:"sheet,omitempty"`
}
type Table struct {
	Source TableSource `json:"source"`
	SHA256 string      `json:"sha256"`
	Rows   [][]string  `json:"rows"`
}

func ReadTable(root workspace.Root, source TableSource) (Table, error) {
	source.Path = strings.TrimPrefix(source.Path, "source/")
	data, err := root.ReadRegularFile(source.Path, 4*1024*1024)
	if err != nil {
		return Table{}, err
	}
	return decodeTable(source, data)
}

func decodeTable(source TableSource, data []byte) (Table, error) {
	sum := sha256.Sum256(data)
	table := Table{Source: source, SHA256: hex.EncodeToString(sum[:])}
	switch strings.ToLower(filepath.Ext(source.Path)) {
	case ".csv":
		reader := csv.NewReader(bytes.NewReader(data))
		for {
			row, err := reader.Read()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return table, err
			}
			table.Rows = append(table.Rows, row)
			if len(table.Rows) > 10001 {
				return table, errors.New("table row limit exceeded")
			}
		}
	case ".xlsx":
		file, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: 16 * 1024 * 1024, UnzipXMLSizeLimit: 4 * 1024 * 1024})
		if err != nil {
			return table, err
		}
		defer file.Close()
		sheet := source.Sheet
		if sheet == "" {
			sheets := file.GetSheetList()
			if len(sheets) != 1 {
				return table, errors.New("select a sheet explicitly for multi-sheet input")
			}
			sheet = sheets[0]
		}
		table.Source.Sheet = sheet
		rows, err := file.Rows(sheet)
		if err != nil {
			return table, err
		}
		defer rows.Close()
		for rows.Next() {
			row, err := rows.Columns()
			if err != nil {
				return table, err
			}
			table.Rows = append(table.Rows, row)
			if len(table.Rows) > 10001 {
				return table, errors.New("table row limit exceeded")
			}
		}
		if err := rows.Error(); err != nil {
			return table, err
		}
	default:
		return table, errors.New("table input must be CSV or XLSX")
	}
	if len(table.Rows) == 0 || len(table.Rows[0]) == 0 || len(table.Rows[0]) > 256 {
		return table, errors.New("table header missing or too wide")
	}
	seen := map[string]bool{}
	for _, name := range table.Rows[0] {
		if strings.TrimSpace(name) == "" || seen[name] {
			return table, errors.New("table headers must be nonempty and unique")
		}
		seen[name] = true
	}
	for i, row := range table.Rows {
		if len(row) > len(table.Rows[0]) {
			return table, fmt.Errorf("row %d is wider than its header", i+1)
		}
		for len(row) < len(table.Rows[0]) {
			row = append(row, "")
		}
		table.Rows[i] = row
	}
	return table, nil
}

type InspectTable struct {
	Root       workspace.Root
	NamedRoots []workspace.NamedRoot
}

func (t InspectTable) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "inspect_table", Description: "Read a bounded CSV or selected XLSX sheet with exact row/cell coordinates and a content digest. Missing cells remain empty.", Parameters: schema(`{"type":"object","additionalProperties":false,"required":["path"],"properties":{"path":{"type":"string"},"sheet":{"type":"string"}}}`)}
}
func (t InspectTable) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var source TableSource
	if err := decodeArguments(raw, &source); err != nil {
		return agent.ToolResult{}, err
	}
	roots, err := workspace.NewNamedRootSet(t.Root, nil, t.NamedRoots)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if len(t.NamedRoots) == 0 {
		source.Path = strings.TrimPrefix(source.Path, "source/")
	}
	payload, display, err := roots.ReadRegularFile(source.Path, 4*1024*1024)
	if err != nil {
		return agent.ToolResult{}, err
	}
	source.Path = display
	table, err := decodeTable(source, payload)
	if err != nil {
		return agent.ToolResult{}, err
	}
	data, err := json.Marshal(table)
	if len(data) > 512*1024 {
		return agent.ToolResult{}, errors.New("table result too large; select a smaller source")
	}
	return agent.ToolResult{Content: string(data)}, err
}

type ExtractDocument struct{ Root workspace.Root }

func (t ExtractDocument) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "extract_document", Description: "Extract supported frozen source documents on demand. Returns path-level provenance; exact PDF pages are unavailable. Unsupported extraction is an error.", Parameters: schema(`{"type":"object","additionalProperties":false,"required":["path"],"properties":{"path":{"type":"string"}}}`)}
}
func (t ExtractDocument) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := decodeArguments(raw, &input); err != nil {
		return agent.ToolResult{}, err
	}
	path := strings.TrimPrefix(input.Path, "source/")
	document, supported, err := attachment.Load(t.Root, path, attachment.MaxDocumentBytes)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if !supported {
		return agent.ToolResult{}, errors.New("unsupported document extraction format")
	}
	if document.MediaType == attachment.PDFMediaType {
		return agent.ToolResult{}, errors.New("PDF text extraction unavailable; explicitly attach the PDF to a capable model")
	}
	sum := sha256.Sum256(document.Data)
	data, err := json.Marshal(map[string]any{"path": path, "locator_precision": "path", "extraction_sha256": hex.EncodeToString(sum[:]), "text": string(document.Data)})
	return agent.ToolResult{Content: string(data)}, err
}

type ReconcileTables struct {
	Source, Output workspace.Root
	Contract       artifact.Contract
}

func (t ReconcileTables) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "reconcile_tables", Description: "Reconcile CSV/XLSX rows by an exact key and decimal amount. Writes a checked XLSX plus explanatory memo with source row references, duplicates, missing values and discrepancies. Decimal values are retained exactly as text.", Parameters: schema(`{"type":"object","additionalProperties":false,"required":["left","right","key","amount","workbook","memo"],"properties":{"left":{"type":"object"},"right":{"type":"object"},"key":{"type":"string"},"amount":{"type":"string"},"workbook":{"type":"string"},"memo":{"type":"string"}}}`)}
}
func (t ReconcileTables) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var input struct {
		Left, Right                 TableSource
		Key, Amount, Workbook, Memo string
	}
	if err := decodeArguments(raw, &input); err != nil {
		return agent.ToolResult{}, err
	}
	left, err := ReadTable(t.Source, input.Left)
	if err != nil {
		return agent.ToolResult{}, err
	}
	right, err := ReadTable(t.Source, input.Right)
	if err != nil {
		return agent.ToolResult{}, err
	}
	type item struct {
		total   *big.Rat
		refs    []string
		invalid bool
	}
	sides := []map[string]*item{{}, {}}
	totals := []*big.Rat{new(big.Rat), new(big.Rat)}
	var issues []string
	for side, table := range []Table{left, right} {
		keyCol, amountCol := -1, -1
		for i, name := range table.Rows[0] {
			if name == input.Key {
				keyCol = i
			}
			if name == input.Amount {
				amountCol = i
			}
		}
		if keyCol < 0 || amountCol < 0 {
			return agent.ToolResult{}, errors.New("required key or amount column missing")
		}
		for i, row := range table.Rows[1:] {
			locator := fmt.Sprintf("source/%s", table.Source.Path)
			if table.Source.Sheet != "" {
				locator += "#" + table.Source.Sheet
			}
			locator += fmt.Sprintf(":row:%d", i+2)
			key := row[keyCol]
			if key == "" {
				issues = append(issues, locator+" missing key")
				continue
			}
			record := sides[side][key]
			if record == nil {
				record = &item{total: new(big.Rat)}
				sides[side][key] = record
			}
			record.refs = append(record.refs, locator)
			value, ok := new(big.Rat).SetString(row[amountCol])
			if !ok || strings.ContainsAny(row[amountCol], "/eE") {
				record.invalid = true
				issues = append(issues, locator+" missing or invalid decimal amount")
				continue
			}
			record.total.Add(record.total, value)
			totals[side].Add(totals[side], value)
		}
	}
	keys := map[string]bool{}
	for _, side := range sides {
		for key := range side {
			keys[key] = true
		}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	sheet := workbook.Sheet{Name: "Reconciliation", Columns: []workbook.Column{{Header: input.Key}, {Header: "Left"}, {Header: "Right"}, {Header: "Difference"}, {Header: "Status"}, {Header: "Source rows"}}, FreezeRows: 1, Filter: true}
	check := new(big.Rat)
	for _, key := range ordered {
		a, b := sides[0][key], sides[1][key]
		status := "matched"
		if a == nil {
			a = &item{total: new(big.Rat)}
			status = "missing left"
		}
		if b == nil {
			b = &item{total: new(big.Rat)}
			status = "missing right"
		}
		delta := new(big.Rat).Sub(a.total, b.total)
		check.Add(check, delta)
		if delta.Sign() != 0 && status == "matched" {
			status = "discrepancy"
		}
		if len(a.refs) > 1 || len(b.refs) > 1 {
			status += "; duplicate key"
		}
		if a.invalid || b.invalid {
			status += "; invalid amount"
		}
		sheet.Rows = append(sheet.Rows, []workbook.Cell{{Value: key}, {Value: a.total.RatString()}, {Value: b.total.RatString()}, {Value: delta.RatString()}, {Value: status}, {Value: strings.Join(append(a.refs, b.refs...), ", ")}})
	}
	expected := new(big.Rat).Sub(totals[0], totals[1])
	if check.Cmp(expected) != 0 {
		return agent.ToolResult{}, errors.New("independent reconciliation total check failed")
	}
	spec := workbook.Spec{Title: "Source reconciliation", Sheets: []workbook.Sheet{sheet, {Name: "Checks", Columns: []workbook.Column{{Header: "Check"}, {Header: "Value"}}, Rows: [][]workbook.Cell{{{Value: "Left total"}, {Value: totals[0].RatString()}}, {{Value: "Right total"}, {Value: totals[1].RatString()}}, {{Value: "Difference"}, {Value: expected.RatString()}}, {{Value: "Row differences equal source total difference"}, {Value: true}}}}}}
	data, _, err := workbook.RenderXLSX(spec, nil)
	if err != nil {
		return agent.ToolResult{}, err
	}
	// reopen the actual deliverable and independently read its checked total.
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return agent.ToolResult{}, err
	}
	actual, err := file.GetCellValue("Checks", "B4")
	file.Close()
	if err != nil || actual != expected.RatString() {
		return agent.ToolResult{}, errors.New("rendered workbook total does not match source calculation")
	}
	if err := artifact.WriteBinary(t.Output, t.Contract, input.Workbook, data); err != nil {
		return agent.ToolResult{}, err
	}
	memo := fmt.Sprintf("# Reconciliation\n\nLeft: source/%s (SHA-256 %s).\nRight: source/%s (SHA-256 %s).\n\nExact totals: left %s; right %s; difference %s.\nEach row difference was checked against independently accumulated source totals. Values use exact rational notation to avoid rounding.\n\nMissing and invalid amounts are excluded from totals and flagged; duplicate keys are aggregated and flagged. Source rows appear in the workbook.\n\n%s\n", left.Source.Path, left.SHA256, right.Source.Path, right.SHA256, totals[0].RatString(), totals[1].RatString(), expected.RatString(), strings.Join(issues, "\n"))
	if err := artifact.WriteText(t.Output, t.Contract, input.Memo, memo); err != nil {
		return agent.ToolResult{}, err
	}
	result, _ := json.Marshal(map[string]any{"workbook": input.Workbook, "memo": input.Memo, "checked": true, "issues": issues, "difference": expected.RatString()})
	return agent.ToolResult{Content: string(result)}, nil
}
