package acp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	gatorrun "github.com/gongahkia/gator/internal/run"
)

func decodeParams(raw json.RawMessage, destination any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return errors.New("params object is required")
	}
	return json.Unmarshal(raw, destination)
}

func hasField(raw json.RawMessage, field string) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object[field] != nil
}

func responseID(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), id...)
}

func promptText(blocks []json.RawMessage) (string, error) {
	if len(blocks) == 0 {
		return "", errors.New("session/prompt requires at least one content block")
	}
	var task strings.Builder
	for index, raw := range blocks {
		var header struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &header); err != nil {
			return "", fmt.Errorf("prompt content block %d is invalid: %w", index+1, err)
		}
		var content string
		switch header.Type {
		case "text":
			var block struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(raw, &block); err != nil {
				return "", err
			}
			content = block.Text
		case "resource_link":
			var block struct {
				Name string `json:"name"`
				URI  string `json:"uri"`
			}
			if err := json.Unmarshal(raw, &block); err != nil {
				return "", err
			}
			if strings.TrimSpace(block.Name) == "" || strings.TrimSpace(block.URI) == "" {
				return "", fmt.Errorf("prompt resource link %d requires name and uri", index+1)
			}
			content = "Developer-provided resource link: " + block.Name + " (" + block.URI + ")"
		default:
			return "", fmt.Errorf("prompt content block %d has unsupported type %q", index+1, header.Type)
		}
		if task.Len()+len(content) > maxPromptBytes {
			return "", fmt.Errorf("prompt exceeds the %d KiB ACP limit", maxPromptBytes/1024)
		}
		if task.Len() > 0 {
			task.WriteString("\n\n")
		}
		task.WriteString(content)
	}
	if strings.TrimSpace(task.String()) == "" {
		return "", errors.New("session/prompt text must not be empty")
	}
	return task.String(), nil
}

func defaultMode(verification [][]string) gatorrun.Mode {
	if len(verification) == 0 {
		return gatorrun.PlanMode
	}
	return gatorrun.ExecuteMode
}

func titleFor(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) <= 96 {
		return value
	}
	return string([]rune(value)[:95]) + "…"
}

func acpToolKind(name string) string {
	switch name {
	case "read_file", "list_files", "http_fetch", "browser_snapshot", "browser_extract":
		return "read"
	case "search_files":
		return "search"
	case "write_file", "apply_patch":
		return "edit"
	case "run_command", "terminal_start", "terminal_write", "terminal_stop", "browser_navigate", "browser_act":
		return "execute"
	default:
		return "other"
	}
}

func cloneArgv(source [][]string) [][]string {
	if len(source) == 0 {
		return nil
	}
	result := make([][]string, len(source))
	for index, argv := range source {
		result[index] = append([]string(nil), argv...)
	}
	return result
}
