package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/desktop"
)

// ComputerAction is the local executor for a provider-native computer_call.
// It intentionally exposes no function schema to a CUA provider; its name is
// used only by the generic runner to bind a decoded computer_call to trusted
// macOS action code.
type ComputerAction struct {
	SessionID  string
	Controller *desktop.Controller
	Policy     CommandPolicy
}

func (t ComputerAction) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "computer_action", Description: "Internal provider-native computer action executor.", Parameters: schema(`{"type":"object"}`)}
}

func (t ComputerAction) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	if t.Controller == nil || strings.TrimSpace(t.SessionID) == "" {
		return agent.ToolResult{}, errors.New("desktop session is unavailable")
	}
	var action struct {
		Type string   `json:"type"`
		X    float64  `json:"x"`
		Y    float64  `json:"y"`
		Text string   `json:"text"`
		Keys []string `json:"keys"`
	}
	if err := decodeArguments(raw, &action); err != nil {
		return agent.ToolResult{}, err
	}
	if action.Type == "" {
		return agent.ToolResult{}, errors.New("computer action type is required")
	}
	var window desktop.Window
	var err error
	switch action.Type {
	case "click":
		if err = t.approve(ctx, "click", fmt.Sprintf("%.1f,%.1f", action.X, action.Y)); err == nil {
			window, err = t.Controller.Click(ctx, t.SessionID, action.X, action.Y)
		}
	case "type":
		if err = t.approve(ctx, "type", boundedBrowserApprovalValue(action.Text)); err == nil {
			window, err = t.Controller.Type(ctx, t.SessionID, action.Text)
		}
	case "keypress":
		if len(action.Keys) != 1 {
			return agent.ToolResult{}, errors.New("computer keypresses with modifiers or key chords are blocked")
		}
		key := canonicalComputerKey(action.Keys[0])
		if key == "" {
			return agent.ToolResult{}, errors.New("computer keypress is unavailable")
		}
		if err = t.approve(ctx, "keypress", key); err == nil {
			window, err = t.Controller.Press(ctx, t.SessionID, key)
		}
	case "screenshot", "wait":
		window, err = t.Controller.Window(ctx, t.SessionID)
	default:
		return agent.ToolResult{}, fmt.Errorf("computer action %q is unavailable in the local macOS safety policy", action.Type)
	}
	if err != nil {
		return agent.ToolResult{}, err
	}
	capture, err := t.Controller.Screenshot(ctx, t.SessionID)
	if err != nil {
		return agent.ToolResult{}, err
	}
	content, err := success(struct {
		Window desktop.Window `json:"window"`
		Action string         `json:"action"`
	}{Window: window, Action: action.Type})
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: content, Computer: &agent.ComputerOutput{Screenshot: agent.Image{Name: "desktop-window.png", MediaType: "image/png", Data: capture.PNG}}}, nil
}

func (t ComputerAction) approve(ctx context.Context, action, detail string) error {
	if t.Policy.Approve == nil {
		return errors.New("computer action requires developer approval")
	}
	argv := []string{"desktop", action, t.SessionID, detail}
	if t.Policy.OnEvent != nil {
		t.Policy.OnEvent(agent.Event{Kind: agent.EventCommandApprovalRequested, ToolCall: &agent.ToolCall{Name: "computer_action", Kind: agent.ToolCallComputer}, Text: strings.Join(argv, " "), Argv: append([]string(nil), argv...)})
	}
	decision, err := t.Policy.Approve(ctx, argv)
	if t.Policy.OnEvent != nil {
		text := CommandDeny.String()
		if err == nil {
			text = decision.String()
		}
		t.Policy.OnEvent(agent.Event{Kind: agent.EventCommandApprovalResolved, ToolCall: &agent.ToolCall{Name: "computer_action", Kind: agent.ToolCallComputer}, Text: text, Argv: append([]string(nil), argv...)})
	}
	if err != nil {
		return err
	}
	if decision != CommandAllowOnce {
		return fmt.Errorf("computer %s denied by developer", action)
	}
	return nil
}

func canonicalComputerKey(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "ENTER", "RETURN":
		return "Enter"
	case "TAB":
		return "Tab"
	case "ESC", "ESCAPE":
		return "Escape"
	case "BACKSPACE":
		return "Backspace"
	case "DELETE":
		return "Delete"
	case "ARROWUP", "UP":
		return "Up"
	case "ARROWDOWN", "DOWN":
		return "Down"
	case "ARROWLEFT", "LEFT":
		return "Left"
	case "ARROWRIGHT", "RIGHT":
		return "Right"
	case "SPACE":
		return "Space"
	case "HOME":
		return "Home"
	case "END":
		return "End"
	case "PAGEUP":
		return "PageUp"
	case "PAGEDOWN":
		return "PageDown"
	default:
		return ""
	}
}
