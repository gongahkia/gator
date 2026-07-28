package runtime

import (
	"strings"
	"testing"
)

func TestAcceptanceRunnerSupportsPageLevelPressKey(t *testing.T) {
	if !strings.Contains(acceptanceRunner, "target(p,s).press(s.key):p.keyboard.press(s.key)") {
		t.Fatal("page-level press_key is not implemented")
	}
}

func TestAcceptanceRunnerSupportsLocalStorageOperations(t *testing.T) {
	if !strings.Contains(acceptanceRunner, "s.op==='clear'") || !strings.Contains(acceptanceRunner, "localStorage.clear()") || !strings.Contains(acceptanceRunner, "localStorage.setItem(k,v)") {
		t.Fatal("local_storage operations are not implemented")
	}
}
