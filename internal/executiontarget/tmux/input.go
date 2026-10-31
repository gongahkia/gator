// Copyright 2026 gongahkia. Apache-2.0; see third_party/onibi/LICENSE.
// Adapted from Onibi internal/tmux/attach.go SendText/SendKey; modified by Gator
// for exact identity, control trust, bounded finite input and delivery semantics.
package tmux

import (
	"context"
	"unicode"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/executiontarget"
)

const ControlTrustScope = "trusted_server_control_command_behavior"
const MaxInputBytes = 2048

var Keys = map[string]string{"Enter": "Enter", "Escape": "Escape", "Tab": "Tab", "Backspace": "BSpace", "Up": "Up", "Down": "Down", "Left": "Left", "Right": "Right"}

type Input struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
	Key  string `json:"key,omitempty"`
}

func (i Input) Operation() executiontarget.Operation {
	switch i.Kind {
	case "text", "text_enter":
		return executiontarget.TextInput
	case "key":
		return executiontarget.KeyInput
	case "interrupt":
		return executiontarget.Interrupt
	}
	return ""
}
func (i Input) Valid() bool {
	switch i.Kind {
	case "text", "text_enter":
		if i.Key != "" || i.Text == "" || len(i.Text) > MaxInputBytes || !utf8.ValidString(i.Text) {
			return false
		}
		for _, r := range i.Text {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
				return false
			}
		}
		return true
	case "key":
		_, ok := Keys[i.Key]
		return ok && i.Text == ""
	case "interrupt":
		return i.Text == "" && i.Key == ""
	}
	return false
}

// Send returns acknowledgement of tmux dispatch, never foreground completion.
// Once a mutation subprocess may have started, all failures are conservative
// unknown (including partial text/Enter). No internal retry is performed.
func (c *Controller) Send(ctx context.Context, expected Native, input Input, authorize func() bool) executiontarget.Delivery {
	if !input.Valid() {
		return executiontarget.NotDispatched
	}
	c.MutationCurrent = authorize
	defer func() { c.MutationCurrent = nil }()
	attempted := false
	send := func(args ...string) bool {
		if !c.Config.ControlReview.ValidFor(ControlTrustScope) || !c.reviewed() || !authorize() {
			return false
		}
		n, err := c.Resolve(ctx, expected.Pane)
		if err != nil || n.Incarnation() != expected.Incarnation() {
			return false
		}
		if !authorize() || !c.reviewed() {
			return false
		}
		// Version/identity probes are read operations; the mutation itself is argv-only.
		attempted = true
		_, err = c.run(ctx, args...)
		return err == nil
	}
	ok := false
	switch input.Kind {
	case "text", "text_enter":
		ok = send("send-keys", "-t", expected.Pane, "-l", "--", input.Text)
		if ok && input.Kind == "text_enter" {
			ok = send("send-keys", "-t", expected.Pane, "--", "Enter")
		}
	case "key":
		ok = send("send-keys", "-t", expected.Pane, "--", Keys[input.Key])
	case "interrupt":
		ok = send("send-keys", "-t", expected.Pane, "--", "C-c")
	}
	if ok {
		return executiontarget.DispatchAcknowledged
	}
	if attempted {
		return executiontarget.OutcomeUnknown
	}
	return executiontarget.NotDispatched
}
