package llm

import (
	"strings"
	"sync"

	"github.com/tiktoken-go/tokenizer"
)

const (
	TokenSourceProvider = "provider"
	TokenSourceEstimate = "estimate"
)

var (
	estimateCodec     tokenizer.Codec
	estimateCodecErr  error
	estimateCodecOnce sync.Once
	estimateCodecLoad = func() (tokenizer.Codec, error) {
		return tokenizer.Get(tokenizer.Cl100kBase)
	}
)

func Estimate(text string) int {
	codec, err := loadEstimateCodec()
	if err != nil || codec == nil {
		return fallbackEstimate(text)
	}
	tokens, _, err := codec.Encode(text)
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

func loadEstimateCodec() (tokenizer.Codec, error) {
	estimateCodecOnce.Do(func() {
		estimateCodec, estimateCodecErr = estimateCodecLoad()
	})
	return estimateCodec, estimateCodecErr
}

func fallbackEstimate(text string) int {
	if text == "" {
		return 0
	}
	return len([]rune(text))/4 + 1
}
