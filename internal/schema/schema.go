package schema

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed *.schema.json
var schemaFiles embed.FS

func Compiled(name string) (*jsonschema.Schema, error) {
	file := schemaFile(name)
	raw := Raw(file)
	if raw == nil {
		return nil, fmt.Errorf("schema %q not found", name)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(file, doc); err != nil {
		return nil, err
	}
	return compiler.Compile(file)
}

func Raw(name string) json.RawMessage {
	b, err := schemaFiles.ReadFile(schemaFile(name))
	if err != nil {
		return nil
	}
	raw := make([]byte, len(b))
	copy(raw, b)
	return raw
}

func schemaFile(name string) string {
	if strings.HasSuffix(name, ".schema.json") {
		return name
	}
	return name + ".schema.json"
}
