package cliagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var nativeUUIDPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func nativeUUID() string {
	token := sessionToken()
	return token[:8] + "-" + token[8:12] + "-4" + token[13:16] + "-8" + token[17:20] + "-" + token[20:]
}

// Claude uses the SDK's documented control envelopes alongside user messages.
// A user-message write has no RPC acknowledgement. Only a terminal result
// counts as turn completion; permission requests are always denied except the
// explicit AskUserQuestion response, which conveys answers without tool grants.
func (c *nativeClient) claudeEvent(raw json.RawMessage) error {
	var event struct {
		Type      string   `json:"type"`
		UUID      string   `json:"uuid"`
		Subtype   string   `json:"subtype"`
		SessionID string   `json:"session_id"`
		RequestID string   `json:"request_id"`
		Result    string   `json:"result"`
		IsError   bool     `json:"is_error"`
		Errors    []string `json:"errors"`
		Request   struct {
			Subtype string `json:"subtype"`
			Tool    string `json:"tool_name"`
			Input   struct {
				Questions json.RawMessage `json:"questions"`
			} `json:"input"`
		} `json:"request"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return err
	}
	c.mu.Lock()
	thread := c.state.ThreadID
	active := c.state.Status == "running" || c.state.Status == "awaiting_input" || c.state.Status == "answering"
	c.mu.Unlock()
	if event.SessionID != "" && event.SessionID != thread {
		return errors.New("Claude event belongs to another session")
	}
	switch event.Type {
	case "result":
		if event.UUID == "" {
			return errors.New("Claude terminal result has no identity")
		}
		c.mu.Lock()
		if c.retiredTurns[event.UUID] {
			c.mu.Unlock()
			return nil
		}
		if c.retiredTurns == nil {
			c.retiredTurns = map[string]bool{}
		}
		c.retiredTurns[event.UUID] = true
		c.mu.Unlock()
		if !active || event.SessionID == "" {
			return errors.New("unexpected Claude terminal result")
		}
		if len(event.Result) > 32768 {
			return errors.New("Claude summary exceeds 32 KiB")
		}
		status, diagnostic := "completed", ""
		if event.IsError || event.Subtype != "success" {
			status = "failed"
			diagnostic = strings.Join(event.Errors, "; ")
			if diagnostic == "" {
				diagnostic = event.Result
			}
			if diagnostic == "" {
				diagnostic = event.Subtype
			}
			if len(diagnostic) > 2048 {
				diagnostic = "Claude error detail exceeds limit"
			}
		}
		c.mu.Lock()
		c.state.Summary = event.Result
		if c.interrupting {
			status = "cancelled"
		}
		c.mu.Unlock()
		c.finish(status, diagnostic)
	case "control_request":
		if !active || event.RequestID == "" || len(event.RequestID) > 256 {
			return errors.New("invalid Claude control request")
		}
		if event.Request.Subtype != "can_use_tool" {
			return fmt.Errorf("unsupported Claude control request %q", event.Request.Subtype)
		}
		id, _ := json.Marshal(event.RequestID)
		if event.Request.Tool != "AskUserQuestion" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return c.wire.Reply(ctx, id, map[string]any{"behavior": "deny", "message": "Additional tool permissions are not granted by Gator."})
		}
		var questions []struct {
			Text    string `json:"question"`
			Secret  bool   `json:"isSecret"`
			Options []struct {
				Label string `json:"label"`
			} `json:"options"`
		}
		if json.Unmarshal(event.Request.Input.Questions, &questions) != nil || len(questions) == 0 || len(questions) > 4 {
			return errors.New("invalid Claude question count")
		}
		seen := map[string]bool{}
		pending := make([]NativeQuestion, 0, len(questions))
		for i, q := range questions {
			if q.Secret || seen[q.Text] || strings.TrimSpace(q.Text) == "" || len(q.Text) > 8192 || len(q.Options) > 8 {
				return errors.New("invalid or secret Claude question")
			}
			seen[q.Text] = true
			value := NativeQuestion{ID: "question-" + strconv.Itoa(i+1), Text: q.Text}
			for _, option := range q.Options {
				if len(option.Label) > 8192 {
					return errors.New("Claude option exceeds limit")
				}
				value.Options = append(value.Options, option.Label)
			}
			pending = append(pending, value)
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.questionID != nil {
			return errors.New("overlapping Claude questions")
		}
		c.questionID = id
		c.questionInput = append(json.RawMessage(nil), event.Request.Input.Questions...)
		c.state.QuestionID = sessionDigest([]byte(sessionToken()))
		c.state.Questions = pending
		c.state.Status = "awaiting_input"
	case "control_cancel_request":
		id, _ := json.Marshal(event.RequestID)
		c.mu.Lock()
		defer c.mu.Unlock()
		if string(c.questionID) == string(id) {
			c.questionID, c.questionInput = nil, nil
			c.state.Questions, c.state.QuestionID = nil, ""
			c.state.Status = "running"
		}
	case "system":
		if event.Subtype == "task_started" {
			return errors.New("Claude started background work outside its assigned capability")
		}
	}
	return nil
}
