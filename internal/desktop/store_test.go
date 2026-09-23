package desktop

import (
	"context"
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

type fakeRuntime struct {
	window    Window
	typed     string
	sensitive bool
}

func (*fakeRuntime) AccessibilityTrusted() (bool, error)                { return true, nil }
func (r *fakeRuntime) FrontWindow(context.Context) (Window, error)      { return r.window, nil }
func (*fakeRuntime) Screenshot(context.Context, Window) ([]byte, error) { return []byte("png"), nil }
func (*fakeRuntime) Activate(context.Context, string) error             { return nil }
func (*fakeRuntime) Click(context.Context, float64, float64) error      { return nil }
func (r *fakeRuntime) Type(_ context.Context, value string) error       { r.typed = value; return nil }
func (*fakeRuntime) Press(context.Context, string) error                { return nil }
func (r *fakeRuntime) FocusedFieldSensitive(context.Context, int) (bool, error) {
	if r == nil {
		return true, errors.New("nil")
	}
	return r.sensitive, nil
}
