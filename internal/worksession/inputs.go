package worksession

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (s Store) inputNamesPath(conversation, revision string) string {
	return filepath.Join(s.root, conversation, "input-files", revision+".json")
}

// Optional presentation data stays outside the strict replay schema.
func (s Store) readInputNames(conversation, revision string) []string {
	file, err := os.Open(s.inputNamesPath(conversation, revision))
	if err != nil {
		return nil
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(data) > 4096 {
		return nil
	}
	var names []string
	if json.Unmarshal(data, &names) != nil || len(names) > 4 {
		return nil
	}
	for _, name := range names {
		if name == "" || len(name) > 800 || strings.ContainsAny(name, "\x00\r\n") {
			return nil
		}
	}
	return names
}
