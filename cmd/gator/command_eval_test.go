package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestEvalCommandRequiresFixtureDirectory(t *testing.T) {
	var output bytes.Buffer
	err := evalCommand(nil, &output)
	if err == nil || !strings.Contains(err.Error(), "usage: gator eval") {
		t.Fatalf("eval error = %v", err)
	}
}
