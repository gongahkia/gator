// Package tools contains policy-bounded implementations of the native agent's
// local workspace tools.
package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func success(value any) (string, error) {
	payload, err := json.Marshal(struct {
		OK     bool `json:"ok"`
		Result any  `json:"result"`
	}{OK: true, Result: value})
	if err != nil {
		return "", fmt.Errorf("encode tool result: %w", err)
	}
	return string(payload), nil
}

func decodeArguments(raw json.RawMessage, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode arguments: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode arguments: unexpected trailing JSON")
	}
	return nil
}
