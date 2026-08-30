package browser

import (
	"context"
	"errors"
)

// Controller is the narrow local-controller boundary exposed to native agent
// runs. Implementations must enforce the selected-tab and origin policy before
// forwarding an operation to Chromium. No method exposes cookies, storage,
// arbitrary JavaScript, raw selectors, or browser-profile paths.
type Controller interface {
	Session(context.Context, string) (Session, error)
	Tabs(context.Context, string) ([]Tab, error)
	Snapshot(context.Context, string, string) (Snapshot, error)
	Screenshot(context.Context, string, string) (Capture, error)
	Navigate(context.Context, string, string, string) (Snapshot, error)
	Click(context.Context, string, string, string) (Snapshot, error)
	Fill(context.Context, string, string, string, string) (Snapshot, error)
	Select(context.Context, string, string, string, string) (Snapshot, error)
	Press(context.Context, string, string, string) (Snapshot, error)
	Download(context.Context, string, string, string) (Artifact, error)
	Upload(context.Context, string, string, string, string) (Snapshot, error)
}

// Snapshot is a bounded, accessibility-oriented browser view. Refs are opaque
// IDs created by the controller and expire after navigation or any operation
// that changes the page. The snapshot's text is untrusted page content.
type Snapshot struct {
	TabID     string    `json:"tab_id"`
	URL       string    `json:"url"`
	Title     string    `json:"title,omitempty"`
	Text      string    `json:"text,omitempty"`
	Elements  []Element `json:"elements,omitempty"`
	Truncated bool      `json:"truncated"`
}

// Element describes one actionable visible element. It intentionally contains
// no CSS/XPath selector, DOM HTML, field value, or hidden accessibility data.
type Element struct {
	Ref       string `json:"ref"`
	Role      string `json:"role"`
	Name      string `json:"name,omitempty"`
	Disabled  bool   `json:"disabled,omitempty"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

// Capture carries private screenshot bytes to the agent runner; it is never
// written into a session journal. Artifact metadata may be retained locally.
type Capture struct {
	Artifact Artifact
	PNG      []byte
}

var ErrUnavailable = errors.New("browser session controller is unavailable; start or attach a local session with 'gator browser'")

// UnavailableController makes absence of an explicitly started local browser
// fail closed. It is used for one-shot runs that did not configure a browser
// controller.
type UnavailableController struct{}

func (UnavailableController) Session(context.Context, string) (Session, error) {
	return Session{}, ErrUnavailable
}
func (UnavailableController) Tabs(context.Context, string) ([]Tab, error) { return nil, ErrUnavailable }
func (UnavailableController) Snapshot(context.Context, string, string) (Snapshot, error) {
	return Snapshot{}, ErrUnavailable
}
func (UnavailableController) Screenshot(context.Context, string, string) (Capture, error) {
	return Capture{}, ErrUnavailable
}
func (UnavailableController) Navigate(context.Context, string, string, string) (Snapshot, error) {
	return Snapshot{}, ErrUnavailable
}
func (UnavailableController) Click(context.Context, string, string, string) (Snapshot, error) {
	return Snapshot{}, ErrUnavailable
}
func (UnavailableController) Fill(context.Context, string, string, string, string) (Snapshot, error) {
	return Snapshot{}, ErrUnavailable
}
func (UnavailableController) Select(context.Context, string, string, string, string) (Snapshot, error) {
	return Snapshot{}, ErrUnavailable
}
func (UnavailableController) Press(context.Context, string, string, string) (Snapshot, error) {
	return Snapshot{}, ErrUnavailable
}
func (UnavailableController) Download(context.Context, string, string, string) (Artifact, error) {
	return Artifact{}, ErrUnavailable
}
func (UnavailableController) Upload(context.Context, string, string, string, string) (Snapshot, error) {
	return Snapshot{}, ErrUnavailable
}
