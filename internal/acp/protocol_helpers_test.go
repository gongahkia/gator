package acp

import "testing"

func TestACPToolKindClassifiesBoundedBrowserOperations(t *testing.T) {
	for _, name := range []string{"browser_snapshot", "browser_extract"} {
		if kind := acpToolKind(name); kind != "read" {
			t.Fatalf("%s kind = %q, want read", name, kind)
		}
	}
	for _, name := range []string{"browser_navigate", "browser_act"} {
		if kind := acpToolKind(name); kind != "execute" {
			t.Fatalf("%s kind = %q, want execute", name, kind)
		}
	}
}
