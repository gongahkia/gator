package llm

import (
	"strings"

	"github.com/tiktoken-go/tokenizer"
)

const (
	TokenSourceProvider = "provider"
	TokenSourceEstimate = "estimate"
)

var estimateCodec = mustEstimateCodec()

func Estimate(text string) int {
	tokens, _, err := estimateCodec.Encode(text)
	if err != nil {
		return fallbackEstimate(text)
	}
	return len(tokens)
}

func usageWithEstimate(usage Usage, inputText, outputText string) Usage {
	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		usage.TokenSource = TokenSourceProvider
		return usage
	}
	return Usage{
		InputTokens:  Estimate(inputText),
		OutputTokens: Estimate(outputText),
		TokenSource:  TokenSourceEstimate,
	}
}

func messageText(messages []ChatMessage) string {
	var b strings.Builder
	for _, msg := range messages {
		b.WriteString(msg.Role)
		b.WriteByte('\n')
		b.WriteString(msg.Content)
		b.WriteByte('\n')
	}
	return b.String()
}

func mustEstimateCodec() tokenizer.Codec {
	codec, err := tokenizer.Get(tokenizer.Cl100kBase)
	if err != nil {
		panic(err)
	}
	return codec
}

func fallbackEstimate(text string) int {
	if text == "" {
		return 0
	}
	return len([]rune(text))/4 + 1
}
