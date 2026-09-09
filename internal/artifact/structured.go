package artifact

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gongahkia/gator/internal/workspace"
)

const (
	maxTableColumns = 256
	maxTableRows    = 10_000
	maxTableCell    = 64 * 1024
)

// WriteJSON validates and formats one complete JSON value before staging it.
func WriteJSON(root workspace.Root, contract Contract, path string, value json.RawMessage) error {
	contract = contract.Normalize()
	if int64(len(value)) > contract.MaxArtifactBytes {
		return fmt.Errorf("JSON value exceeds the %d-byte artifact limit", contract.MaxArtifactBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decode JSON artifact: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("JSON artifact must contain exactly one value")
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, value, "", "  "); err != nil {
		return fmt.Errorf("format JSON artifact: %w", err)
	}
	formatted.WriteByte('\n')
	return WriteText(root, contract, path, formatted.String())
}

// WriteTable encodes a rectangular, bounded table as RFC 4180-compatible CSV.
// Headers are validated before any output file is changed.
func WriteTable(root workspace.Root, contract Contract, path string, headers []string, rows [][]string) error {
	contract = contract.Normalize()
	if err := contract.Validate(); err != nil {
		return fmt.Errorf("validate artifact contract: %w", err)
	}
	if len(headers) == 0 || len(headers) > maxTableColumns {
		return fmt.Errorf("table requires 1-%d columns", maxTableColumns)
	}
	if len(rows) > maxTableRows {
		return fmt.Errorf("table exceeds the %d-row limit", maxTableRows)
	}
	seen := make(map[string]struct{}, len(headers))
	var estimatedBytes int64
	for _, header := range headers {
		if strings.TrimSpace(header) != header || header == "" || len(header) > maxTableCell || strings.ContainsRune(header, 0) {
			return errors.New("table header is empty or invalid")
		}
		if _, duplicate := seen[header]; duplicate {
			return fmt.Errorf("table repeats header %q", header)
		}
		seen[header] = struct{}{}
		estimatedBytes += int64(2*len(header) + 3)
		if estimatedBytes > contract.MaxArtifactBytes {
			return fmt.Errorf("encoded table may exceed the %d-byte artifact limit", contract.MaxArtifactBytes)
		}
	}
	for rowIndex, row := range rows {
		if len(row) != len(headers) {
			return fmt.Errorf("table row %d has %d cells; expected %d", rowIndex+1, len(row), len(headers))
		}
		for _, cell := range row {
			if len(cell) > maxTableCell || strings.ContainsRune(cell, 0) {
				return fmt.Errorf("table row %d contains an invalid or oversized cell", rowIndex+1)
			}
			estimatedBytes += int64(2*len(cell) + 3)
			if estimatedBytes > contract.MaxArtifactBytes {
				return fmt.Errorf("encoded table may exceed the %d-byte artifact limit", contract.MaxArtifactBytes)
			}
		}
	}
	var encoded bytes.Buffer
	writer := csv.NewWriter(&encoded)
	if err := writer.Write(headers); err != nil {
		return err
	}
	writer.WriteAll(rows)
	if err := writer.Error(); err != nil {
		return fmt.Errorf("encode CSV artifact: %w", err)
	}
	return WriteText(root, contract, path, encoded.String())
}
