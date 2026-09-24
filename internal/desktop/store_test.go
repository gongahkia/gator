package desktop

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
)

func TestStoreRequiresBoundedApprovedApps(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Start(StartOptions{}); err == nil {
		t.Fatal("empty app allowlist was accepted")
	}
	if _, err := store.Start(StartOptions{Apps: []Application{{BundleID: "com.apple.Terminal"}}}); err == nil {
		t.Fatal("terminal was approved")
	}
	session, err := store.Start(StartOptions{Apps: []Application{{BundleID: "com.microsoft.Powerpoint"}}, RetainProviderState: true})
	if err != nil {
		t.Fatal(err)
	}
	if session.State != StateRunning || !session.RetainProviderState {
		t.Fatalf("session = %#v", session)
	}
	stopped, err := store.Stop(session.ID)
	if err != nil || stopped.State != StateStopped {
		t.Fatalf("stop = %#v, %v", stopped, err)
	}
}

func TestControllerChecksForegroundAppAndSensitiveField(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Start(StartOptions{Apps: []Application{{BundleID: "com.microsoft.Powerpoint"}}})
	if err != nil {
		t.Fatal(err)
	}
	runtime := fakeRuntime{window: Window{Application: Application{BundleID: "com.microsoft.Powerpoint"}, ID: 1, PID: 42}}
	controller, err := NewController(store, &runtime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Type(context.Background(), session.ID, "hello"); err != nil {
		t.Fatal(err)
	}
	if runtime.typed != "hello" {
		t.Fatalf("typed = %q", runtime.typed)
	}
	runtime.sensitive = true
	if _, err := controller.Type(context.Background(), session.ID, "secret"); err == nil {
		t.Fatal("sensitive field was typed into")
	}
	runtime.window.Application.BundleID = "com.apple.Terminal"
	if _, err := controller.Window(context.Background(), session.ID); err == nil {
		t.Fatal("unapproved foreground app was accepted")
	}
}

func TestControllerMapsScreenshotCoordinatesToCurrentWindow(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Start(StartOptions{Apps: []Application{{BundleID: "com.microsoft.Powerpoint"}}})
	if err != nil {
		t.Fatal(err)
	}
	runtime := fakeRuntime{window: Window{Application: Application{BundleID: "com.microsoft.Powerpoint"}, ID: 7, PID: 42, X: 100, Y: 200, Width: 50, Height: 25}}
	controller, err := NewController(store, &runtime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Screenshot(context.Background(), session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Click(context.Background(), session.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	if runtime.clickX != 100 || runtime.clickY != 200 {
		t.Fatalf("global click = %.1f,%.1f", runtime.clickX, runtime.clickY)
	}
}

type fakeRuntime struct {
	window    Window
	typed     string
	sensitive bool
	clickX    float64
	clickY    float64
}

func (*fakeRuntime) AccessibilityTrusted() (bool, error)           { return true, nil }
func (r *fakeRuntime) FrontWindow(context.Context) (Window, error) { return r.window, nil }
func (*fakeRuntime) Screenshot(context.Context, Window) ([]byte, error) {
	return base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLqWQAAAABJRU5ErkJggg==")
}
func (*fakeRuntime) Activate(context.Context, string) error { return nil }
func (r *fakeRuntime) Click(_ context.Context, x, y float64) error {
	r.clickX, r.clickY = x, y
	return nil
}
func (*fakeRuntime) DoubleClick(context.Context, float64, float64) error { return nil }
func (*fakeRuntime) Drag(context.Context, []Point) error                 { return nil }
func (*fakeRuntime) Move(context.Context, float64, float64) error        { return nil }
func (*fakeRuntime) Scroll(context.Context, float64, float64, int, int) error {
	return nil
}
func (r *fakeRuntime) Type(_ context.Context, value string) error { r.typed = value; return nil }
func (*fakeRuntime) Press(context.Context, string) error          { return nil }
func (r *fakeRuntime) FocusedFieldSensitive(context.Context, int) (bool, error) {
	if r == nil {
		return true, errors.New("nil")
	}
	return r.sensitive, nil
}
