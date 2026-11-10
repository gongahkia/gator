package config

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type TerminalSections struct {
	Outputs     bool `json:"outputs"`
	Context     bool `json:"context"`
	Connections bool `json:"connections"`
	Workspace   bool `json:"workspace"`
}

// Theme has one owner: Settings.Theme. Only navigation/layout live under TUI.
// A nil layout marks a profile whose existing frontend values need importing.
type TerminalLayoutPreferences struct {
	SidebarCollapsed    bool             `json:"sidebarCollapsed"`
	GroupChatsByFolder  bool             `json:"groupChatsByFolder"`
	WorkDetailsSections TerminalSections `json:"workDetailsSections"`
}

type TerminalPreferences struct {
	Theme string `json:"theme"`
	TerminalLayoutPreferences
}

func (p *TerminalPreferences) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{"theme", "sidebarCollapsed", "groupChatsByFolder", "workDetailsSections"} {
		if len(fields[key]) == 0 || bytes.Equal(fields[key], []byte("null")) {
			return fmt.Errorf("terminal preference import is missing %s", key)
		}
	}
	var sections map[string]json.RawMessage
	if err := json.Unmarshal(fields["workDetailsSections"], &sections); err != nil {
		return err
	}
	for _, key := range []string{"outputs", "context", "connections", "workspace"} {
		if len(sections[key]) == 0 || bytes.Equal(sections[key], []byte("null")) {
			return fmt.Errorf("terminal preference import is missing section %s", key)
		}
	}
	type plain TerminalPreferences
	var result plain
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return err
	}
	*p = TerminalPreferences(result)
	return p.Validate()
}

type TerminalSectionsPatch struct {
	Outputs     *bool `json:"outputs,omitempty"`
	Context     *bool `json:"context,omitempty"`
	Connections *bool `json:"connections,omitempty"`
	Workspace   *bool `json:"workspace,omitempty"`
}

type TerminalPreferencesPatch struct {
	Theme               *string                `json:"theme,omitempty"`
	SidebarCollapsed    *bool                  `json:"sidebarCollapsed,omitempty"`
	GroupChatsByFolder  *bool                  `json:"groupChatsByFolder,omitempty"`
	WorkDetailsSections *TerminalSectionsPatch `json:"workDetailsSections,omitempty"`
}

func DefaultTerminalPreferences() TerminalPreferences {
	return TerminalPreferences{Theme: "terminal", TerminalLayoutPreferences: TerminalLayoutPreferences{SidebarCollapsed: true, WorkDetailsSections: TerminalSections{Outputs: true}}}
}

func (s Settings) TerminalPreferences() TerminalPreferences {
	result := DefaultTerminalPreferences()
	if s.Theme != "" {
		result.Theme = s.Theme
	}
	if s.TUI.Preferences != nil {
		result.TerminalLayoutPreferences = *s.TUI.Preferences
	}
	return result
}

func (p TerminalPreferences) Validate() error {
	switch p.Theme {
	case "terminal", "gator", "light", "contrast", "mono":
		return nil
	}
	return fmt.Errorf("unknown theme %q", p.Theme)
}

func (p TerminalPreferencesPatch) Empty() bool {
	if p.Theme != nil || p.SidebarCollapsed != nil || p.GroupChatsByFolder != nil {
		return false
	}
	s := p.WorkDetailsSections
	return s == nil || s.Outputs == nil && s.Context == nil && s.Connections == nil && s.Workspace == nil
}

func (p TerminalPreferencesPatch) Apply(current TerminalPreferences) (TerminalPreferences, error) {
	if p.Theme != nil {
		current.Theme = *p.Theme
	}
	if p.SidebarCollapsed != nil {
		current.SidebarCollapsed = *p.SidebarCollapsed
	}
	if p.GroupChatsByFolder != nil {
		current.GroupChatsByFolder = *p.GroupChatsByFolder
	}
	if s := p.WorkDetailsSections; s != nil {
		if s.Outputs != nil {
			current.WorkDetailsSections.Outputs = *s.Outputs
		}
		if s.Context != nil {
			current.WorkDetailsSections.Context = *s.Context
		}
		if s.Connections != nil {
			current.WorkDetailsSections.Connections = *s.Connections
		}
		if s.Workspace != nil {
			current.WorkDetailsSections.Workspace = *s.Workspace
		}
	}
	return current, current.Validate()
}

// Only fields the user asked to change participate in the stale-value check.
// A concurrent unrelated preference is preserved by the settings transaction.
func (p TerminalPreferencesPatch) Matches(current, expected TerminalPreferences) bool {
	if p.Theme != nil && current.Theme != expected.Theme || p.SidebarCollapsed != nil && current.SidebarCollapsed != expected.SidebarCollapsed || p.GroupChatsByFolder != nil && current.GroupChatsByFolder != expected.GroupChatsByFolder {
		return false
	}
	if s := p.WorkDetailsSections; s != nil {
		a, b := current.WorkDetailsSections, expected.WorkDetailsSections
		if s.Outputs != nil && a.Outputs != b.Outputs || s.Context != nil && a.Context != b.Context || s.Connections != nil && a.Connections != b.Connections || s.Workspace != nil && a.Workspace != b.Workspace {
			return false
		}
	}
	return true
}

func (s Store) UpdateTerminalPreferences(patch TerminalPreferencesPatch, expected *TerminalPreferences) (TerminalPreferences, error) {
	var saved TerminalPreferences
	if patch.Empty() {
		return saved, fmt.Errorf("choose a terminal preference to change")
	}
	err := s.Update(func(current *Settings) error {
		before := current.TerminalPreferences()
		if expected != nil && !patch.Matches(before, *expected) {
			return fmt.Errorf("terminal preferences changed; inspect and try again")
		}
		// The native host imports layout before exposing its controls. A shell
		// theme command may run earlier; keep the old layout available to import.
		if current.TUI.Preferences == nil && (patch.SidebarCollapsed != nil || patch.GroupChatsByFolder != nil || patch.WorkDetailsSections != nil) {
			return fmt.Errorf("import terminal preferences before changing layout")
		}
		next, err := patch.Apply(before)
		if err != nil {
			return err
		}
		if patch.Theme != nil {
			current.Theme, current.TUI.ThemeConfigured = next.Theme, true
		}
		if current.TUI.Preferences != nil {
			current.TUI.Preferences = &next.TerminalLayoutPreferences
		}
		saved = next
		return nil
	})
	return saved, err
}

func (s Store) ImportTerminalPreferences(legacy TerminalPreferences) (TerminalPreferences, error) {
	var saved TerminalPreferences
	if err := legacy.Validate(); err != nil {
		return saved, err
	}
	err := s.Update(func(current *Settings) error {
		if current.TUI.Preferences == nil {
			current.TUI.Preferences = &legacy.TerminalLayoutPreferences
			// Explicit theme changes made by this version win a startup race;
			// the old profile still supplies its unrelated navigation preferences.
			if !current.TUI.ThemeConfigured {
				current.Theme = legacy.Theme
			}
			current.TUI.ThemeConfigured = true
		}
		saved = current.TerminalPreferences()
		return nil
	})
	return saved, err
}
