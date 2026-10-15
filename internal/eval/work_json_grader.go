package eval

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

func validateStructuredGrader(g WorkGrader) error {
	switch g.Kind {
	case "json_pointer_equals", "json_pointer_contains", "json_pointer_absent":
		if g.Path == "" {
			return errors.New("JSON pointer grader requires an output path")
		}
		if _, err := pointerSegments(g.Locator); err != nil {
			return err
		}
		if g.Kind == "json_pointer_absent" {
			if g.Expected != "" {
				return errors.New("JSON pointer absence grader cannot have an expected value")
			}
			return nil
		}
		if g.Kind == "json_pointer_contains" {
			if g.Expected == "" {
				return errors.New("JSON pointer substring grader requires non-empty text")
			}
			return nil
		}
		if _, err := decodeJSONValue([]byte(g.Expected)); err != nil {
			return fmt.Errorf("expected value must be one JSON value: %w", err)
		}
	case "file_excludes":
		if g.Path == "" || g.Expected == "" {
			return errors.New("file exclusion grader requires a path and non-empty text")
		}
	case "document_text_contains", "document_text_excludes":
		if g.Expected == "" || (!strings.EqualFold(filepath.Ext(g.Path), ".docx") && !strings.EqualFold(filepath.Ext(g.Path), ".pdf")) {
			return errors.New("document text grader requires DOCX or PDF output and non-empty text")
		}
	case "file_regex", "file_not_regex":
		if g.Path == "" || len(g.Expected) == 0 || len(g.Expected) > 1024 {
			return errors.New("file regex grader requires a path and a pattern within 1024 bytes")
		}
		if _, err := regexp.Compile(g.Expected); err != nil {
			return fmt.Errorf("invalid file grader regex: %w", err)
		}
	case "pptx_slide_contains", "pptx_slide_excludes":
		index, err := strconv.Atoi(g.Locator)
		if g.Path == "" || index < 1 || err != nil || g.Expected == "" {
			return errors.New("PPTX slide grader requires a path, positive slide number, and non-empty text")
		}
	case "pptx_text_contains", "pptx_text_excludes":
		if g.Path == "" || g.Locator != "" || g.Expected == "" {
			return errors.New("PPTX visible text grader requires a path and non-empty text without a slide locator")
		}
	case "pptx_slide_count":
		count, err := strconv.Atoi(g.Expected)
		if g.Path == "" || g.Locator != "" || err != nil || count < 1 {
			return errors.New("PPTX slide count grader requires a path and positive count")
		}
	case "cell_formula_equals":
		if !strings.EqualFold(filepath.Ext(g.Path), ".xlsx") || g.Sheet == "" || g.Cell == "" || g.Expected == "" {
			return errors.New("workbook formula grader requires an XLSX path, sheet, cell, and formula")
		}
	case "xlsx_chart_count":
		count, err := strconv.Atoi(g.Expected)
		if !strings.EqualFold(filepath.Ext(g.Path), ".xlsx") || g.Sheet != "" || g.Cell != "" || err != nil || count < 1 {
			return errors.New("workbook chart grader requires an XLSX path and positive chart count")
		}
	}
	return nil
}

func decodeJSONValue(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	return value, nil
}

func pointerSegments(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if len(pointer) > 1024 || !strings.HasPrefix(pointer, "/") {
		return nil, errors.New("JSON pointer must be empty or start with / and fit within 1024 bytes")
	}
	encoded := strings.Split(pointer[1:], "/")
	segments := make([]string, 0, len(encoded))
	for _, segment := range encoded {
		var decoded strings.Builder
		for i := 0; i < len(segment); i++ {
			if segment[i] != '~' {
				decoded.WriteByte(segment[i])
				continue
			}
			i++
			if i >= len(segment) || (segment[i] != '0' && segment[i] != '1') {
				return nil, errors.New("JSON pointer contains an invalid escape")
			}
			if segment[i] == '0' {
				decoded.WriteByte('~')
			} else {
				decoded.WriteByte('/')
			}
		}
		segments = append(segments, decoded.String())
	}
	return segments, nil
}

func jsonPointerValue(data []byte, pointer string) (any, bool, error) {
	value, err := decodeJSONValue(data)
	if err != nil {
		return nil, false, err
	}
	segments, err := pointerSegments(pointer)
	if err != nil {
		return nil, false, err
	}
	for _, segment := range segments {
		switch current := value.(type) {
		case map[string]any:
			var found bool
			value, found = current[segment]
			if !found {
				return nil, false, nil
			}
		case []any:
			if segment == "" || (len(segment) > 1 && segment[0] == '0') {
				return nil, false, nil
			}
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(current) {
				return nil, false, nil
			}
			value = current[index]
		default:
			return nil, false, nil
		}
	}
	return value, true, nil
}

func jsonValuesEqual(a, b any) bool {
	return reflect.DeepEqual(a, b)
}
