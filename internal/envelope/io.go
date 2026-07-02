package envelope

import (
	"encoding/json"
	"io"
)

func NewEnvelope(taskID, instruction, cwd string) *Envelope {
	return &Envelope{
		SchemaVersion: SchemaVersion,
		TaskID:        taskID,
		Instruction:   instruction,
		Cwd:           cwd,
		Budget:        Budget{},
	}
}

func (e *Envelope) Marshal(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(e)
}

func Unmarshal(r io.Reader) (*Envelope, error) {
	var env Envelope
	if err := json.NewDecoder(r).Decode(&env); err != nil {
		return nil, err
	}
	return &env, nil
}
