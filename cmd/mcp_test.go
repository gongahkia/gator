package cmd

import (
	"strings"
	"testing"
)

func TestMCPServeListsTools(t *testing.T) {
	isolateEnv(t)
	out := executeRoot(t, []string{"mcp", "serve"}, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n")
	for _, want := range []string{`"name":"gather"`, `"name":"compress"`, `"name":"digest"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %s:\n%s", want, out)
		}
	}
}
