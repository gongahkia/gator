package desktop

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"strings"
	"sync"
)

// Controller serializes a local desktop session. It checks the current
// foreground application's bundle identifier before every read or action, so
// an allowed action cannot drift into a different app after a user takeover.
type Controller struct {
	store    *Store
	runtime  Runtime
	mu       sync.Mutex
	captures map[string]captureGeometry
}

type captureGeometry struct {
	WindowID         uint32
	WindowX, WindowY float64
	WindowWidth      float64
	WindowHeight     float64
	ScreenshotWidth  int
	ScreenshotHeight int
}

func NewController(store *Store, runtime Runtime) (*Controller, error) {
	if store == nil || runtime == nil {
		return nil, errors.New("desktop controller requires a store and runtime")
	}
	return &Controller{store: store, runtime: runtime, captures: map[string]captureGeometry{}}, nil
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
	configuration, _, err := image.DecodeConfig(bytes.NewReader(png))
	if err != nil || configuration.Width < 1 || configuration.Height < 1 {
		return Capture{}, errors.New("desktop screenshot is not a readable PNG")
	}
	if window.ID == 0 || window.Width <= 0 || window.Height <= 0 {
		return Capture{}, errors.New("desktop window has no usable capture bounds")
	}
	c.captures[id] = captureGeometry{WindowID: window.ID, WindowX: window.X, WindowY: window.Y, WindowWidth: window.Width, WindowHeight: window.Height, ScreenshotWidth: configuration.Width, ScreenshotHeight: configuration.Height}
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
	x, y, err = c.capturedPoint(id, window, x, y)
	if err != nil {
		return Window{}, err
	}
	if err := c.runtime.Click(ctx, x, y); err != nil {
		return Window{}, err
	}
	return window, nil
}

func (c *Controller) DoubleClick(ctx context.Context, id string, x, y float64) (Window, error) {
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
	x, y, err = c.capturedPoint(id, window, x, y)
	if err != nil {
		return Window{}, err
	}
	if err := c.runtime.DoubleClick(ctx, x, y); err != nil {
		return Window{}, err
	}
	return window, nil
}

func (c *Controller) Drag(ctx context.Context, id string, points []Point) (Window, error) {
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
	if len(points) < 2 || len(points) > 128 {
		return Window{}, errors.New("desktop drag requires between two and 128 points")
	}
	translated := make([]Point, 0, len(points))
	for _, point := range points {
		x, y, err := c.capturedPoint(id, window, point.X, point.Y)
		if err != nil {
			return Window{}, err
		}
		translated = append(translated, Point{X: x, Y: y})
	}
	if err := c.runtime.Drag(ctx, translated); err != nil {
		return Window{}, err
	}
	return window, nil
}

func (c *Controller) Move(ctx context.Context, id string, x, y float64) (Window, error) {
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
	x, y, err = c.capturedPoint(id, window, x, y)
	if err != nil {
		return Window{}, err
	}
	if err := c.runtime.Move(ctx, x, y); err != nil {
		return Window{}, err
	}
	return window, nil
}

func (c *Controller) Scroll(ctx context.Context, id string, x, y float64, deltaX, deltaY int) (Window, error) {
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
	if deltaX < -10000 || deltaX > 10000 || deltaY < -10000 || deltaY > 10000 || (deltaX == 0 && deltaY == 0) {
		return Window{}, errors.New("desktop scroll delta is invalid")
	}
	x, y, err = c.capturedPoint(id, window, x, y)
	if err != nil {
		return Window{}, err
	}
	if err := c.runtime.Scroll(ctx, x, y, deltaX, deltaY); err != nil {
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

func (c *Controller) capturedPoint(id string, window Window, x, y float64) (float64, float64, error) {
	geometry, ok := c.captures[id]
	if !ok {
		return 0, 0, errors.New("take a fresh desktop screenshot before requesting pointer input")
	}
	if geometry.WindowID != window.ID || geometry.WindowX != window.X || geometry.WindowY != window.Y || geometry.WindowWidth != window.Width || geometry.WindowHeight != window.Height {
		return 0, 0, errors.New("desktop window changed since the last screenshot; take a fresh screenshot")
	}
	if x < 0 || y < 0 || x >= float64(geometry.ScreenshotWidth) || y >= float64(geometry.ScreenshotHeight) {
		return 0, 0, errors.New("desktop coordinates fall outside the latest screenshot")
	}
	return geometry.WindowX + x*geometry.WindowWidth/float64(geometry.ScreenshotWidth), geometry.WindowY + y*geometry.WindowHeight/float64(geometry.ScreenshotHeight), nil
}

func safeKey(value string) bool {
	switch value {
	case "Enter", "Tab", "Escape", "Backspace", "Delete", "Up", "Down", "Left", "Right", "Space", "Home", "End", "PageUp", "PageDown":
		return true
	default:
		return false
	}
}
