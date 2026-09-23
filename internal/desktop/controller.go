package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Controller serializes a local desktop session. It checks the current
// foreground application's bundle identifier before every read or action, so
// an allowed action cannot drift into a different app after a user takeover.
type Controller struct {
	store   *Store
	runtime Runtime
	mu      sync.Mutex
}

func NewController(store *Store, runtime Runtime) (*Controller, error) {
	if store == nil || runtime == nil {
		return nil, errors.New("desktop controller requires a store and runtime")
	}
	return &Controller{store: store, runtime: runtime}, nil
}

func (c *Controller) Session(_ context.Context, id string) (Session, error) { return c.running(id) }

func (c *Controller) Window(ctx context.Context, id string) (Window, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	session, err := c.running(id)
	if err != nil {
		return Window{}, err
	}
	return c.allowedFront(ctx, session)
}

func (c *Controller) Screenshot(ctx context.Context, id string) (Capture, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	session, err := c.running(id)
	if err != nil {
		return Capture{}, err
	}
	window, err := c.allowedFront(ctx, session)
	if err != nil {
		return Capture{}, err
	}
	png, err := c.runtime.Screenshot(ctx, window)
	if err != nil {
		return Capture{}, err
	}
	if len(png) == 0 || len(png) > 8*1024*1024 {
		return Capture{}, errors.New("desktop screenshot is empty or exceeds 8 MiB")
	}
	return Capture{Window: window, PNG: png}, nil
}

func (c *Controller) Activate(ctx context.Context, id, bundleID string) (Window, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	session, err := c.running(id)
	if err != nil {
		return Window{}, err
	}
	if !applicationAllowed(session.Apps, bundleID) {
		return Window{}, errors.New("application is not approved for this desktop session")
	}
	if err := c.runtime.Activate(ctx, bundleID); err != nil {
		return Window{}, err
	}
	return c.allowedFront(ctx, session)
}

func (c *Controller) Click(ctx context.Context, id string, x, y float64) (Window, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	session, err := c.running(id)
	if err != nil {
		return Window{}, err
	}
	window, err := c.allowedFront(ctx, session)
	if err != nil {
		return Window{}, err
	}
	if x < 0 || y < 0 || x > 32768 || y > 32768 {
		return Window{}, errors.New("desktop coordinates are invalid")
	}
	if err := c.runtime.Click(ctx, x, y); err != nil {
		return Window{}, err
	}
	return window, nil
}

func (c *Controller) Type(ctx context.Context, id, value string) (Window, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	session, err := c.running(id)
	if err != nil {
		return Window{}, err
	}
	window, err := c.allowedFront(ctx, session)
	if err != nil {
		return Window{}, err
	}
	if strings.TrimSpace(value) == "" || len(value) > 8*1024 || strings.ContainsRune(value, 0) {
		return Window{}, errors.New("desktop text is invalid or exceeds 8 KiB")
	}
	sensitive, err := c.runtime.FocusedFieldSensitive(ctx, window.PID)
	if err != nil {
		return Window{}, err
	}
	if sensitive {
		return Window{}, errors.New("desktop typing into a sensitive field is blocked; complete authentication and secrets yourself")
	}
	if err := c.runtime.Type(ctx, value); err != nil {
		return Window{}, err
	}
	return window, nil
}

func (c *Controller) Press(ctx context.Context, id, key string) (Window, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	session, err := c.running(id)
	if err != nil {
		return Window{}, err
	}
	window, err := c.allowedFront(ctx, session)
	if err != nil {
		return Window{}, err
	}
	if !safeKey(key) {
		return Window{}, fmt.Errorf("desktop key %q is unavailable", key)
	}
	if err := c.runtime.Press(ctx, key); err != nil {
		return Window{}, err
	}
	return window, nil
}

func (c *Controller) running(id string) (Session, error) {
	session, err := c.store.Get(id)
	if err != nil {
		return Session{}, err
	}
	if session.State != StateRunning {
		return Session{}, ErrStopped
	}
	return session, nil
}

func (c *Controller) allowedFront(ctx context.Context, session Session) (Window, error) {
	window, err := c.runtime.FrontWindow(ctx)
	if err != nil {
		return Window{}, err
	}
	if !applicationAllowed(session.Apps, window.Application.BundleID) || forbiddenBundleID(window.Application.BundleID) {
		return Window{}, errors.New("foreground application is not approved for this desktop session")
	}
	return window, nil
}

func safeKey(value string) bool {
	switch value {
	case "Enter", "Tab", "Escape", "Backspace", "Delete", "Up", "Down", "Left", "Right", "Space", "Home", "End", "PageUp", "PageDown":
		return true
	default:
		return false
	}
}
