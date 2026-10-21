package worktui

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gongahkia/gator/internal/uisnapshot"
)

// Validate the complete submitted form before changing any draft field. Native
// inputs describe existing values, choices, and limits, not new authority.
func validBridgeInput(field *uisnapshot.Input, value string) bool {
	if len(value) > field.Limit || (!field.Multiline && strings.ContainsAny(value, "\r\n")) || strings.ContainsRune(value, 0) || (field.ReadOnly && value != field.Value) {
		return false
	}
	allowed := func(value string) bool {
		for _, option := range field.Options {
			if option.Value == value {
				return true
			}
		}
		return false
	}
	switch field.Kind {
	case "select":
		return allowed(value)
	case "multi":
		var values []string
		if json.Unmarshal([]byte(value), &values) != nil || (field.Max > 0 && int64(len(values)) > field.Max) {
			return false
		}
		seen := map[string]bool{}
		for _, item := range values {
			if !allowed(item) || seen[item] {
				return false
			}
			seen[item] = true
		}
	case "number":
		return bridgeNumber(value, field.Min, field.Max)
	case "budget":
		parts := strings.Split(value, ",")
		if len(parts) != len(field.Parts) {
			return false
		}
		for index, part := range field.Parts {
			if !bridgeNumber(parts[index], part.Min, part.Max) {
				return false
			}
		}
	}
	return true
}

func bridgeNumber(value string, min, max int64) bool {
	number, err := strconv.ParseInt(value, 10, 64)
	return err == nil && number >= min && (max == 0 || number <= max)
}

func bridgeSelect(value string, options ...uisnapshot.Option) *uisnapshot.Input {
	return &uisnapshot.Input{Kind: "select", Value: value, Options: options, Limit: 4096}
}

func bridgeNumberInput(value, min, max int64) *uisnapshot.Input {
	return &uisnapshot.Input{Kind: "number", Value: strconv.FormatInt(value, 10), Min: min, Max: max, Limit: 32}
}
