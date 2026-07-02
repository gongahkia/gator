package compress

import (
	"encoding/json"
	"strings"

	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm"
)

func droneMessages(env *envelope.Envelope, schema []byte) []llm.ChatMessage {
	return []llm.ChatMessage{
		{Role: "system", Content: droneSystemPrompt(string(schema))},
		{Role: "user", Content: droneUserPrompt(env)},
	}
}

func droneSystemPrompt(schemaText string) string {
	return strings.Join([]string{
		"You are the context compression drone.",
		"Only score relevance, extract minimal verbatim spans, and return JSON matching the schema.",
		"Never invent paths, symbols, line numbers, or quotes.",
		"Every quote must be copied exactly from a raw unit.",
		"Schema:",
		schemaText,
	}, "\n")
}

func droneUserPrompt(env *envelope.Envelope) string {
	raw, _ := json.Marshal(env.Raw)
	return strings.Join([]string{
		"Instruction:",
		env.Instruction,
		"",
		"RawContext JSON:",
		string(raw),
	}, "\n")
}
