package external

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/gongahkia/gator/internal/actionlog"
	"github.com/gongahkia/gator/internal/companionaccess"
	"github.com/gongahkia/gator/internal/executiontarget"
	"github.com/gongahkia/gator/internal/executiontarget/tmux"
)

type Action struct {
	ID         string     `json:"id"`
	Target     string     `json:"target"`
	Generation string     `json:"generation"`
	Input      tmux.Input `json:"input"`
}
type ControlReply struct {
	Delivery   executiontarget.Delivery   `json:"delivery,omitempty"`
	Authorized bool                       `json:"authorized"`
	Receipt    *actionlog.TerminalReceipt `json:"receipt,omitempty"`
}

func (e Entry) controlTrusted() bool {
	r := e.Config.ControlReview
	return !e.Invalid && e.Config.ReadConfigReviewed && e.Config.Review.Valid() && r.ValidFor(tmux.ControlTrustScope) && r.SocketEvidence == e.Native.SocketEvidence && r.ServerPID == e.Native.ServerPID && r.ServerStart == e.Native.ServerStart && r.ExecutableDigest == e.ExecutableDigest
}
func (s Store) ReviewControl(ctx context.Context, id string, actor executiontarget.Actor) error {
	if actor.Kind != executiontarget.Human || actor.Origin != executiontarget.Local || actor.ID == "" {
		return ErrDenied
	}
	e, err := s.entry(id)
	if err != nil || e.Invalid {
		return ErrDenied
	}
	if _, err = s.verify(ctx, e); err != nil {
		return ErrDenied
	}
	log, err := actionlog.Open(s.stateDir)
	if err != nil {
		return err
	}
	if err = log.PrepareTerminal(e.Target.Binding, !e.ControlInitialized); err != nil {
		return ErrDenied
	}
	r := *e.Config.Review
	r.Reviewer = actor
	r.ReviewedAt = time.Now().UTC()
	r.Scope = tmux.ControlTrustScope
	return s.transaction(true, func(c *catalog) error {
		for i := range c.Entries {
			v := &c.Entries[i]
			if v.Target.Binding == e.Target.Binding && !v.Invalid && v.Config.Review.Valid() {
				v.Config.ControlReview = &r
				v.ControlInitialized = true
				v.ControlGrants = nil
				return nil
			}
		}
		return ErrDenied
	})
}

// Withdrawal affects every enrollment of the same backend, preserving read access.
func (s Store) UnreviewControl(id string) error {
	e, err := s.entry(id)
	if err != nil {
		return err
	}
	return s.transaction(true, func(c *catalog) error {
		for i := range c.Entries {
			v := &c.Entries[i]
			if v.Native.SocketEvidence == e.Native.SocketEvidence && v.Native.ServerPID == e.Native.ServerPID && v.Native.ServerStart == e.Native.ServerStart {
				v.Config.ControlReview = nil
				v.ControlGrants = nil
			}
		}
		return nil
	})
}
func (s Store) GrantControl(ctx context.Context, id string, ref companionaccess.Reference, op executiontarget.Operation, seconds int) error {
	if seconds < 0 || seconds > 900 || (op != executiontarget.TextInput && op != executiontarget.KeyInput && op != executiontarget.Interrupt) {
		return ErrDenied
	}
	actor, err := ref.Actor()
	if err != nil {
		return ErrDenied
	}
	e, err := s.entry(id)
	if err != nil {
		return ErrDenied
	}
	if seconds > 0 {
		if !e.controlTrusted() {
			return ErrDenied
		}
		if _, err = s.verify(ctx, e); err != nil {
			return ErrDenied
		}
	}
	return s.transaction(true, func(c *catalog) error {
		for i := range c.Entries {
			v := &c.Entries[i]
			if v.Target.Binding != e.Target.Binding {
				continue
			}
			if seconds > 0 && !v.controlTrusted() {
				return ErrDenied
			}
			next := []InspectGrant{}
			now := time.Now().UTC()
			for _, g := range v.ControlGrants {
				if !(g.Grant.Actor.ID == actor && g.Grant.Operation == op) && now.Before(g.Grant.ExpiresAt) {
					next = append(next, g)
				}
			}
			if seconds > 0 {
				if len(next) >= 24 {
					return ErrDenied
				}
				next = append(next, InspectGrant{Grant: executiontarget.Grant{Actor: executiontarget.Actor{ID: actor, Kind: executiontarget.Human, Origin: executiontarget.Remote}, Target: v.Target.Binding, Operation: op, ExpiresAt: now.Add(time.Duration(seconds) * time.Second)}, Authority: ref})
			}
			v.ControlGrants = next
			return nil
		}
		return ErrDenied
	})
}
func (s Store) controlAuthorized(e Entry, actor executiontarget.Actor, op executiontarget.Operation) bool {
	if !e.controlTrusted() || !actorPattern.MatchString(actor.ID) || actor.Kind != executiontarget.Human || actor.Origin != executiontarget.Remote {
		return false
	}
	log, err := actionlog.Open(s.stateDir)
	if err != nil || log.PrepareTerminal(e.Target.Binding, false) != nil {
		return false
	}
	t := e.Target
	t.Capabilities = []executiontarget.Operation{executiontarget.Inspect, executiontarget.TextInput, executiontarget.KeyInput, executiontarget.Interrupt}
	for _, g := range e.ControlGrants {
		if id, err := g.Authority.Actor(); err == nil && id == actor.ID && executiontarget.Check(time.Now(), actor, e.Target.Binding, t, op, g.Grant, &e.Enrollment) == nil {
			return true
		}
	}
	return false
}
func (s Store) ControlOffers(e Entry, actor executiontarget.Actor) []executiontarget.Operation {
	result := []executiontarget.Operation{}
	for _, op := range []executiontarget.Operation{executiontarget.TextInput, executiontarget.KeyInput, executiontarget.Interrupt} {
		if s.controlAuthorized(e, actor, op) {
			result = append(result, op)
		}
	}
	return result
}
func (s Store) Receipt(actor executiontarget.Actor, id, generation, action string) (ControlReply, error) {
	e, err := s.entry(id)
	if err != nil || e.Target.Binding.Generation != generation {
		return ControlReply{}, ErrDenied
	}
	log, err := actionlog.Open(s.stateDir)
	if err != nil {
		return ControlReply{}, err
	}
	r, err := log.ReadTerminal(e.Target.Binding, action)
	if err != nil || r.Actor != actor {
		return ControlReply{}, ErrDenied
	}
	return ControlReply{Receipt: &r}, nil
}
func (s Store) Control(ctx context.Context, actor executiontarget.Actor, a Action, dispatch bool) (ControlReply, error) {
	if !actionlog.ControlID.MatchString(a.ID) || !a.Input.Valid() {
		return ControlReply{}, ErrDenied
	}
	e, err := s.entry(a.Target)
	if err != nil || e.Target.Binding.Generation != a.Generation {
		return ControlReply{}, ErrDenied
	}
	log, err := actionlog.Open(s.stateDir)
	if err != nil {
		return ControlReply{}, err
	}
	// Digest is typed canonical JSON; raw payload is never stored.
	data, _ := json.Marshal(struct {
		Actor   executiontarget.Actor
		Binding executiontarget.Binding
		Action  Action
	}{actor, e.Target.Binding, a})
	sum := sha256.Sum256(data)
	want := actionlog.TerminalReceipt{ID: a.ID, Target: e.Target.Binding, Actor: actor, Operation: a.Input.Operation(), Scope: a.Input.Kind, Digest: hex.EncodeToString(sum[:]), CreatedAt: time.Now().UTC()}
	if prior, err := log.LookupTerminalID(a.ID); err == nil {
		if prior.Target != e.Target.Binding || prior.Actor != actor || prior.Digest != want.Digest {
			return ControlReply{}, ErrDenied
		}
		return ControlReply{Receipt: &prior}, nil
	}
	authorize := func() bool {
		fresh, err := s.entry(a.Target)
		return ctx.Err() == nil && err == nil && fresh.Target.Binding == e.Target.Binding && s.controlAuthorized(fresh, actor, a.Input.Operation())
	}
	if !authorize() {
		return ControlReply{}, ErrDenied
	}
	if !dispatch {
		if _, err = s.verify(ctx, e); err != nil {
			return ControlReply{}, ErrDenied
		}
		return ControlReply{Authorized: authorize()}, nil
	}
	release, err := log.LockTerminal(e.Target.Binding)
	if err != nil {
		return ControlReply{}, err
	}
	defer release()
	// Waiting for another helper conveys no authorization.
	if !authorize() {
		return ControlReply{}, ErrDenied
	}
	receipt, claimed, err := log.ClaimTerminal(want)
	if err != nil {
		// No native dispatch is possible on this branch. If a failed durable write
		// nevertheless left a matching claim, expose its retained uncertainty.
		if prior, e := log.LookupTerminalID(a.ID); e == nil && prior.Actor == actor && prior.Target == want.Target && prior.Digest == want.Digest {
			return ControlReply{Receipt: &prior}, nil
		}
		return ControlReply{Delivery: executiontarget.NotDispatched}, nil
	}
	if !claimed {
		return ControlReply{Receipt: &receipt}, nil
	}
	delivery := executiontarget.NotDispatched
	reader, err := s.verify(ctx, e)
	if err != nil {
		s.Revoke(a.Target)
	}
	if err == nil && authorize() {
		if sender, ok := reader.(interface {
			Send(context.Context, tmux.Native, tmux.Input, func() bool) executiontarget.Delivery
		}); ok {
			delivery = sender.Send(ctx, e.Native, a.Input, authorize)
		}
	}
	note := "Input not dispatched."
	if delivery == executiontarget.DispatchAcknowledged {
		note = "tmux acknowledged input dispatch; foreground outcome is not established."
	} else if delivery == executiontarget.OutcomeUnknown {
		note = "Input may be partially or fully delivered. Inspect; do not resend automatically."
	}
	settled, err := log.FinishTerminal(receipt, delivery, note)
	if err != nil {
		return ControlReply{Receipt: &receipt}, nil
	}
	return ControlReply{Receipt: &settled}, nil
}
