package doctor

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type Status string

const (
	StatusOK      Status = "ok"
	StatusWarn    Status = "warn"
	StatusFail    Status = "fail"
	StatusFixed   Status = "fixed"
	StatusPlanned Status = "plan"
	StatusSkipped Status = "skip"
)

type Finding struct {
	ID       string            `json:"id"`
	Section  string            `json:"section"`
	Severity Severity          `json:"severity"`
	Status   Status            `json:"status"`
	Message  string            `json:"message"`
	Detail   string            `json:"detail,omitempty"`
	Fix      string            `json:"fix,omitempty"`
	Command  string            `json:"command,omitempty"`
	Path     string            `json:"path,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type Summary struct {
	OK      int `json:"ok"`
	Warning int `json:"warning"`
	Error   int `json:"error"`
	Fixed   int `json:"fixed"`
	Planned int `json:"planned"`
	Skipped int `json:"skipped"`
}

type Report struct {
	SchemaVersion int       `json:"schema_version"`
	Version       string    `json:"version,omitempty"`
	CWD           string    `json:"cwd"`
	Summary       Summary   `json:"summary"`
	Findings      []Finding `json:"findings"`
}

type Operation struct {
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	Status    string `json:"status"`
	Path      string `json:"path,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Error     string `json:"error,omitempty"`
}
