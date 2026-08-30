package render

import "testing"

func TestTitleTrimsWhitespace(t *testing.T) {
	if got := Title("  Gator  "); got != "Gator" {
		t.Fatalf("Title() = %q", got)
	}
}
