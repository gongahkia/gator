package gather

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gongahkia/paw/internal/envelope"
)

func collectSyntaxContexts(cwd string, hits []envelope.RawUnit, maxBytes int) ([]envelope.RawUnit, error) {
	if cwd == "" {
		return nil, nil
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxFileBytes
	}
	parsed := map[string][]declSpan{}
	seen := map[string]bool{}
	var units []envelope.RawUnit
	for _, hit := range hits {
		if hit.Kind != "search_hits" || filepath.Ext(hit.Path) != ".go" {
			continue
		}
		spans, ok := parsed[hit.Path]
		if !ok {
			var err error
			spans, err = goDeclSpans(cwd, hit.Path)
			if err != nil {
				parsed[hit.Path] = nil
				continue
			}
			parsed[hit.Path] = spans
		}
		for _, line := range hitLines(hit) {
			span, ok := enclosingDecl(spans, line)
			if !ok {
				continue
			}
			key := hit.Path + ":" + strconv.Itoa(span.start) + ":" + strconv.Itoa(span.end)
			if seen[key] {
				continue
			}
			unit, ok, err := syntaxContextUnit(cwd, hit.Path, span.start, span.end, maxBytes)
			if err != nil {
				return nil, err
			}
			if ok {
				seen[key] = true
				units = append(units, unit)
			}
		}
	}
	return units, nil
}

func goDeclSpans(cwd, rel string) ([]declSpan, error) {
	path, ok := safePath(cwd, rel)
	if !ok {
		return nil, nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}
	spans := make([]declSpan, 0, len(file.Decls))
	for _, decl := range file.Decls {
		switch decl.(type) {
		case *ast.FuncDecl, *ast.GenDecl:
			spans = append(spans, declSpan{
				start: fset.Position(decl.Pos()).Line,
				end:   fset.Position(decl.End()).Line,
			})
		}
	}
	return spans, nil
}

func enclosingDecl(spans []declSpan, line int) (declSpan, bool) {
	var best declSpan
	found := false
	for _, span := range spans {
		if line < span.start || line > span.end {
			continue
		}
		if !found || span.end-span.start < best.end-best.start {
			best = span
			found = true
		}
	}
	return best, found
}

func hitLines(hit envelope.RawUnit) []int {
	seen := map[int]bool{}
	var lines []int
	scanner := bufio.NewScanner(strings.NewReader(hit.Text))
	for scanner.Scan() {
		lineText := scanner.Text()
		idx := strings.IndexByte(lineText, ':')
		if idx <= 0 {
			continue
		}
		line, err := strconv.Atoi(strings.TrimSpace(lineText[:idx]))
		if err == nil && line > 0 && !seen[line] {
			seen[line] = true
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 && hit.StartLine > 0 {
		lines = append(lines, hit.StartLine)
	}
	return lines
}

func syntaxContextUnit(cwd, rel string, startLine, endLine, maxBytes int) (envelope.RawUnit, bool, error) {
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
	scanner := bufio.NewScanner(f)
	lineNo := 0
	actualEnd := startLine - 1
	var b strings.Builder
	for scanner.Scan() {
		lineNo++
		if lineNo < startLine {
			continue
		}
		if lineNo > endLine {
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
		Kind:      "syntax_context",
		Path:      filepath.Clean(rel),
		StartLine: startLine,
		EndLine:   actualEnd,
		Text:      b.String(),
	}, true, nil
}

type declSpan struct {
	start int
	end   int
}
