package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

type headerBinding struct {
	Name, Type string
	Path       []string
}

var headerToken = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

func (c *httpClient) rememberTools(tools []toolDescription) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.headers = map[string][]headerBinding{}
	if c.version != modernVersion {
		return nil
	}
	for _, tool := range tools {
		var schema any
		if json.Unmarshal(tool.InputSchema, &schema) != nil {
			return errors.New("invalid MCP tool schema")
		}
		names := map[string]bool{}
		var bindings []headerBinding
		var walk func(any, []string, bool, int) error
		walk = func(value any, path []string, reachable bool, depth int) error {
			if depth > 24 {
				return errors.New("MCP schema exceeds 24 nesting levels")
			}
			switch value := value.(type) {
			case map[string]any:
				if annotation, present := value["x-mcp-header"]; present {
					name, ok := annotation.(string)
					kind, _ := value["type"].(string)
					if !ok || !reachable || len(path) == 0 || len(name) > 64 || !headerToken.MatchString(name) || names[strings.ToLower(name)] || (kind != "string" && kind != "boolean" && kind != "integer") || len(bindings) >= 32 {
						return errors.New("MCP tool has an unsupported or invalid header annotation")
					}
					names[strings.ToLower(name)] = true
					bindings = append(bindings, headerBinding{Name: name, Type: kind, Path: append([]string(nil), path...)})
				}
				for key, child := range value {
					if key == "properties" {
						if properties, ok := child.(map[string]any); ok {
							for key, property := range properties {
								next := append(append([]string(nil), path...), key)
								if err := walk(property, next, reachable, depth+1); err != nil {
									return err
								}
							}
							continue
						}
					}
					if err := walk(child, path, false, depth+1); err != nil {
						return err
					}
				}
			case []any:
				for _, child := range value {
					if err := walk(child, path, false, depth+1); err != nil {
						return err
					}
				}
			}
			return nil
		}
		if err := walk(schema, nil, true, 0); err != nil {
			return err
		}
		c.headers[tool.Name] = bindings
	}
	return nil
}
func applyHeaders(headers http.Header, bindings []headerBinding, raw json.RawMessage) error {
	var arguments map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&arguments) != nil || arguments == nil {
		return errors.New("MCP tool arguments must be an object")
	}
	total := 0
	for _, binding := range bindings {
		var current any = arguments
		for _, key := range binding.Path {
			object, ok := current.(map[string]any)
			if !ok {
				current = nil
				break
			}
			current = object[key]
		}
		if current == nil {
			continue
		}
		var text string
		valid := false
		switch binding.Type {
		case "string":
			text, valid = current.(string)
		case "boolean":
			if value, ok := current.(bool); ok {
				text = strconv.FormatBool(value)
				valid = true
			}
		case "integer":
			if value, ok := current.(json.Number); ok {
				text, valid = headerInteger(value.String())
			}
		}
		if !valid {
			return errors.New("MCP header parameter does not match its declared primitive type")
		}
		text = encodeHeader(text)
		total += len(text) + len(binding.Name)
		if total > 16*1024 {
			return errors.New("MCP derived headers exceed 16 KiB")
		}
		headers.Set("Mcp-Param-"+binding.Name, text)
	}
	return nil
}
func encodeHeader(value string) string {
	encode := strings.TrimSpace(value) != value || strings.HasPrefix(value, "=?base64?") && strings.HasSuffix(value, "?=")
	for _, b := range []byte(value) {
		if b < 0x20 && b != '\t' || b > 0x7e {
			encode = true
			break
		}
	}
	if encode {
		return "=?base64?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
	}
	return value
}

// JSON Schema integers include 42.0 and exponent forms. Keep exact arithmetic
// bounded before parsing so tiny fractions cannot round into authorized values.
func headerInteger(value string) (string, bool) {
	if len(value) > 128 {
		return "", false
	}
	if _, exponent, ok := strings.Cut(strings.ToLower(value), "e"); ok {
		e, err := strconv.Atoi(exponent)
		if err != nil || e < -128 || e > 128 {
			return "", false
		}
	}
	rational, ok := new(big.Rat).SetString(value)
	if !ok || !rational.IsInt() || !rational.Num().IsInt64() {
		return "", false
	}
	number := rational.Num().Int64()
	if number < -9007199254740991 || number > 9007199254740991 {
		return "", false
	}
	return strconv.FormatInt(number, 10), true
}
