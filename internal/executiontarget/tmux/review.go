package tmux

import (
	"time"

	"github.com/gongahkia/gator/internal/executiontarget"
)

const ReadTrustScope = "trusted_server_read_command_behavior"

// Review is a local owner's acceptance of backend command behavior, not proof
// about aliases. Socket evidence is obtained passively BEFORE any tmux command.
// Config supplies the executable/socket paths; the digest pins the executable.
type Review struct {
	ServerPID        int                   `json:"server_pid,omitempty"`
	ServerStart      string                `json:"server_start,omitempty"`
	SocketEvidence   string                `json:"socket_evidence"`
	ExecutableDigest string                `json:"executable_digest"`
	Reviewer         executiontarget.Actor `json:"reviewer"`
	ReviewedAt       time.Time             `json:"reviewed_at"`
	Scope            string                `json:"scope"`
}

func (r *Review) Valid() bool {
	return r != nil && r.SocketEvidence != "" && r.ExecutableDigest != "" && r.Reviewer.ID != "" && r.Reviewer.Kind == executiontarget.Human && r.Reviewer.Origin == executiontarget.Local && !r.ReviewedAt.IsZero() && !r.ReviewedAt.After(time.Now()) && r.Scope == ReadTrustScope
}

// AcceptReadConfig is reachable only from trusted local administration. It does
// not connect to tmux, enumerate panes or imply device Inspect authorization.
func AcceptReadConfig(c Config, actor executiontarget.Actor) (Config, error) {
	if actor.ID == "" || actor.Kind != executiontarget.Human || actor.Origin != executiontarget.Local {
		return Config{}, ErrUnavailable
	}
	r := &Review{Reviewer: actor, ReviewedAt: time.Now().UTC(), Scope: ReadTrustScope}
	var err error
	r.SocketEvidence, err = (OSProbe{}).Socket(c.Socket)
	if err != nil {
		return Config{}, ErrUnavailable
	}
	r.ExecutableDigest, err = ExecutableFingerprint(c.Executable)
	if err != nil {
		return Config{}, ErrUnavailable
	}
	if !r.Valid() {
		return Config{}, ErrUnavailable
	}
	c.ReadConfigReviewed = true
	c.Review = r
	return c, nil
}
func (c *Controller) reviewed() bool {
	if c.ReviewCurrent != nil && !c.ReviewCurrent() {
		return false
	}
	if !c.Config.ReadConfigReviewed || !c.Config.Review.Valid() {
		return false
	}
	evidence, err := c.Probe.Socket(c.Config.Socket)
	if err != nil || evidence != c.Config.Review.SocketEvidence {
		return false
	}
	if c.Config.Review.ServerPID != 0 {
		start, err := c.Probe.Process(c.Config.Review.ServerPID)
		if err != nil || start != c.Config.Review.ServerStart {
			return false
		}
	}
	return true
}
