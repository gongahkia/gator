package llm

import (
	"strings"
	"testing"
)

func TestCLIParserOutputVariants(t *testing.T) {
	parsers := map[string]func(cliResult) (string, error){
		"aider":    parseAiderOutput,
		"codex":    parseCodexOutput,
		"gemini":   parseGeminiOutput,
		"claude":   parseClaudeOutput,
		"goose":    parseGooseOutput,
		"opencode": parseOpenCodeOutput,
	}
	cases := []struct {
		name   string
		stdout string
		want   string
	}{
		{name: "raw text", stdout: "plain answer", want: "plain answer"},
		{name: "json object", stdout: `{"done":true}`, want: `{"done":true}`},
		{name: "wrapped result field", stdout: `{"result":"{\"done\":true}"}`, want: `{"done":true}`},
		{name: "ndjson stream", stdout: strings.Join([]string{
			`{"type":"status","text":"starting"}`,
			`{"type":"message","message":{"content":"{\"done\":true}"}}`,
		}, "\n"), want: `{"done":true}`},
		{name: "tool event noise", stdout: strings.Join([]string{
			`{"type":"tool","content":"noise"}`,
			`{"type":"result","result":"{\"done\":true}"}`,
		}, "\n"), want: `{"done":true}`},
	}
	for parserName, parse := range parsers {
		for _, tc := range cases {
			t.Run(parserName+"/"+tc.name, func(t *testing.T) {
				got, err := parse(cliResult{Stdout: tc.stdout})
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				if got != tc.want {
					t.Fatalf("got = %q; want %q", got, tc.want)
				}
			})
		}
		t.Run(parserName+"/empty error output", func(t *testing.T) {
			_, err := parse(cliResult{Stdout: " \n\t", Stderr: "boom"})
			if err == nil || !strings.Contains(err.Error(), "empty stdout") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
