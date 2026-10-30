package cliagent

import (
	"encoding/json"
	"errors"
	"github.com/gongahkia/gator/internal/agent"
	"strings"
)

type streamState struct {
	summary    string
	diagnostic string
	complete   bool
	failed     bool
	turns      int
	usage      agent.Usage
}

func (s *streamState) accept(kind string, line []byte) error {
	var event struct {
		Type    string          `json:"type"`
		Subtype string          `json:"subtype"`
		IsError bool            `json:"is_error"`
		Aborted bool            `json:"aborted"`
		Result  json.RawMessage `json:"result"`
		Item    struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"item"`
		Usage struct {
			Input  int64 `json:"input_tokens"`
			Output int64 `json:"output_tokens"`
		} `json:"usage"`
		Message json.RawMessage `json:"message"`
	}
	if err := json.Unmarshal(line, &event); err != nil {
		return errors.New("CLI emitted malformed JSON")
	}
	switch kind {
	case "codex":
		if event.Type == "error" {
			_ = json.Unmarshal(event.Message, &s.diagnostic)
		}
		if event.Type == "item.completed" && event.Item.Type == "agent_message" {
			s.summary = event.Item.Text
			s.turns++
		}
		if event.Type == "turn.completed" {
			s.complete = true
			s.usage.InputTokens += event.Usage.Input
			s.usage.OutputTokens += event.Usage.Output
			s.usage.Reported = true
		}
		if event.Type == "turn.failed" {
			s.failed = true
		}
	case "claude":
		if event.Type == "assistant" {
			s.turns++
		}
		if event.Type == "result" {
			s.complete = true
			s.failed = event.IsError || event.Subtype != "success"
			if err := json.Unmarshal(event.Result, &s.summary); err != nil && !event.IsError {
				return errors.New("Claude emitted invalid result text")
			}
			s.usage.InputTokens = event.Usage.Input
			s.usage.OutputTokens = event.Usage.Output
			s.usage.Reported = true
		}
	case "pi":
		if event.Type == "message_end" {
			var message struct {
				Error   string          `json:"errorMessage"`
				Role    string          `json:"role"`
				Stop    string          `json:"stopReason"`
				Content json.RawMessage `json:"content"`
				Usage   struct {
					Input      int64 `json:"input"`
					Output     int64 `json:"output"`
					CacheRead  int64 `json:"cacheRead"`
					CacheWrite int64 `json:"cacheWrite"`
				} `json:"usage"`
			}
			if err := json.Unmarshal(event.Message, &message); err != nil {
				return errors.New("CLI emitted invalid message")
			}
			if message.Role != "assistant" {
				return nil
			}
			s.turns++
			s.diagnostic = message.Error
			parts := []string{}
			var content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(message.Content, &content); err != nil {
				return errors.New("CLI emitted invalid assistant content")
			}
			for _, part := range content {
				if part.Type == "text" {
					parts = append(parts, part.Text)
				}
			}
			s.summary = strings.Join(parts, "\n")
			s.failed = message.Stop == "error" || message.Stop == "aborted" || message.Stop == "length"
			s.usage.InputTokens += message.Usage.Input + message.Usage.CacheRead + message.Usage.CacheWrite
			s.usage.OutputTokens += message.Usage.Output
			s.usage.Reported = true
		}
		if event.Type == "agent_settled" {
			s.complete = true
			s.failed = s.failed || event.Aborted
		}
	}
	if len(s.summary) > 32768 {
		return errors.New("CLI summary exceeds 32 KiB")
	}
	if s.turns > 16 {
		return errors.New("CLI exceeded 16 observed assistant messages")
	}
	return nil
}
