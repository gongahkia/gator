package config

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type DesktopSettings struct {
	ReferToChats   *bool             `json:"refer_to_chats,omitempty"`
	LearnFromChats *bool             `json:"learn_from_chats,omitempty"`
	Shortcuts      map[string]string `json:"shortcuts,omitempty"`
	Voice          VoiceSettings     `json:"voice,omitzero"`
}

func (d DesktopSettings) RefersToChats() bool { return d.ReferToChats == nil || *d.ReferToChats }

func (d DesktopSettings) LearnsFromChats() bool { return d.LearnFromChats == nil || *d.LearnFromChats }

type VoiceSettings struct {
	SpokenReplies bool    `json:"spoken_replies"`
	SystemVoice   string  `json:"system_voice,omitempty"`
	Provider      string  `json:"provider,omitempty"`
	Model         string  `json:"model,omitempty"`
	Language      string  `json:"language,omitempty"`
	Rate          float64 `json:"rate,omitempty"`
}

var DefaultShortcuts = map[string]string{
	"new_chat": "Ctrl+N", "command_search": "Ctrl+P", "conversation_search": "Ctrl+F", "settings": "Ctrl+,",
	"send_message": "Enter", "queue_message": "Tab", "newline": "Ctrl+J", "guide_work": "Ctrl+G",
	"voice_input": "Ctrl+V", "help": "Ctrl+O",
}

var retiredFunctionShortcut = regexp.MustCompile(`(?i)(^|\+)f([1-9]|1[0-2])$`)

func (d DesktopSettings) ResolvedShortcuts() map[string]string {
	result := make(map[string]string, len(DefaultShortcuts))
	occupied := map[string]bool{}
	for action, binding := range d.Shortcuts {
		// Old GUI profiles may contain function keys. Resolve those actions to
		// current defaults while keeping the stored profile readable.
		if retiredFunctionShortcut.MatchString(strings.Join(strings.Fields(binding), "")) {
			continue
		}
		canonical, err := NormalizeActionShortcut(action, binding)
		if err != nil {
			result[action] = binding
			continue
		}
		result[action] = physicalShortcut(canonical)
		occupied[result[action]] = true
	}
	actions := make([]string, 0, len(DefaultShortcuts))
	for action := range DefaultShortcuts {
		actions = append(actions, action)
	}
	slices.Sort(actions)
	for _, action := range actions {
		if _, custom := result[action]; custom {
			continue
		}
		binding := DefaultShortcuts[action]
		key := binding[strings.LastIndex(binding, "+")+1:]
		choices := []string{binding, "Ctrl+Shift+" + key, "Ctrl+Alt+" + key, "Ctrl+Alt+Shift+" + key}
		for letter := 'A'; letter <= 'Z'; letter++ {
			choices = append(choices, "Ctrl+Shift+"+string(letter))
		}
		for _, candidate := range choices {
			if _, err := NormalizeActionShortcut(action, candidate); err == nil && !occupied[candidate] {
				result[action] = candidate
				occupied[candidate] = true
				break
			}
		}
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
		case "mod", "cmdorctrl", "cmd", "command", "meta", "ctrl", "control":
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
		} else {
			return "", errors.New("use a Ctrl shortcut with a letter, number, comma, Enter or Space")
		}
	}
	if len(mods) == 0 && key != "Enter" {
		return "", errors.New("single letters and other typing keys need a modifier")
	}
	ordered := []string{}
	for _, name := range []string{"Ctrl", "Alt", "Shift"} {
		if mods[name] {
			ordered = append(ordered, name)
		}
	}
	ordered = append(ordered, key)
	canonical := strings.Join(ordered, "+")
	physical := physicalShortcut(canonical)
	if slices.Contains([]string{"Ctrl+Q", "Ctrl+W", "Ctrl+R", "Ctrl+Alt+I", "Ctrl+Shift+I"}, physical) {
		return "", errors.New("this shortcut is reserved by Gator or the terminal")
	}
	if key == "Enter" && mods["Alt"] && len(mods) == 1 {
		return "", errors.New("Alt+Enter is reserved")
	}
	return canonical, nil
}

// Tab queues only in the non-command conversation composer during active work.
// It remains completion/navigation elsewhere and cannot become a global action.
func NormalizeActionShortcut(action, value string) (string, error) {
	if action == "queue_message" && strings.EqualFold(strings.TrimSpace(value), "Tab") {
		return "Tab", nil
	}
	return NormalizeShortcut(value)
}

func physicalShortcut(value string) string {
	return strings.NewReplacer("Mod", "Ctrl", "Meta", "Ctrl").Replace(value)
}

func (d DesktopSettings) Validate() error {
	for action := range d.Shortcuts {
		if _, exists := DefaultShortcuts[action]; !exists {
			return fmt.Errorf("unknown shortcut action %q", action)
		}
	}
	seen := map[string]string{}
	for action, shortcut := range d.ResolvedShortcuts() {
		if _, exists := DefaultShortcuts[action]; !exists {
			return fmt.Errorf("unknown shortcut action %q", action)
		}
		normalized, err := NormalizeActionShortcut(action, shortcut)
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
	if !slices.Contains([]string{"", "gemini", "openai", "whisper-local"}, v.Provider) || len(v.Model) > 128 || strings.ContainsAny(v.Model, "/\\\r\n") || len(v.Language) > 35 || v.Rate < 0 || v.Rate > 2 || v.Rate > 0 && v.Rate < 0.5 {
		return errors.New("invalid voice preferences")
	}
	if len(v.SystemVoice) > 256 || strings.ContainsAny(v.SystemVoice, "\x00\r\n") {
		return errors.New("invalid system voice")
	}
	if v.Provider == "whisper-local" && !slices.Contains([]string{"", "tiny", "base", "small"}, v.Model) {
		return errors.New("choose Tiny, Base or Small for local Whisper")
	}
	if v.Language != "" && !regexp.MustCompile(`^[A-Za-z]{2,3}(?:-[A-Za-z0-9]{2,8})*$`).MatchString(v.Language) {
		return errors.New("voice language must be a language tag such as en or en-SG")
	}
	return nil
}
