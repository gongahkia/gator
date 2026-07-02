package gather

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strconv"

	"github.com/gongahkia/paw/internal/envelope"
)

func collectSymbols(ctx context.Context, cwd string, maxBytes int) ([]envelope.RawUnit, error) {
	if cwd == "" {
		return nil, nil
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxFileBytes
	}
	if _, err := exec.LookPath("ctags"); err != nil {
		return nil, nil
	}
	args := []string{"--output-format=json", "-R", "-f", "-", "--exclude=.git", "--exclude=node_modules", "--exclude=.paw", cwd}
	out, err := exec.CommandContext(ctx, "ctags", args...).Output()
	if err != nil {
		return nil, nil
	}
	text := symbolText(cwd, out, maxBytes)
	if text == "" {
		return nil, nil
	}
	return []envelope.RawUnit{{Kind: "symbol-map", Text: text}}, nil
}

func symbolText(cwd string, out []byte, maxBytes int) string {
	scanner := bufio.NewScanner(bytes.NewReader(out))
	var b bytes.Buffer
	for scanner.Scan() {
		var tag ctag
		if json.Unmarshal(scanner.Bytes(), &tag) != nil || tag.Name == "" || tag.Path == "" {
			continue
		}
		line := tag.Kind + " " + tag.Name + " " + relPath(cwd, tag.Path) + ":" + strconv.Itoa(tag.Line) + "\n"
		if b.Len()+len(line) > maxBytes {
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

type ctag struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Line int    `json:"line"`
	Kind string `json:"kind"`
}
