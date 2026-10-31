package executiontarget

import (
	"errors"
	"time"
)

type ActorKind string
type ActorOrigin string

const (
	Human  ActorKind   = "human"
	Model  ActorKind   = "model"
	Local  ActorOrigin = "local"
	Remote ActorOrigin = "remote"
)

// Actor is resolved by the trusted host, never accepted as a client assertion.
// A remote device and a model acting for its user are distinct principals.
type Actor struct {
	ID     string      `json:"id"`
	Kind   ActorKind   `json:"kind"`
	Origin ActorOrigin `json:"origin"`
}

func (a Actor) valid() bool {
	return bounded(a.ID, 256) && (a.Kind == Human || a.Kind == Model) && (a.Origin == Local || a.Origin == Remote)
}

// Grant is an explicit authorization, not an authentication token. No wildcard
// actor, target, generation, operation, or non-expiring grant is supported.
type Grant struct {
	Actor     Actor     `json:"actor"`
	Target    Binding   `json:"target"`
	Operation Operation `json:"operation"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Enrollment is evidence supplied by a trusted local enrollment workflow.
// T0 provides no enrollment endpoint or persistence. Revoked evidence must not
// be supplied to Check. Changing the target binding invalidates old enrollment.
type Enrollment struct {
	Target     Binding   `json:"target"`
	EnrolledBy Actor     `json:"enrolled_by"`
	EnrolledAt time.Time `json:"enrolled_at"`
}

// Check is a pure, fail-closed policy check, not an authority store or executor.
// The execution owner MUST reload current target, grant and enrollment state and
// call Check immediately before dispatch, under its target serialization gate.
// Never cache a successful check across waits, reviews, queues, or mutations.
// Existing Work mode, sandbox, and exact-input approval checks still apply.
func Check(now time.Time, actor Actor, expected Binding, current Target, op Operation, grant Grant, enrollment *Enrollment) error {
	if now.IsZero() || !actor.valid() {
		return errors.New("invalid execution actor or authorization time")
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if err := current.Validate(); err != nil {
		return err
	}
	if expected != current.Binding {
		return errors.New("execution target changed")
	}
	if !current.Available {
		return errors.New("execution target unavailable")
	}
	if !current.Supports(op) {
		return errors.New("execution operation unsupported")
	}
	if grant.Actor != actor || grant.Target != expected || grant.Operation != op || !now.Before(grant.ExpiresAt) {
		return errors.New("execution operation is not authorized")
	}
	if expected.Backend.External() {
		if enrollment == nil || enrollment.Target != expected || !enrollment.EnrolledBy.valid() || enrollment.EnrolledBy.Kind != Human || enrollment.EnrolledBy.Origin != Local || enrollment.EnrolledAt.IsZero() || enrollment.EnrolledAt.After(now) {
			return errors.New("external execution target requires explicit local enrollment")
		}
	}
	return nil
}
