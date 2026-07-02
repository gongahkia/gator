package envelope

const SchemaVersion = "paw.env/1"

type Envelope struct {
	SchemaVersion string         `json:"schema_version"`
	TaskID        string         `json:"task_id"`
	Instruction   string         `json:"instruction"`
	Cwd           string         `json:"cwd"`
	Stage         string         `json:"stage"`
	Turn          int            `json:"turn"`
	Digest        *ContextDigest `json:"digest,omitempty"`
	Plan          *Plan          `json:"plan,omitempty"`
	Patch         *Patch         `json:"patch,omitempty"`
	Verify        *VerifyResult  `json:"verify,omitempty"`
	Budget        Budget         `json:"budget"`
	Raw           *RawContext    `json:"raw,omitempty"`
	Done          bool           `json:"done"`
}
