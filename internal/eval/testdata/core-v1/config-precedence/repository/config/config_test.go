package config

import "testing"

func TestLookupUsesFileThenFallback(t *testing.T) {
	if got := Lookup(map[string]string{"theme": "light"}, nil, "theme", "system"); got != "light" {
		t.Fatalf("Lookup() = %q", got)
	}
	if got := Lookup(nil, nil, "theme", "system"); got != "system" {
		t.Fatalf("Lookup() fallback = %q", got)
	}
}
