package schema

import (
	"bytes"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func ValidateContextDigest(raw []byte) error {
	return validate("context_digest", raw)
}

func ValidatePlan(raw []byte) error {
	return validate("plan", raw)
}

func validate(name string, raw []byte) error {
	compiled, err := Compiled(name)
	if err != nil {
		return err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	return compiled.Validate(doc)
}
