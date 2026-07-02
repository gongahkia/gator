package llm

import (
	"context"
	"encoding/json"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string
	Messages    []ChatMessage
	Temperature float64
	MaxTokens   int
	JSONSchema  json.RawMessage
}

type Usage struct {
	InputTokens  int
	OutputTokens int
}

type ChatResponse struct {
	Content string
	Usage   Usage
}

type Client interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}
