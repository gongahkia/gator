package uisnapshot

import "encoding/json"

// Result is a display-only command result. Go owns the facts and actions;
// the terminal frontend owns their visual presentation. Kind is one of
// notice, details, list, report, preview, or approval.
type Result struct {
	Kind     string          `json:"kind"`
	Title    string          `json:"title"`
	Tone     string          `json:"tone,omitempty"`
	Summary  string          `json:"summary,omitempty"`
	Fields   []Field         `json:"fields,omitempty"`
	Items    []ResultItem    `json:"items,omitempty"`
	Sections []ResultSection `json:"sections,omitempty"`
	Lines    []string        `json:"lines,omitempty"`
	Markdown string          `json:"markdown,omitempty"`
	Preview  json.RawMessage `json:"preview,omitempty"`
	Footer   string          `json:"footer,omitempty"`
}

type Field struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Tone  string `json:"tone,omitempty"`
}

type ResultItem struct {
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
	State  string `json:"state,omitempty"`
	Tone   string `json:"tone,omitempty"`
}

type ResultSection struct {
	Title    string       `json:"title"`
	Summary  string       `json:"summary,omitempty"`
	Fields   []Field      `json:"fields,omitempty"`
	Items    []ResultItem `json:"items,omitempty"`
	Lines    []string     `json:"lines,omitempty"`
	Markdown string       `json:"markdown,omitempty"`
}
