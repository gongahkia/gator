package acp

import "testing"

func TestACPToolKindClassifiesBoundedBrowserOperations(t *testing.T) {
	for _, name := range []string{"browser_tabs", "browser_snapshot", "browser_screenshot", "browser_extract"} {
		if kind := acpToolKind(name); kind != "read" {
			t.Fatalf("%s kind = %q, want read", name, kind)
		}
	}
	for _, name := range []string{"browser_navigate", "browser_click", "browser_fill", "browser_select", "browser_press", "browser_download", "browser_upload", "browser_act"} {
		if kind := acpToolKind(name); kind != "execute" {
			t.Fatalf("%s kind = %q, want execute", name, kind)
		}
	}
}
