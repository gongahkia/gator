package config

import (
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

type DesktopSettings struct {
	Shortcuts map[string]string `json:"shortcuts,omitempty"`
	Voice     VoiceSettings     `json:"voice,omitzero"`
}

type VoiceSettings struct {
	SpokenReplies bool    `json:"spoken_replies"`
	Provider      string  `json:"provider,omitempty"`
	Model         string  `json:"model,omitempty"`
	Language      string  `json:"language,omitempty"`
	Rate          float64 `json:"rate,omitempty"`
}

var DefaultShortcuts = map[string]string{
	"command_search": "Mod+K", "conversation_search": "Mod+F", "settings": "Mod+,",
	"send_message": "Enter", "newline": "Shift+Enter", "guide_work": "Mod+Enter",
	"voice_input": "Mod+Shift+V", "help": "F1",
}

func (d DesktopSettings) ResolvedShortcuts() map[string]string {
	result := make(map[string]string, len(DefaultShortcuts))
	for action, shortcut := range DefaultShortcuts {
		result[action] = shortcut
	}
	for action, shortcut := range d.Shortcuts {
		result[action] = shortcut
	}
	return result
}

func NormalizeShortcut(value string) (string, error) {
	if len(value) > 64 {
		return "", errors.New("shortcut is too long")
	}
	parts := strings.Split(strings.TrimSpace(value), "+")
	mods := map[string]bool{}
	for _, part := range parts[:len(parts)-1] {
		var name string
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "mod", "cmdorctrl":
			name = "Mod"
		case "cmd", "command", "meta":
			name = "Meta"
		case "ctrl", "control":
			name = "Ctrl"
		case "option", "alt":
			name = "Alt"
		case "shift":
			name = "Shift"
		default:
			return "", errors.New("use Mod, Cmd, Ctrl, Alt or Shift modifiers")
		}
		if mods[name] {
			return "", errors.New("duplicate shortcut modifier")
		}
		mods[name] = true
	}
	key := strings.TrimSpace(parts[len(parts)-1])
	lower := strings.ToLower(key)
	switch lower {
	case "enter", "return":
		key = "Enter"
	case "space":
		key = "Space"
	case ",", "comma":
		key = ","
	default:
		if regexp.MustCompile(`^[a-z0-9]$`).MatchString(lower) {
			key = strings.ToUpper(key)
		} else if regexp.MustCompile(`^f([1-9]|1[0-2])$`).MatchString(lower) {
			key = strings.ToUpper(key)
		} else {
			return "", errors.New("use a letter, number, comma, Enter, Space or F1–F12")
		}
	}
	if len(mods) == 0 && key != "Enter" && key != "F1" {
		return "", errors.New("single letters and other typing keys need a modifier")
	}
	ordered := []string{}
	for _, name := range []string{"Mod", "Meta", "Ctrl", "Alt", "Shift"} {
		if mods[name] {
			ordered = append(ordered, name)
		}
	}
	ordered = append(ordered, key)
	canonical := strings.Join(ordered, "+")
	physical := physicalShortcut(canonical)
	if slices.Contains([]string{"Meta+Q", "Meta+W", "Meta+R", "Meta+H", "Ctrl+Q", "Ctrl+W", "Ctrl+R", "Alt+F4", "Meta+Alt+I", "Ctrl+Shift+I"}, physical) {
		return "", errors.New("this shortcut is reserved by the operating system or app menu")
	}
	if key == "Enter" && mods["Alt"] && len(mods) == 1 {
		return "", errors.New("Alt+Enter is reserved")
	}
	return canonical, nil
}

func physicalShortcut(value string) string {
	modifier := "Ctrl"
	if runtime.GOOS == "darwin" {
		modifier = "Meta"
	}
	return strings.ReplaceAll(value, "Mod", modifier)
}

func (d DesktopSettings) Validate() error {
	seen := map[string]string{}
	for action, shortcut := range d.ResolvedShortcuts() {
		if _, exists := DefaultShortcuts[action]; !exists {
			return fmt.Errorf("unknown shortcut action %q", action)
		}
		normalized, err := NormalizeShortcut(shortcut)
		if err != nil {
			return fmt.Errorf("%s shortcut: %w", action, err)
		}
		physical := physicalShortcut(normalized)
		if prior, exists := seen[physical]; exists {
			return fmt.Errorf("shortcut conflicts with %s", prior)
		}
		seen[physical] = action
	}
	v := d.Voice
	if !slices.Contains([]string{"", "gemini", "openai"}, v.Provider) || len(v.Model) > 128 || strings.ContainsAny(v.Model, "/\\\r\n") || len(v.Language) > 35 || v.Rate < 0 || v.Rate > 2 || v.Rate > 0 && v.Rate < 0.5 {
		return errors.New("invalid voice preferences")
	}
	if v.Language != "" && !regexp.MustCompile(`^[A-Za-z]{2,3}(?:-[A-Za-z0-9]{2,8})*$`).MatchString(v.Language) {
		return errors.New("voice language must be a language tag such as en or en-SG")
	}
	return nil
}
