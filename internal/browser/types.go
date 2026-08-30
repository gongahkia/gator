// Package browser owns Gator's local, developer-controlled browser sessions.
//
// It deliberately does not expose a general remote-control service. A session
// is a private local capability that may be granted to a bounded native run.
package browser

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	ModeManaged  Mode = "managed"
	ModeAttached Mode = "attached"

	StateRunning SessionState = "running"
	StateStopped SessionState = "stopped"
)

// Mode describes how a locally controlled session acquired its Chromium
// connection. Managed sessions own a fresh temporary profile. Attached
// sessions connect to a developer-started local Chromium CDP endpoint.
type Mode string

// SessionState is intentionally small: session data is useful only while its
// local controller is alive, and a stopped session cannot be resumed.
type SessionState string

// Session is display-safe persistent session metadata. It never includes a
// CDP URL, browser token, cookie, storage value, or artifact contents.
type Session struct {
	ID             string       `json:"id"`
	Mode           Mode         `json:"mode"`
	State          SessionState `json:"state"`
	Headed         bool         `json:"headed"`
	VisualCapture  bool         `json:"visual_capture"`
	CreatedAt      time.Time    `json:"created_at"`
	StoppedAt      *time.Time   `json:"stopped_at,omitempty"`
	SelectedTabs   []Tab        `json:"selected_tabs,omitempty"`
	Origins        []Origin     `json:"origins,omitempty"`
	AllowedUploads []Upload     `json:"allowed_uploads,omitempty"`
}

// Tab is deliberately limited to developer-visible metadata. The controller
// keeps the browser's opaque page identity private; tools receive only an
// opaque ID for a tab that the developer selected.
type Tab struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
	URL   string `json:"url"`
}

// Origin is a canonical scheme, hostname, and port that a developer has
// explicitly permitted for one local session.
type Origin struct {
	URL string `json:"url"`
}

// Upload identifies one developer-selected regular file. The real path is
// private controller state; models can only refer to the opaque ID and the
// non-sensitive display name.
type Upload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// StartOptions sets the only user-configurable managed-session behavior.
type StartOptions struct {
	Headed        bool
	VisualCapture bool
}

// AttachOptions configures an attached session. CDPEndpoint is held in
// controller-private memory and is intentionally absent from Session.
type AttachOptions struct {
	CDPEndpoint   string
	VisualCapture bool
}

// Artifact describes a private browser capture stored below Gator state. It
// is metadata-safe to show in the CLI and TUI but never contains raw contents.
type Artifact struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Bytes     int64     `json:"bytes"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	ErrNotFound       = errors.New("browser session was not found")
	ErrStopped        = errors.New("browser session is stopped")
	ErrNoSelectedTabs = errors.New("browser session has no developer-selected tabs")
)

func validateSessionID(value string) error {
	if len(value) < 8 || len(value) > 80 {
		return errors.New("browser session id must contain 8-80 characters")
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-') {
			return errors.New("browser session id contains unsupported characters")
		}
	}
	return nil
}

func validateTab(tab Tab) error {
	if strings.TrimSpace(tab.ID) == "" {
		return errors.New("browser tab id is required")
	}
	if len(tab.ID) > 128 || strings.ContainsAny(tab.ID, "\r\n") {
		return errors.New("browser tab id is invalid")
	}
	if len(tab.Title) > 4096 || strings.ContainsAny(tab.Title, "\r\n") {
		return errors.New("browser tab title is invalid")
	}
	if strings.TrimSpace(tab.URL) == "" || len(tab.URL) > 8192 || strings.ContainsAny(tab.URL, "\r\n") {
		return errors.New("browser tab URL is invalid")
	}
	return nil
}

func validateUpload(upload Upload) error {
	if strings.TrimSpace(upload.ID) == "" || len(upload.ID) > 128 || strings.ContainsAny(upload.ID, "\r\n") {
		return errors.New("browser upload id is invalid")
	}
	if strings.TrimSpace(upload.Name) == "" || len(upload.Name) > 512 || strings.ContainsAny(upload.Name, "\r\n") {
		return errors.New("browser upload name is invalid")
	}
	return nil
}

func sessionError(id string) error {
	if err := validateSessionID(id); err != nil {
		return fmt.Errorf("invalid browser session: %w", err)
	}
	return nil
}
