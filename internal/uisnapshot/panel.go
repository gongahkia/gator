package uisnapshot

// Panel carries display-independent state from a Go workflow to the terminal
// renderer. Keyboard actions remain owned by the workflow that produced it.
type Panel struct {
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
}

// Hint is a keyboard action the frontend can present with the kit's shared
// shortcut styling. Footer remains available for longer workflow guidance.
type Hint struct {
	Shortcut string `json:"shortcut"`
	Action   string `json:"action"`
}

type Row struct {
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
	State  string `json:"state,omitempty"`
	Tone   string `json:"tone,omitempty"`
}

type Block struct {
	Title string   `json:"title,omitempty"`
	Lines []string `json:"lines"`
}
