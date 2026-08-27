package tools

import (
	"strings"
	"testing"
)

func TestPrefixAllowsAnchoredLiteralTokensOnly(t *testing.T) {
	if err := ValidateCommandPrefix([]string{"go", "test"}); err != nil {
		t.Fatal(err)
	}
	if !PrefixAllows([][]string{{"go", "test"}}, []string{"go", "test", "./internal/tui"}) {
		t.Fatal("literal prefix should match a longer argv")
	}
	if PrefixAllows([][]string{{"go", "test"}}, []string{"go", "test ./pkg"}) {
		t.Fatal("whitespace-bearing token must not match a prefix")
	}
	if PrefixAllows([][]string{{"go", "test"}}, []string{"env", "go", "test"}) {
		t.Fatal("unanchored program must not match")
	}
	if PrefixAllows([][]string{{"go", "test"}}, []string{"go"}) {
		t.Fatal("shorter argv must not match a longer prefix")
	}
}

func TestValidateCommandPrefixRejectsGlobsShellAndLaunchers(t *testing.T) {
	for _, pattern := range [][]string{
		{"go", "test*"},
		{"bash", "-lc", "go test"},
		{"sh"},
		{"env", "go"},
		{"go test"},
		nil,
	} {
		if err := ValidateCommandPrefix(pattern); err == nil {
			t.Fatalf("accepted %#v", pattern)
		}
	}
	if PrefixAllows([][]string{{"go", "*"}}, []string{"go", "test"}) {
		t.Fatal("glob tokens must not match even if they bypass validation")
	}
	if strings.Contains("go test ./pkg", "*") {
		t.Fatal("sanity")
	}
}
