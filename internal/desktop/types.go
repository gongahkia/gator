// Package desktop owns the narrow, macOS-only local desktop capability used by
// an explicitly approved Work session. It is deliberately not a remote desktop
// service: every action occurs in the signed-in user's current local session.
package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type State string

const (
	StateRunning State = "running"
	StateStopped State = "stopped"
)

// Application identifies an app by its macOS bundle identifier, never by a
// model-selected path or executable. The display name is best-effort status
// only and is not used for authorization.
type Application struct {
	BundleID string `json:"bundle_id"`
	Name     string `json:"name,omitempty"`
}

// Session contains only display-safe local desktop capability metadata.
// RetainProviderState records the developer's explicit consent for a CUA
// provider session to retain state for its lifetime; screenshot bytes are not
// retained in this record or in Work evidence.
type Session struct {
	ID                  string        `json:"id"`
	State               State         `json:"state"`
	Apps                []Application `json:"apps"`
	RetainProviderState bool          `json:"retain_provider_state"`
	CreatedAt           time.Time     `json:"created_at"`
	StoppedAt           *time.Time    `json:"stopped_at,omitempty"`
}

type StartOptions struct {
	Apps                []Application
	RetainProviderState bool
}

// Window is a current, non-persistent accessibility-safe observation. Window
// IDs are not exposed to models; they exist only to make a window-only capture.
type Window struct {
	Application Application `json:"application"`
	Title       string      `json:"title,omitempty"`
	ID          uint32      `json:"-"`
	PID         int         `json:"-"`
	X           float64     `json:"-"`
	Y           float64     `json:"-"`
	Width       float64     `json:"-"`
	Height      float64     `json:"-"`
}

// Point is a screen coordinate in a model-visible captured window. The
// controller converts it to a macOS global coordinate only after verifying
// that the capture and foreground window still match.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Capture is deliberately ephemeral. The runner can attach its image to the
// next model turn but must never write the raw screenshot to Work evidence.
type Capture struct {
	Window Window
	PNG    []byte
}

// Runtime is a tiny platform boundary. It uses Quartz events and macOS
// accessibility state in production; tests supply a deterministic fake.
type Runtime interface {
	AccessibilityTrusted() (bool, error)
	FrontWindow(context.Context) (Window, error)
	Screenshot(context.Context, Window) ([]byte, error)
	Activate(context.Context, string) error
	Click(context.Context, float64, float64) error
	DoubleClick(context.Context, float64, float64) error
	Drag(context.Context, []Point) error
	Move(context.Context, float64, float64) error
	Scroll(context.Context, float64, float64, int, int) error
	Type(context.Context, string) error
	Press(context.Context, string) error
	FocusedFieldSensitive(context.Context, int) (bool, error)
}

var (
	ErrNotFound = errors.New("desktop session was not found")
	ErrStopped  = errors.New("desktop session is stopped")
)

func validateSessionID(value string) error {
	if len(value) < 8 || len(value) > 80 || !strings.HasPrefix(value, "desktop-") {
		return errors.New("desktop session id is invalid")
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-') {
			return errors.New("desktop session id contains unsupported characters")
		}
	}
	return nil
}

func validateApplication(app Application) error {
	value := strings.TrimSpace(app.BundleID)
	if value == "" || value != app.BundleID || len(value) > 255 || strings.ContainsAny(value, " /\\\t\r\n\x00") || !strings.Contains(value, ".") {
		return errors.New("desktop application bundle id is invalid")
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '.' || character == '-') {
			return errors.New("desktop application bundle id is invalid")
		}
	}
	if app.Name != "" && (len(app.Name) > 512 || strings.ContainsAny(app.Name, "\r\n\x00")) {
		return errors.New("desktop application name is invalid")
	}
	if forbiddenBundleID(value) {
		return fmt.Errorf("desktop application %q is never eligible for automation", value)
	}
	return nil
}

func forbiddenBundleID(value string) bool {
	switch strings.ToLower(value) {
	case "com.apple.systempreferences", "com.apple.systemsettings", "com.apple.terminal", "com.googlecode.iterm2", "com.apple.keychainaccess", "com.apple.finder":
		return true
	default:
		return false
	}
}

func applicationAllowed(apps []Application, bundleID string) bool {
	for _, app := range apps {
		if app.BundleID == bundleID {
			return true
		}
	}
	return false
}
