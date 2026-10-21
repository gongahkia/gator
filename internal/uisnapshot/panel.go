package uisnapshot

// Panel carries display-independent state from a Go workflow to its frontends.
// Workflow actions remain owned by the Go application that produced it.
type Panel struct {
	ID       string  `json:"id,omitempty"`
	Kind     string  `json:"kind"`
	Title    string  `json:"title"`
	Subtitle string  `json:"subtitle,omitempty"`
	Query    string  `json:"query,omitempty"`
	Rows     []Row   `json:"rows,omitempty"`
	Blocks   []Block `json:"blocks,omitempty"`
	Selected int     `json:"selected,omitempty"`
	Notice   string  `json:"notice,omitempty"`
	Footer   string  `json:"footer,omitempty"`
	Hints    []Hint  `json:"hints,omitempty"`
	Usage    *Usage  `json:"usage,omitempty"`
	Filter   *Filter `json:"filter,omitempty"`
	Toggle   *Toggle `json:"toggle,omitempty"`
}

type Toggle struct {
	Label   string `json:"label"`
	Checked bool   `json:"checked"`
}

type Filter struct {
	Value   string   `json:"value"`
	Options []Option `json:"options"`
}

// Hint is a keyboard action the frontend can present with the kit's shared
// shortcut styling. Footer remains available for longer workflow guidance.
type Hint struct {
	Shortcut string `json:"shortcut"`
	Action   string `json:"action"`
}

type Row struct {
	Label    string `json:"label"`
	Group    string `json:"group,omitempty"`
	Checked  bool   `json:"checked,omitempty"`
	Position int    `json:"position,omitempty"`
	Detail   string `json:"detail,omitempty"`
	State    string `json:"state,omitempty"`
	Tone     string `json:"tone,omitempty"`
	Input    *Input `json:"input,omitempty"`
}

// Input describes an editable field without exposing secrets. Value is empty
// for secret fields; the desktop keeps a replacement only until submission.
type Input struct {
	Kind        string       `json:"kind,omitempty"`
	Help        string       `json:"help,omitempty"`
	Options     []Option     `json:"options,omitempty"`
	Parts       []NumberPart `json:"parts,omitempty"`
	Min         int64        `json:"min,omitempty"`
	Max         int64        `json:"max,omitempty"`
	ReadOnly    bool         `json:"read_only,omitempty"`
	Value       string       `json:"value,omitempty"`
	Placeholder string       `json:"placeholder,omitempty"`
	Secret      bool         `json:"secret,omitempty"`
	Multiline   bool         `json:"multiline,omitempty"`
	Limit       int          `json:"limit"`
}

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type NumberPart struct {
	Label string `json:"label"`
	Min   int64  `json:"min"`
	Max   int64  `json:"max"`
}

type Block struct {
	Title       string   `json:"title,omitempty"`
	Lines       []string `json:"lines"`
	Collapsible bool     `json:"collapsible,omitempty"`
}
