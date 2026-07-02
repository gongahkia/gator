package envelope

type NextAction struct {
	Kind        string `json:"kind"`
	Description string `json:"description"`
	TargetPath  string `json:"target_path,omitempty"`
	Command     string `json:"command,omitempty"`
}

type Plan struct {
	Done       bool        `json:"done"`
	Reasoning  string      `json:"reasoning"`
	NextAction *NextAction `json:"next_action,omitempty"`
}
