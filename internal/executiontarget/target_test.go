package executiontarget

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
)

func fixture(t *testing.T, backend Backend, operations ...Operation) Target {
	t.Helper()
	b, err := NewBinding(backend, "owner-instance-one", "native-start-one")
	if err != nil {
		t.Fatal(err)
	}
	return Target{Binding: b, Label: "Selected task", Capabilities: operations, Available: true}
}

func TestIdentityAndRoundTrip(t *testing.T) {
	first := fixture(t, PTY, Inspect)
	second := fixture(t, PTY, Inspect)
	if first.Binding.ID == second.Binding.ID || first.Binding.Generation == second.Binding.Generation {
		t.Fatal("identity reused")
	}
	if strings.Contains(first.Binding.ID, first.Binding.Owner) {
		t.Fatal("identity exposes backend owner")
	}
	data, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var restored Target
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatal(err)
	}
	if restored.Binding != first.Binding || !restored.Supports(Inspect) {
		t.Fatal("binding lost in roundtrip")
	}
	for _, b := range []Backend{"", "ssh", "unknown"} {
		if _, err := NewBinding(b, "owner", ""); err == nil {
			t.Fatalf("accepted backend %q", b)
		}
	}
}

func TestCapabilitiesDoNotConflateNativeControlsAndKeys(t *testing.T) {
	for _, backend := range []Backend{Work, PTY, Tmux, Ghostty} {
		for _, op := range []Operation{Inspect, Steer, Answer, Decision, Interrupt, TextInput, KeyInput, Cancel, "launch"} {
			target := fixture(t, backend, op)
			want := op == Inspect || op == Interrupt || op == Cancel || backend == Work && (op == Steer || op == Answer || op == Decision) || backend != Work && (op == TextInput || op == KeyInput)
			if (target.Validate() == nil) != want {
				t.Fatalf("%s/%s validation", backend, op)
			}
		}
	}
	target := fixture(t, PTY, Inspect)
	if target.Supports(TextInput) {
		t.Fatal("unadvertised input supported")
	}
	target.Capabilities = append(target.Capabilities, Inspect)
	if target.Validate() == nil {
		t.Fatal("duplicate capability accepted")
	}
}

func TestAvailabilityFailsClosed(t *testing.T) {
	target := fixture(t, Work, Inspect)
	target.Available = false
	if target.Validate() == nil {
		t.Fatal("missing unavailable reason")
	}
	target.UnavailableReason = "Owner disconnected"
	if err := target.Validate(); err != nil {
		t.Fatal(err)
	}
	target.Available = true
	if target.Validate() == nil {
		t.Fatal("available with unavailable reason")
	}
}

func TestTargetBoundPolicy(t *testing.T) {
	now := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	actor := Actor{ID: "device-one", Kind: Human, Origin: Remote}
	target := fixture(t, PTY, Inspect, TextInput, Interrupt, Cancel)
	grant := Grant{Actor: actor, Target: target.Binding, Operation: TextInput, ExpiresAt: now.Add(time.Minute)}
	if err := Check(now, actor, target.Binding, target, TextInput, grant, nil); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(*Actor, *Binding, *Target, *Operation, *Grant)
	}{
		{"authentication alone", func(_ *Actor, _ *Binding, _ *Target, _ *Operation, g *Grant) { *g = Grant{} }},
		{"inspect is not input", func(_ *Actor, _ *Binding, _ *Target, _ *Operation, g *Grant) { g.Operation = Inspect }},
		{"input is not interrupt", func(_ *Actor, _ *Binding, _ *Target, op *Operation, _ *Grant) { *op = Interrupt }},
		{"input is not cancel", func(_ *Actor, _ *Binding, _ *Target, op *Operation, _ *Grant) { *op = Cancel }},
		{"different device", func(a *Actor, _ *Binding, _ *Target, _ *Operation, _ *Grant) { a.ID = "device-two" }},
		{"model separate", func(a *Actor, _ *Binding, _ *Target, _ *Operation, _ *Grant) { a.Kind = Model }},
		{"remote is not local", func(a *Actor, _ *Binding, _ *Target, _ *Operation, _ *Grant) { a.Origin = Local }},
		{"expired", func(_ *Actor, _ *Binding, _ *Target, _ *Operation, g *Grant) { g.ExpiresAt = now }},
		{"no expiry", func(_ *Actor, _ *Binding, _ *Target, _ *Operation, g *Grant) { g.ExpiresAt = time.Time{} }},
		{"owner replaced", func(_ *Actor, _ *Binding, c *Target, _ *Operation, _ *Grant) { c.Binding.Owner = "owner-two" }},
		{"generation replaced", func(_ *Actor, _ *Binding, c *Target, _ *Operation, _ *Grant) {
			c.Binding.Generation = strings.Repeat("b", 32)
		}},
		{"incarnation replaced", func(_ *Actor, _ *Binding, c *Target, _ *Operation, _ *Grant) {
			c.Binding.Incarnation = "native-start-two"
		}},
		{"backend replaced", func(_ *Actor, _ *Binding, c *Target, _ *Operation, _ *Grant) { c.Binding.Backend = Tmux }},
		{"grant wrong target", func(_ *Actor, _ *Binding, _ *Target, _ *Operation, g *Grant) {
			g.Target.ID = "target-" + strings.Repeat("a", 32)
		}},
		{"stale request", func(_ *Actor, b *Binding, _ *Target, _ *Operation, _ *Grant) { b.Generation = strings.Repeat("a", 32) }},
		{"unavailable", func(_ *Actor, _ *Binding, c *Target, _ *Operation, _ *Grant) {
			c.Available = false
			c.UnavailableReason = "Disconnected"
		}},
		{"capability withdrawn", func(_ *Actor, _ *Binding, c *Target, _ *Operation, _ *Grant) { c.Capabilities = []Operation{Inspect} }},
		{"unknown operation", func(_ *Actor, _ *Binding, _ *Target, op *Operation, g *Grant) { *op = "launch"; g.Operation = *op }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b, c, op, g := actor, target.Binding, target, TextInput, grant
			tc.change(&a, &b, &c, &op, &g)
			if err := Check(now, a, b, c, op, g, nil); err == nil {
				t.Fatal("operation authorized")
			}
		})
	}
	// A human grant never authorizes the model; a separately issued model grant can.
	model := Actor{ID: "model-run-one", Kind: Model, Origin: Local}
	grant.Actor = model
	if err := Check(now, model, target.Binding, target, TextInput, grant, nil); err != nil {
		t.Fatal(err)
	}
}

func TestExternalEnrollmentIsSeparateFromGrant(t *testing.T) {
	now := time.Now()
	actor := Actor{ID: "device", Kind: Human, Origin: Remote}
	local := Actor{ID: "owner", Kind: Human, Origin: Local}
	for _, backend := range []Backend{Tmux, Ghostty} {
		target := fixture(t, backend, Inspect)
		grant := Grant{Actor: actor, Target: target.Binding, Operation: Inspect, ExpiresAt: now.Add(time.Minute)}
		if Check(now, actor, target.Binding, target, Inspect, grant, nil) == nil {
			t.Fatal("unenrolled external inspect authorized")
		}
		enrollment := Enrollment{Target: target.Binding, EnrolledBy: local, EnrolledAt: now}
		if err := Check(now, actor, target.Binding, target, Inspect, grant, &enrollment); err != nil {
			t.Fatal(err)
		}
		if Check(now, actor, target.Binding, target, Inspect, Grant{}, &enrollment) == nil {
			t.Fatal("enrollment became grant")
		}
		for _, change := range []func(*Enrollment){
			func(e *Enrollment) { e.EnrolledBy = actor },
			func(e *Enrollment) { e.EnrolledBy.Kind = Model },
			func(e *Enrollment) { e.Target.Generation = strings.Repeat("c", 32) },
			func(e *Enrollment) { e.EnrolledAt = now.Add(time.Second) },
			func(e *Enrollment) { e.EnrolledAt = time.Time{} },
		} {
			e := enrollment
			change(&e)
			if Check(now, actor, target.Binding, target, Inspect, grant, &e) == nil {
				t.Fatal("invalid enrollment accepted")
			}
		}
	}
}

func TestRecheckAfterReview(t *testing.T) {
	now := time.Now()
	actor := Actor{ID: "device", Kind: Human, Origin: Remote}
	target := fixture(t, Work, Cancel)
	grant := Grant{Actor: actor, Target: target.Binding, Operation: Cancel, ExpiresAt: now.Add(time.Second)}
	if err := Check(now, actor, target.Binding, target, Cancel, grant, nil); err != nil {
		t.Fatal(err)
	}
	if Check(now.Add(time.Second), actor, target.Binding, target, Cancel, grant, nil) == nil {
		t.Fatal("cached authorization outlived grant")
	}
}

func TestDeliveryUsesExistingActionSemantics(t *testing.T) {
	for _, tc := range []struct {
		status action.Status
		want   Delivery
	}{
		{action.Pending, NotDispatched}, {action.Denied, NotDispatched},
		{action.Executed, DispatchAcknowledged}, {action.Unknown, OutcomeUnknown},
		{action.Failed, OutcomeUnknown}, {"", OutcomeUnknown}, {"future", OutcomeUnknown},
	} {
		if got := DeliveryFromAction(tc.status); got != tc.want {
			t.Fatalf("%s: %s, want %s", tc.status, got, tc.want)
		}
	}
}
