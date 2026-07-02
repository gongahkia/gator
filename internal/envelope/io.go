package envelope

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var ErrSchemaVersion = errors.New("unsupported envelope schema version")

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
	if env.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("%w: got %q want %q", ErrSchemaVersion, env.SchemaVersion, SchemaVersion)
	}
	return &env, nil
}
