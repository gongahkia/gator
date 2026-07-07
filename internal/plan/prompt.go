package plan

import (
	"encoding/json"
	"strings"

	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm"
)

func planMessages(env *envelope.Envelope, useRawContext bool) []llm.ChatMessage {
	return []llm.ChatMessage{
		{Role: "system", Content: planSystemPrompt(useRawContext)},
		{Role: "user", Content: planUserPrompt(env, useRawContext)},
	}
}

func planSystemPrompt(useRawContext bool) string {
	contextRule := "Use only the provided ContextDigest and prior VerifyResult."
	if useRawContext {
		contextRule = "Use only the provided RawContext and prior VerifyResult."
	}
	return strings.Join([]string{
		"You are the brain planning stage.",
		"Choose exactly one concrete next action, or mark done.",
		"Do not make a long-horizon plan.",
		contextRule,
		"Return only JSON matching the provided plan schema.",
	}, "\n")
}

func planUserPrompt(env *envelope.Envelope, useRawContext bool) string {
	contextLabel := "ContextDigest JSON:"
	contextValue, _ := json.Marshal(env.Digest)
	if useRawContext {
		contextLabel = "RawContext JSON:"
		contextValue, _ = json.Marshal(env.Raw)
	}
	verify, _ := json.Marshal(env.Verify)
	return strings.Join([]string{
		"Instruction:",
		env.Instruction,
		"",
		contextLabel,
		string(contextValue),
		"",
		"Prior VerifyResult JSON:",
		string(verify),
	}, "\n")
}
