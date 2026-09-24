package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

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
	var batch struct {
		Actions []json.RawMessage `json:"actions"`
	}
	if err := decodeArguments(raw, &batch); err != nil {
		return agent.ToolResult{}, err
	}
	if len(batch.Actions) == 0 || len(batch.Actions) > 32 {
		return agent.ToolResult{}, errors.New("computer action batch must contain between one and 32 actions")
	}
	actions := make([]string, 0, len(batch.Actions))
	var window desktop.Window
	for _, rawAction := range batch.Actions {
		var action computerAction
		if err := json.Unmarshal(rawAction, &action); err != nil {
			return agent.ToolResult{}, errors.New("computer action is invalid JSON")
		}
		current, err := t.executeAction(ctx, action)
		if err != nil {
			return agent.ToolResult{}, err
		}
		window = current
		actions = append(actions, action.Type)
	}
	capture, err := t.Controller.Screenshot(ctx, t.SessionID)
	if err != nil {
		return agent.ToolResult{}, err
	}
	content, err := success(struct {
		Window  desktop.Window `json:"window"`
		Actions []string       `json:"actions"`
	}{Window: window, Actions: actions})
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: content, Computer: &agent.ComputerOutput{Screenshot: agent.Image{Name: "desktop-window.png", MediaType: "image/png", Data: capture.PNG}}}, nil
}

type computerAction struct {
	Type    string          `json:"type"`
	Button  string          `json:"button"`
	X       float64         `json:"x"`
	Y       float64         `json:"y"`
	Text    string          `json:"text"`
	Keys    []string        `json:"keys"`
	Path    []desktop.Point `json:"path"`
	ScrollX float64         `json:"scroll_x"`
	ScrollY float64         `json:"scroll_y"`
}

func (t ComputerAction) executeAction(ctx context.Context, action computerAction) (desktop.Window, error) {
	if action.Type == "" {
		return desktop.Window{}, errors.New("computer action type is required")
	}
	var window desktop.Window
	var err error
	switch action.Type {
	case "click":
		if action.Button != "" && action.Button != "left" {
			return desktop.Window{}, errors.New("only left desktop clicks are available")
		}
		if err = t.approve(ctx, "click", fmt.Sprintf("%.1f,%.1f", action.X, action.Y)); err == nil {
			window, err = t.Controller.Click(ctx, t.SessionID, action.X, action.Y)
		}
	case "double_click":
		if action.Button != "" && action.Button != "left" {
			return desktop.Window{}, errors.New("only left desktop double clicks are available")
		}
		if err = t.approve(ctx, "double_click", fmt.Sprintf("%.1f,%.1f", action.X, action.Y)); err == nil {
			window, err = t.Controller.DoubleClick(ctx, t.SessionID, action.X, action.Y)
		}
	case "drag":
		if len(action.Path) < 2 {
			return desktop.Window{}, errors.New("desktop drag requires at least two points")
		}
		if err = t.approve(ctx, "drag", fmt.Sprintf("%d points", len(action.Path))); err == nil {
			window, err = t.Controller.Drag(ctx, t.SessionID, action.Path)
		}
	case "move":
		if err = t.approve(ctx, "move", fmt.Sprintf("%.1f,%.1f", action.X, action.Y)); err == nil {
			window, err = t.Controller.Move(ctx, t.SessionID, action.X, action.Y)
		}
	case "scroll":
		deltaX, validX := computerScrollDelta(action.ScrollX)
		deltaY, validY := computerScrollDelta(action.ScrollY)
		if !validX || !validY {
			return desktop.Window{}, errors.New("computer scroll delta is invalid")
		}
		if err = t.approve(ctx, "scroll", fmt.Sprintf("%.0f,%.0f", action.ScrollX, action.ScrollY)); err == nil {
			window, err = t.Controller.Scroll(ctx, t.SessionID, action.X, action.Y, deltaX, deltaY)
		}
	case "type":
		if err = t.approve(ctx, "type", boundedBrowserApprovalValue(action.Text)); err == nil {
			window, err = t.Controller.Type(ctx, t.SessionID, action.Text)
		}
	case "keypress":
		if len(action.Keys) != 1 {
			return desktop.Window{}, errors.New("computer keypresses with modifiers or key chords are blocked")
		}
		key := canonicalComputerKey(action.Keys[0])
		if key == "" {
			return desktop.Window{}, errors.New("computer keypress is unavailable")
		}
		if err = t.approve(ctx, "keypress", key); err == nil {
			window, err = t.Controller.Press(ctx, t.SessionID, key)
		}
	case "screenshot", "wait":
		window, err = t.Controller.Window(ctx, t.SessionID)
		if err == nil && action.Type == "wait" {
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return desktop.Window{}, ctx.Err()
			case <-timer.C:
			}
		}
	default:
		return desktop.Window{}, fmt.Errorf("computer action %q is unavailable in the local macOS safety policy", action.Type)
	}
	if err != nil {
		return desktop.Window{}, err
	}
	return window, nil
}

func computerScrollDelta(value float64) (int, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value < -10000 || value > 10000 {
		return 0, false
	}
	return int(value), true
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
