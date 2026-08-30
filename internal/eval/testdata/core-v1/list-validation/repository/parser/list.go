package parser

import "strings"

// ParseList splits a comma-separated list and normalizes whitespace.
func ParseList(input string) ([]string, error) {
	var values []string
	for _, item := range strings.Split(input, ",") {
		if item = strings.TrimSpace(item); item != "" {
			values = append(values, item)
		}
	}
	return values, nil
}
