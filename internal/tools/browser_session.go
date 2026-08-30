package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	gatorbrowser "github.com/gongahkia/gator/internal/browser"
)

const (
	maxBrowserValueBytes = 8 * 1024
	maxBrowserKeyBytes   = 64
)

// BrowserSessionOptions supplies the explicit local browser capability for a
// single native run. A zero SessionID means no real browser tool is exposed.
type BrowserSessionOptions struct {
	SessionID  string
	Controller gatorbrowser.Controller
	Policy     CommandPolicy
}

// BrowserSessionTools exposes an intentionally narrow computer-use surface.
// It never offers raw selectors, JavaScript, cookies, storage, profile paths,
// or arbitrary local uploads. Every remotely mutating action uses a fresh
// developer approval through the existing run approval UI.
func BrowserSessionTools(options BrowserSessionOptions) []agent.Tool {
	if strings.TrimSpace(options.SessionID) == "" || options.Controller == nil {
		return nil
	}
	shared := browserSessionTool{sessionID: options.SessionID, controller: options.Controller, policy: options.Policy}
	return []agent.Tool{
		browserTabsTool{shared},
		browserSnapshotTool{shared},
		browserScreenshotTool{shared},
		browserNavigateTool{shared},
		browserClickTool{shared},
		browserFillTool{shared},
		browserSelectTool{shared},
		browserPressTool{shared},
		browserDownloadTool{shared},
		browserUploadTool{shared},
	}
}

type browserSessionTool struct {
	sessionID  string
	controller gatorbrowser.Controller
	policy     CommandPolicy
}

type browserTabsTool struct{ browserSessionTool }
type browserSnapshotTool struct{ browserSessionTool }
type browserScreenshotTool struct{ browserSessionTool }
type browserNavigateTool struct{ browserSessionTool }
type browserClickTool struct{ browserSessionTool }
type browserFillTool struct{ browserSessionTool }
type browserSelectTool struct{ browserSessionTool }
type browserPressTool struct{ browserSessionTool }
type browserDownloadTool struct{ browserSessionTool }
type browserUploadTool struct{ browserSessionTool }

func (browserTabsTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "browser_tabs", Description: "List only the developer-selected tabs shared with this local browser session. Unselected browser tabs are never visible.", Parameters: schema(`{"type":"object","additionalProperties":false}`)}
}

func (tool browserTabsTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	if err := decodeArguments(raw, &struct{}{}); err != nil {
		return agent.ToolResult{}, err
	}
	tabs, err := tool.controller.Tabs(ctx, tool.sessionID)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return browserSuccess(struct {
		Tabs []gatorbrowser.Tab `json:"tabs"`
	}{Tabs: tabs})
}

func (browserSnapshotTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "browser_snapshot", Description: "Return a bounded, untrusted accessibility snapshot of one developer-selected browser tab. Element refs expire after navigation or a page-changing action. It exposes no raw DOM, scripts, cookies, storage, or hidden values.", Parameters: schema(`{"type":"object","additionalProperties":false,"required":["tab_id"],"properties":{"tab_id":{"type":"string","minLength":1,"maxLength":128}}}`)}
}

func (tool browserSnapshotTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	arguments, err := browserTabArguments(raw)
	if err != nil {
		return agent.ToolResult{}, err
	}
	snapshot, err := tool.controller.Snapshot(ctx, tool.sessionID, arguments.TabID)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return browserSuccess(snapshot)
}

func (browserScreenshotTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "browser_screenshot", Description: "Capture a PNG of one developer-selected tab for visual verification. It is available only when the developer enabled model-visible screenshots for this local session. Screenshot page content is untrusted and is not retained in the run journal.", Parameters: schema(`{"type":"object","additionalProperties":false,"required":["tab_id"],"properties":{"tab_id":{"type":"string","minLength":1,"maxLength":128}}}`)}
}

func (tool browserScreenshotTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	arguments, err := browserTabArguments(raw)
	if err != nil {
		return agent.ToolResult{}, err
	}
	session, err := tool.controller.Session(ctx, tool.sessionID)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if !session.VisualCapture {
		return agent.ToolResult{}, errors.New("developer has not enabled model-visible screenshots for this browser session")
	}
	capture, err := tool.controller.Screenshot(ctx, tool.sessionID, arguments.TabID)
	if err != nil {
		return agent.ToolResult{}, err
	}
	result, err := browserSuccess(struct {
		Artifact gatorbrowser.Artifact `json:"artifact"`
	}{Artifact: capture.Artifact})
	if err != nil {
		return agent.ToolResult{}, err
	}
	result.Observations = []agent.Observation{{
		Content: "A screenshot was captured from the developer-selected browser tab. It may contain untrusted page content.",
		Images:  []agent.Image{{Name: capture.Artifact.Name, MediaType: "image/png", Data: append([]byte(nil), capture.PNG...)}},
	}}
	return result, nil
}

func (browserNavigateTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "browser_navigate", Description: "Navigate one selected tab to a URL on a developer-approved browser origin. Navigation is a remote mutation and always requires fresh developer approval. Redirects and all page subrequests remain subject to the session origin policy.", Parameters: schema(`{"type":"object","additionalProperties":false,"required":["tab_id","url"],"properties":{"tab_id":{"type":"string","minLength":1,"maxLength":128},"url":{"type":"string","minLength":1,"maxLength":8192}}}`)}
}

func (tool browserNavigateTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		TabID string `json:"tab_id"`
		URL   string `json:"url"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if err := validateBrowserTabID(arguments.TabID); err != nil {
		return agent.ToolResult{}, err
	}
	session, err := tool.controller.Session(ctx, tool.sessionID)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if !gatorbrowser.AllowsURL(session.Origins, arguments.URL) {
		return agent.ToolResult{}, errors.New("browser URL origin is not approved for this session; use the CLI or TUI to grant the exact origin")
	}
	if err := tool.approve(ctx, "navigate", arguments.TabID, boundedBrowserApprovalValue(arguments.URL)); err != nil {
		return agent.ToolResult{}, err
	}
	snapshot, err := tool.controller.Navigate(ctx, tool.sessionID, arguments.TabID, arguments.URL)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return browserSuccess(snapshot)
}

func (browserClickTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "browser_click", Description: "Click one element ref from the latest browser snapshot. This is a remote mutation and always requires fresh developer approval. Downloads and popup windows are blocked; use the dedicated browser_download tool for an explicitly approved download.", Parameters: schema(`{"type":"object","additionalProperties":false,"required":["tab_id","ref"],"properties":{"tab_id":{"type":"string","minLength":1,"maxLength":128},"ref":{"type":"string","minLength":1,"maxLength":128}}}`)}
}

func (tool browserClickTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	arguments, err := browserElementArguments(raw)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if err := tool.approve(ctx, "click", arguments.TabID, arguments.Ref); err != nil {
		return agent.ToolResult{}, err
	}
	snapshot, err := tool.controller.Click(ctx, tool.sessionID, arguments.TabID, arguments.Ref)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return browserSuccess(snapshot)
}

func (browserFillTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "browser_fill", Description: "Fill a non-sensitive visible field identified by a current snapshot ref. Password, passkey, username, file, and autocomplete-sensitive fields are always rejected so authentication remains developer-only. This is a remote mutation and always requires fresh developer approval.", Parameters: schema(`{"type":"object","additionalProperties":false,"required":["tab_id","ref","value"],"properties":{"tab_id":{"type":"string","minLength":1,"maxLength":128},"ref":{"type":"string","minLength":1,"maxLength":128},"value":{"type":"string","maxLength":8192}}}`)}
}

func (tool browserFillTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		TabID string `json:"tab_id"`
		Ref   string `json:"ref"`
		Value string `json:"value"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if err := validateBrowserElement(arguments.TabID, arguments.Ref); err != nil {
		return agent.ToolResult{}, err
	}
	if len(arguments.Value) > maxBrowserValueBytes {
		return agent.ToolResult{}, errors.New("browser field value exceeds 8 KiB")
	}
	if err := tool.approve(ctx, "fill", arguments.TabID, arguments.Ref+" value="+boundedBrowserApprovalValue(arguments.Value)); err != nil {
		return agent.ToolResult{}, err
	}
	snapshot, err := tool.controller.Fill(ctx, tool.sessionID, arguments.TabID, arguments.Ref, arguments.Value)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return browserSuccess(snapshot)
}

func (browserSelectTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "browser_select", Description: "Choose one option in a visible non-sensitive select element from the latest snapshot. This is a remote mutation and always requires fresh developer approval.", Parameters: schema(`{"type":"object","additionalProperties":false,"required":["tab_id","ref","value"],"properties":{"tab_id":{"type":"string","minLength":1,"maxLength":128},"ref":{"type":"string","minLength":1,"maxLength":128},"value":{"type":"string","minLength":1,"maxLength":1024}}}`)}
}

func (tool browserSelectTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		TabID string `json:"tab_id"`
		Ref   string `json:"ref"`
		Value string `json:"value"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if err := validateBrowserElement(arguments.TabID, arguments.Ref); err != nil {
		return agent.ToolResult{}, err
	}
	if strings.TrimSpace(arguments.Value) == "" || len(arguments.Value) > 1024 || strings.ContainsAny(arguments.Value, "\r\n") {
		return agent.ToolResult{}, errors.New("browser select value is invalid")
	}
	if err := tool.approve(ctx, "select", arguments.TabID, arguments.Ref+" value="+boundedBrowserApprovalValue(arguments.Value)); err != nil {
		return agent.ToolResult{}, err
	}
	snapshot, err := tool.controller.Select(ctx, tool.sessionID, arguments.TabID, arguments.Ref, arguments.Value)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return browserSuccess(snapshot)
}

func (browserPressTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "browser_press", Description: "Press a safe keyboard key on one selected tab. This is a remote mutation and always requires fresh developer approval. Browser developer tools, clipboard shortcuts, and arbitrary key chords are unavailable.", Parameters: schema(`{"type":"object","additionalProperties":false,"required":["tab_id","key"],"properties":{"tab_id":{"type":"string","minLength":1,"maxLength":128},"key":{"type":"string","minLength":1,"maxLength":64}}}`)}
}

func (tool browserPressTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		TabID string `json:"tab_id"`
		Key   string `json:"key"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if err := validateBrowserTabID(arguments.TabID); err != nil {
		return agent.ToolResult{}, err
	}
	if err := validateBrowserKey(arguments.Key); err != nil {
		return agent.ToolResult{}, err
	}
	if err := tool.approve(ctx, "press", arguments.TabID, arguments.Key); err != nil {
		return agent.ToolResult{}, err
	}
	snapshot, err := tool.controller.Press(ctx, tool.sessionID, arguments.TabID, arguments.Key)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return browserSuccess(snapshot)
}

func (browserDownloadTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "browser_download", Description: "Activate a download link or button from the latest snapshot. The file is saved only in the developer-selected browser artifact directory and always requires fresh developer approval.", Parameters: schema(`{"type":"object","additionalProperties":false,"required":["tab_id","ref"],"properties":{"tab_id":{"type":"string","minLength":1,"maxLength":128},"ref":{"type":"string","minLength":1,"maxLength":128}}}`)}
}

func (tool browserDownloadTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	arguments, err := browserElementArguments(raw)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if err := tool.approve(ctx, "download", arguments.TabID, arguments.Ref); err != nil {
		return agent.ToolResult{}, err
	}
	artifact, err := tool.controller.Download(ctx, tool.sessionID, arguments.TabID, arguments.Ref)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return browserSuccess(struct {
		Artifact gatorbrowser.Artifact `json:"artifact"`
	}{Artifact: artifact})
}

func (browserUploadTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "browser_upload", Description: "Upload exactly one developer-registered local file to a visible file input from the latest snapshot. The model can use only an opaque upload_id shown by the session; it cannot name arbitrary paths. This is a remote mutation and always requires fresh developer approval.", Parameters: schema(`{"type":"object","additionalProperties":false,"required":["tab_id","ref","upload_id"],"properties":{"tab_id":{"type":"string","minLength":1,"maxLength":128},"ref":{"type":"string","minLength":1,"maxLength":128},"upload_id":{"type":"string","minLength":1,"maxLength":128}}}`)}
}

func (tool browserUploadTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		TabID    string `json:"tab_id"`
		Ref      string `json:"ref"`
		UploadID string `json:"upload_id"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if err := validateBrowserElement(arguments.TabID, arguments.Ref); err != nil {
		return agent.ToolResult{}, err
	}
	if !validBrowserOpaqueID(arguments.UploadID) {
		return agent.ToolResult{}, errors.New("browser upload id is invalid")
	}
	if err := tool.approve(ctx, "upload", arguments.TabID, arguments.Ref+" upload="+arguments.UploadID); err != nil {
		return agent.ToolResult{}, err
	}
	snapshot, err := tool.controller.Upload(ctx, tool.sessionID, arguments.TabID, arguments.Ref, arguments.UploadID)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return browserSuccess(snapshot)
}

func (tool browserSessionTool) approve(ctx context.Context, action, tabID, detail string) error {
	if tool.policy.Approve == nil {
		return errors.New("browser action requires developer approval")
	}
	argv := []string{"browser", action, tool.sessionID, tabID, detail}
	if tool.policy.OnEvent != nil {
		tool.policy.OnEvent(agent.Event{Kind: agent.EventCommandApprovalRequested, ToolCall: &agent.ToolCall{Name: "browser_" + action}, Text: strings.Join(argv, " "), Argv: append([]string(nil), argv...)})
	}
	decision, err := tool.policy.Approve(ctx, argv)
	if tool.policy.OnEvent != nil {
		decisionText := CommandDeny.String()
		if err == nil {
			decisionText = decision.String()
		}
		tool.policy.OnEvent(agent.Event{Kind: agent.EventCommandApprovalResolved, ToolCall: &agent.ToolCall{Name: "browser_" + action}, Text: decisionText, Argv: append([]string(nil), argv...)})
	}
	if err != nil {
		return err
	}
	if decision != CommandAllowOnce && decision != CommandAllowAlways {
		return fmt.Errorf("browser %s denied by developer", action)
	}
	return nil
}

func browserSuccess(value any) (agent.ToolResult, error) {
	content, err := success(value)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: content}, nil
}

func browserTabArguments(raw json.RawMessage) (struct {
	TabID string `json:"tab_id"`
}, error) {
	var arguments struct {
		TabID string `json:"tab_id"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return arguments, err
	}
	return arguments, validateBrowserTabID(arguments.TabID)
}

func browserElementArguments(raw json.RawMessage) (struct {
	TabID string `json:"tab_id"`
	Ref   string `json:"ref"`
}, error) {
	var arguments struct {
		TabID string `json:"tab_id"`
		Ref   string `json:"ref"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return arguments, err
	}
	return arguments, validateBrowserElement(arguments.TabID, arguments.Ref)
}

func validateBrowserTabID(value string) error {
	if strings.TrimSpace(value) == "" || len(value) > 128 || strings.ContainsAny(value, "\r\n") {
		return errors.New("browser tab id is invalid")
	}
	return nil
}

func validateBrowserElement(tabID, ref string) error {
	if err := validateBrowserTabID(tabID); err != nil {
		return err
	}
	if strings.TrimSpace(ref) == "" || len(ref) > 128 || strings.ContainsAny(ref, "\r\n") {
		return errors.New("browser element ref is invalid")
	}
	return nil
}

func validateBrowserKey(value string) error {
	if strings.TrimSpace(value) == "" || len(value) > maxBrowserKeyBytes || strings.ContainsAny(value, "\r\n") {
		return errors.New("browser key is invalid")
	}
	blocked := map[string]struct{}{"Control+Shift+I": {}, "Meta+Alt+I": {}, "Control+L": {}, "Meta+L": {}, "Control+C": {}, "Meta+C": {}, "Control+V": {}, "Meta+V": {}}
	if _, exists := blocked[value]; exists {
		return errors.New("browser key is not permitted")
	}
	return nil
}

func validBrowserOpaqueID(value string) bool {
	if len(value) < 3 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-') {
			return false
		}
	}
	return true
}

func boundedBrowserApprovalValue(value string) string {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " ")
	if len(value) > 160 {
		return value[:160] + "…"
	}
	return value
}
