// Package executiontarget defines host-owned execution capability contracts.
// It does not discover targets, dispatch input, authenticate clients, or store grants.
package executiontarget

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Backend string

const (
	Work    Backend = "gator_work"
	PTY     Backend = "gator_pty"
	Tmux    Backend = "external_tmux"
	Ghostty Backend = "external_ghostty"
)

func (b Backend) External() bool { return b == Tmux || b == Ghostty }

type Operation string

const (
	Inspect   Operation = "inspect"
	Steer     Operation = "steer"
	Answer    Operation = "answer"
	Interrupt Operation = "interrupt"
	TextInput Operation = "text_input"
	KeyInput  Operation = "key_input"
	Cancel    Operation = "cancel"
	Decision  Operation = "decision"
)

// Binding is an exact authority boundary, not a display name or PID selector.
// Owner identifies the actual execution owner/backend instance. Generation is a
// Gator-issued epoch that changes on replacement, restart, or re-enrollment.
// Incarnation adds native backend evidence (e.g. application start identity),
// when available; it must never be inferred from titles or pane ordering.
type Binding struct {
	ID          string  `json:"id"`
	Backend     Backend `json:"backend"`
	Owner       string  `json:"owner"`
	Generation  string  `json:"generation"`
	Incarnation string  `json:"incarnation,omitempty"`
}

type Target struct {
	Binding           Binding     `json:"binding"`
	Label             string      `json:"label"`
	Capabilities      []Operation `json:"capabilities"`
	Available         bool        `json:"available"`
	UnavailableReason string      `json:"unavailable_reason,omitempty"`
}

// NewBinding issues opaque identities on the host. Preserve the binding while
// the same target is live. Native handles remain private to the future adapter.
func NewBinding(backend Backend, owner, incarnation string) (Binding, error) {
	id, err := token()
	if err != nil {
		return Binding{}, err
	}
	generation, err := token()
	if err != nil {
		return Binding{}, err
	}
	b := Binding{ID: "target-" + id, Backend: backend, Owner: owner, Generation: generation, Incarnation: incarnation}
	return b, b.Validate()
}

func token() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func opaque(value string) bool {
	if len(value) != 32 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func bounded(value string, max int) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= max && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func (b Binding) Validate() error {
	if !strings.HasPrefix(b.ID, "target-") || !opaque(strings.TrimPrefix(b.ID, "target-")) || !opaque(b.Generation) || !bounded(b.Owner, 256) || b.Incarnation != "" && !bounded(b.Incarnation, 512) {
		return errors.New("invalid execution target binding")
	}
	switch b.Backend {
	case Work, PTY, Tmux, Ghostty:
		return nil
	}
	return errors.New("unknown execution target backend")
}

// Structural support prevents semantic controls from becoming keystrokes.
// An adapter may advertise only a subset, based on its actual implementation.
func supports(backend Backend, op Operation) bool {
	switch op {
	case Inspect, Interrupt, Cancel:
		return true
	case Steer, Answer, Decision:
		return backend == Work
	case TextInput, KeyInput:
		return backend == PTY || backend.External()
	default:
		return false
	}
}

func (t Target) Validate() error {
	if err := t.Binding.Validate(); err != nil {
		return err
	}
	if !bounded(t.Label, 256) || t.Available && t.UnavailableReason != "" || !t.Available && !bounded(t.UnavailableReason, 512) {
		return errors.New("invalid execution target availability or label")
	}
	seen := map[Operation]bool{}
	for _, op := range t.Capabilities {
		if seen[op] || !supports(t.Binding.Backend, op) {
			return errors.New("invalid execution target capability")
		}
		seen[op] = true
	}
	return nil
}

func (t Target) Supports(op Operation) bool {
	return t.Validate() == nil && slices.Contains(t.Capabilities, op)
}
