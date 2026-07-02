package plan

import (
	"encoding/json"
	"strings"

	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm"
)

func planMessages(env *envelope.Envelope) []llm.ChatMessage {
	return []llm.ChatMessage{
		{Role: "system", Content: planSystemPrompt()},
		{Role: "user", Content: planUserPrompt(env)},
	}
}

func planSystemPrompt() string {
	return strings.Join([]string{
		"You are the brain planning stage.",
		"Choose exactly one concrete next action, or mark done.",
		"Do not make a long-horizon plan.",
		"Use only the provided ContextDigest and prior VerifyResult.",
		"Never assume access to RawContext.",
		"Return only JSON matching the provided plan schema.",
	}, "\n")
}

func planUserPrompt(env *envelope.Envelope) string {
	digest, _ := json.Marshal(env.Digest)
	verify, _ := json.Marshal(env.Verify)
	return strings.Join([]string{
		"Instruction:",
		env.Instruction,
		"",
		"ContextDigest JSON:",
		string(digest),
		"",
		"Prior VerifyResult JSON:",
		string(verify),
	}, "\n")
}
