package gather

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/paw/internal/envelope"
)

const defaultMaxFileBytes = 64 * 1024

func collectFileSlices(cwd string, hits []envelope.RawUnit, maxBytes int) ([]envelope.RawUnit, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxFileBytes
	}
	var units []envelope.RawUnit
	for _, hit := range hits {
		if hit.Kind != "search_hits" || hit.Path == "" {
			continue
		}
		unit, ok, err := fileSlice(cwd, hit.Path, hit.StartLine, hit.EndLine, maxBytes)
		if err != nil {
			return nil, err
		}
		if ok {
			units = append(units, unit)
		}
	}
	return units, nil
}

func fileSlice(cwd, rel string, startLine, endLine, maxBytes int) (envelope.RawUnit, bool, error) {
	path, ok := safePath(cwd, rel)
	if !ok {
		return envelope.RawUnit{}, false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return envelope.RawUnit{}, false, nil
		}
		return envelope.RawUnit{}, false, err
	}
	defer func() { _ = f.Close() }()
	start := startLine - 2
	if start < 1 {
		start = 1
	}
	end := endLine + 2
	scanner := bufio.NewScanner(f)
	lineNo := 0
	var b strings.Builder
	actualEnd := start - 1
	for scanner.Scan() {
		lineNo++
		if lineNo < start {
			continue
		}
		if lineNo > end {
			break
		}
		line := scanner.Text()
		if b.Len() > 0 && b.Len()+len(line)+1 > maxBytes {
			break
		}
		if b.Len() == 0 && len(line)+1 > maxBytes {
			line = line[:maxBytes-1]
		}
		b.WriteString(line)
		b.WriteByte('\n')
		actualEnd = lineNo
	}
	if err := scanner.Err(); err != nil {
		return envelope.RawUnit{}, false, err
	}
	if b.Len() == 0 {
		return envelope.RawUnit{}, false, nil
	}
	return envelope.RawUnit{
		Kind:      "file_slice",
		Path:      filepath.Clean(rel),
		StartLine: start,
		EndLine:   actualEnd,
		Text:      b.String(),
	}, true, nil
}

func safePath(cwd, rel string) (string, bool) {
	base := filepath.Clean(cwd)
	full := filepath.Clean(filepath.Join(base, rel))
	if full != base && !strings.HasPrefix(full, base+string(os.PathSeparator)) {
		return "", false
	}
	return full, true
}
