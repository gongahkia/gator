package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
	gatorbrowser "github.com/gongahkia/gator/internal/browser"
)

func TestBrowserSessionToolsExposeOnlyExplicitSession(t *testing.T) {
	if surface := BrowserSessionTools(BrowserSessionOptions{}); len(surface) != 0 {
		t.Fatalf("zero browser session surface = %#v", surface)
	}
	controller := &fakeBrowserController{session: selectedBrowserSession()}
	surface := BrowserSessionTools(BrowserSessionOptions{SessionID: controller.session.ID, Controller: controller})
	if len(surface) != 10 || surface[0].Definition().Name != "browser_tabs" || surface[2].Definition().Name != "browser_screenshot" {
		t.Fatalf("browser session surface = %#v", surface)
	}
}

func TestBrowserSessionToolsRequireFreshApprovalForMutations(t *testing.T) {
	controller := &fakeBrowserController{session: selectedBrowserSession(), snapshot: gatorbrowser.Snapshot{TabID: "tab-1", URL: "https://example.test/start", Title: "start"}}
	var approvals [][]string
	surface := BrowserSessionTools(BrowserSessionOptions{
		SessionID:  controller.session.ID,
		Controller: controller,
		Policy: CommandPolicy{Approve: func(_ context.Context, argv []string) (CommandDecision, error) {
			approvals = append(approvals, append([]string(nil), argv...))
			return CommandAllowAlways, nil
		}},
	})
	navigate := toolNamed(t, surface, "browser_navigate")
	click := toolNamed(t, surface, "browser_click")
	if _, err := navigate.Execute(context.Background(), json.RawMessage(`{"tab_id":"tab-1","url":"https://example.test/next"}`)); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	if _, err := click.Execute(context.Background(), json.RawMessage(`{"tab_id":"tab-1","ref":"e1"}`)); err != nil {
		t.Fatalf("click: %v", err)
	}
	if controller.navigateURL != "https://example.test/next" || controller.clickRef != "e1" {
		t.Fatalf("controller calls = %#v", controller)
	}
	if len(approvals) != 2 || strings.Join(approvals[0], " ") != "browser navigate browser-1234567890 tab-1 https://example.test/next" || strings.Join(approvals[1], " ") != "browser click browser-1234567890 tab-1 e1" {
		t.Fatalf("approvals = %#v", approvals)
	}
}

func TestBrowserSessionToolsFailClosedForOriginAndVisualConsent(t *testing.T) {
	controller := &fakeBrowserController{session: selectedBrowserSession()}
	controller.session.Origins = nil
	controller.session.VisualCapture = false
	surface := BrowserSessionTools(BrowserSessionOptions{SessionID: controller.session.ID, Controller: controller, Policy: CommandPolicy{Approve: allowOnce}})
	if _, err := toolNamed(t, surface, "browser_navigate").Execute(context.Background(), json.RawMessage(`{"tab_id":"tab-1","url":"https://example.test/"}`)); err == nil || !strings.Contains(err.Error(), "not approved") {
		t.Fatalf("unapproved navigation error = %v", err)
	}
	if _, err := toolNamed(t, surface, "browser_screenshot").Execute(context.Background(), json.RawMessage(`{"tab_id":"tab-1"}`)); err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("screenshot without visual consent error = %v", err)
	}
}

func TestBrowserScreenshotCreatesBoundedUntrustedObservation(t *testing.T) {
	controller := &fakeBrowserController{session: selectedBrowserSession(), capture: gatorbrowser.Capture{Artifact: gatorbrowser.Artifact{ID: "artifact-1", SessionID: "browser-1234567890", Kind: "screenshot", Name: "screen.png", Bytes: 3}, PNG: []byte("png")}}
	tool := toolNamed(t, BrowserSessionTools(BrowserSessionOptions{SessionID: controller.session.ID, Controller: controller}), "browser_screenshot")
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"tab_id":"tab-1"}`))
	if err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	if !strings.Contains(result.Content, `"kind":"screenshot"`) || len(result.Observations) != 1 || len(result.Observations[0].Images) != 1 || string(result.Observations[0].Images[0].Data) != "png" {
		t.Fatalf("screenshot result = %#v", result)
	}
}

func TestBrowserUploadAndSensitiveFillRemainControllerBounded(t *testing.T) {
	controller := &fakeBrowserController{session: selectedBrowserSession(), fillErr: errors.New("browser refuses password and autocomplete-sensitive fields")}
	surface := BrowserSessionTools(BrowserSessionOptions{SessionID: controller.session.ID, Controller: controller, Policy: CommandPolicy{Approve: allowOnce}})
	if _, err := toolNamed(t, surface, "browser_fill").Execute(context.Background(), json.RawMessage(`{"tab_id":"tab-1","ref":"password","value":"secret"}`)); err == nil || !strings.Contains(err.Error(), "refuses password") {
		t.Fatalf("sensitive fill error = %v", err)
	}
	if _, err := toolNamed(t, surface, "browser_upload").Execute(context.Background(), json.RawMessage(`{"tab_id":"tab-1","ref":"file","upload_id":"../../secret"}`)); err == nil || !strings.Contains(err.Error(), "upload id") {
		t.Fatalf("invalid upload id error = %v", err)
	}
}

func selectedBrowserSession() gatorbrowser.Session {
	return gatorbrowser.Session{ID: "browser-1234567890", State: gatorbrowser.StateRunning, Mode: gatorbrowser.ModeManaged, VisualCapture: true, SelectedTabs: []gatorbrowser.Tab{{ID: "tab-1", URL: "https://example.test/start"}}, Origins: []gatorbrowser.Origin{{URL: "https://example.test:443"}}}
}

func toolNamed(t *testing.T, surface []agent.Tool, name string) agent.Tool {
	t.Helper()
	for _, tool := range surface {
		if tool.Definition().Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}

type fakeBrowserController struct {
	session     gatorbrowser.Session
	snapshot    gatorbrowser.Snapshot
	capture     gatorbrowser.Capture
	navigateURL string
	clickRef    string
	fillErr     error
}

func (f *fakeBrowserController) Session(context.Context, string) (gatorbrowser.Session, error) {
	return f.session, nil
}
func (f *fakeBrowserController) Tabs(context.Context, string) ([]gatorbrowser.Tab, error) {
	return append([]gatorbrowser.Tab(nil), f.session.SelectedTabs...), nil
}
func (f *fakeBrowserController) Snapshot(context.Context, string, string) (gatorbrowser.Snapshot, error) {
	return f.snapshot, nil
}
func (f *fakeBrowserController) Screenshot(context.Context, string, string) (gatorbrowser.Capture, error) {
	return f.capture, nil
}
func (f *fakeBrowserController) Navigate(_ context.Context, _ string, _ string, value string) (gatorbrowser.Snapshot, error) {
	f.navigateURL = value
	return f.snapshot, nil
}
func (f *fakeBrowserController) Click(_ context.Context, _ string, _ string, ref string) (gatorbrowser.Snapshot, error) {
	f.clickRef = ref
	return f.snapshot, nil
}
func (f *fakeBrowserController) Fill(context.Context, string, string, string, string) (gatorbrowser.Snapshot, error) {
	return f.snapshot, f.fillErr
}
func (f *fakeBrowserController) Select(context.Context, string, string, string, string) (gatorbrowser.Snapshot, error) {
	return f.snapshot, nil
}
func (f *fakeBrowserController) Press(context.Context, string, string, string) (gatorbrowser.Snapshot, error) {
	return f.snapshot, nil
}
func (f *fakeBrowserController) Download(context.Context, string, string, string) (gatorbrowser.Artifact, error) {
	return gatorbrowser.Artifact{}, nil
}
func (f *fakeBrowserController) Upload(context.Context, string, string, string, string) (gatorbrowser.Snapshot, error) {
	return f.snapshot, nil
}
