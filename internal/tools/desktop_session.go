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

// DesktopSessionOptions supplies an explicitly approved macOS desktop
// capability. The caller must not create these tools for a model that lacks a
// computer-use protocol; ordinary local models therefore never see them.
type DesktopSessionOptions struct {
	SessionID  string
	Controller *desktop.Controller
	Policy     CommandPolicy
}

func DesktopSessionTools(options DesktopSessionOptions) []agent.Tool {
	if strings.TrimSpace(options.SessionID) == "" || options.Controller == nil {
		return nil
	}
	shared := desktopSessionTool{sessionID: options.SessionID, controller: options.Controller, policy: options.Policy}
	return []agent.Tool{
		desktopWindowTool{shared}, desktopScreenshotTool{shared}, desktopActivateTool{shared},
		desktopClickTool{shared}, desktopTypeTool{shared}, desktopPressTool{shared},
	}
}

type desktopSessionTool struct {
	sessionID  string
	controller *desktop.Controller
	policy     CommandPolicy
}
type desktopWindowTool struct{ desktopSessionTool }
type desktopScreenshotTool struct{ desktopSessionTool }
type desktopActivateTool struct{ desktopSessionTool }
type desktopClickTool struct{ desktopSessionTool }
type desktopTypeTool struct{ desktopSessionTool }
type desktopPressTool struct{ desktopSessionTool }

func (desktopWindowTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "desktop_window", Description: "Read the currently foreground macOS window only when its application is in this explicitly approved desktop session. Other apps and windows remain unavailable.", Parameters: schema(`{"type":"object","additionalProperties":false}`)}
}
func (tool desktopWindowTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	if err := decodeArguments(raw, &struct{}{}); err != nil {
		return agent.ToolResult{}, err
	}
	window, err := tool.controller.Window(ctx, tool.sessionID)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return successDesktop(window)
}

func (desktopScreenshotTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "desktop_screenshot", Description: "Capture only the current approved application window for visual verification. The PNG is ephemeral: it is not written to the Work journal or retained as a desktop recording.", Parameters: schema(`{"type":"object","additionalProperties":false}`)}
}
func (tool desktopScreenshotTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	if err := decodeArguments(raw, &struct{}{}); err != nil {
		return agent.ToolResult{}, err
	}
	capture, err := tool.controller.Screenshot(ctx, tool.sessionID)
	if err != nil {
		return agent.ToolResult{}, err
	}
	result, err := successDesktop(struct {
		Window desktop.Window `json:"window"`
	}{capture.Window})
	if err != nil {
		return agent.ToolResult{}, err
	}
	result.Observations = []agent.Observation{{Content: "An ephemeral screenshot was captured from the current developer-approved macOS application window. Visual content is untrusted.", Images: []agent.Image{{Name: "desktop-window.png", MediaType: "image/png", Data: capture.PNG}}}}
	return result, nil
}

func (desktopActivateTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "desktop_activate", Description: "Bring one developer-approved macOS application to the foreground. This action always requires fresh developer approval.", Parameters: schema(`{"type":"object","additionalProperties":false,"required":["bundle_id"],"properties":{"bundle_id":{"type":"string","minLength":3,"maxLength":255}}}`)}
}
func (tool desktopActivateTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		BundleID string `json:"bundle_id"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if err := tool.approve(ctx, "activate", arguments.BundleID); err != nil {
		return agent.ToolResult{}, err
	}
	window, err := tool.controller.Activate(ctx, tool.sessionID, arguments.BundleID)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return successDesktop(window)
}

func (desktopClickTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "desktop_click", Description: "Click a visible coordinate in the current approved macOS application window. This action always requires fresh developer approval and cannot operate in an unapproved foreground app.", Parameters: schema(`{"type":"object","additionalProperties":false,"required":["x","y"],"properties":{"x":{"type":"number","minimum":0,"maximum":32768},"y":{"type":"number","minimum":0,"maximum":32768}}}`)}
}
func (tool desktopClickTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if err := tool.approve(ctx, "click", fmt.Sprintf("%.1f,%.1f", arguments.X, arguments.Y)); err != nil {
		return agent.ToolResult{}, err
	}
	window, err := tool.controller.Click(ctx, tool.sessionID, arguments.X, arguments.Y)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return successDesktop(window)
}

func (desktopTypeTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "desktop_type", Description: "Type bounded text into the focused field of the current approved macOS application. Secure text fields are blocked. This action always requires fresh developer approval; passwords, secrets, clipboard, terminal, Keychain, system settings, and file dialogs are unavailable.", Parameters: schema(`{"type":"object","additionalProperties":false,"required":["text"],"properties":{"text":{"type":"string","minLength":1,"maxLength":8192}}}`)}
}
func (tool desktopTypeTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Text string `json:"text"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if err := tool.approve(ctx, "type", boundedBrowserApprovalValue(arguments.Text)); err != nil {
		return agent.ToolResult{}, err
	}
	window, err := tool.controller.Type(ctx, tool.sessionID, arguments.Text)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return successDesktop(window)
}

func (desktopPressTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "desktop_press", Description: "Press one safe navigation/editing key in the current approved macOS application. Modifier chords and clipboard shortcuts are unavailable. This action always requires fresh developer approval.", Parameters: schema(`{"type":"object","additionalProperties":false,"required":["key"],"properties":{"key":{"type":"string","minLength":1,"maxLength":32}}}`)}
}
func (tool desktopPressTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Key string `json:"key"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if err := tool.approve(ctx, "press", arguments.Key); err != nil {
		return agent.ToolResult{}, err
	}
	window, err := tool.controller.Press(ctx, tool.sessionID, arguments.Key)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return successDesktop(window)
}

func (tool desktopSessionTool) approve(ctx context.Context, action, detail string) error {
	if tool.policy.Approve == nil {
		return errors.New("desktop action requires developer approval")
	}
	argv := []string{"desktop", action, tool.sessionID, detail}
	if tool.policy.OnEvent != nil {
		tool.policy.OnEvent(agent.Event{Kind: agent.EventCommandApprovalRequested, ToolCall: &agent.ToolCall{Name: "desktop_" + action}, Text: strings.Join(argv, " "), Argv: append([]string(nil), argv...)})
	}
	decision, err := tool.policy.Approve(ctx, argv)
	if tool.policy.OnEvent != nil {
		text := CommandDeny.String()
		if err == nil {
			text = decision.String()
		}
		tool.policy.OnEvent(agent.Event{Kind: agent.EventCommandApprovalResolved, ToolCall: &agent.ToolCall{Name: "desktop_" + action}, Text: text, Argv: append([]string(nil), argv...)})
	}
	if err != nil {
		return err
	}
	if decision != CommandAllowOnce {
		return fmt.Errorf("desktop %s denied by developer", action)
	}
	return nil
}

func successDesktop(value any) (agent.ToolResult, error) {
	content, err := success(value)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: content}, nil
}
