package external

import (
	"context"
	"github.com/gongahkia/gator/internal/actionlog"
	"github.com/gongahkia/gator/internal/executiontarget"
	"github.com/gongahkia/gator/internal/executiontarget/tmux"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type controlledFixture struct {
	*nativeFixture
	mu       sync.Mutex
	sent     int
	delivery executiontarget.Delivery
	before   func()
}

func (f *controlledFixture) Send(_ context.Context, _ tmux.Native, _ tmux.Input, auth func() bool) executiontarget.Delivery {
	if f.before != nil {
		f.before()
	}
	if !auth() {
		return executiontarget.NotDispatched
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent++
	return f.delivery
}
func TestTerminalPermissionsDurabilityAndDuplicates(t *testing.T) {
	s, n, ref, actor, e := setup(t)
	ctx := context.Background()
	f := &controlledFixture{nativeFixture: n, delivery: executiontarget.DispatchAcknowledged}
	s.OpenReader = func(tmux.Config) (NativeReader, error) { return f, nil }
	a := Action{ID: strings.Repeat("a", 32), Target: e.Target.Binding.ID, Generation: e.Target.Binding.Generation, Input: tmux.Input{Kind: "text", Text: "private instruction"}}
	if _, err := s.Control(ctx, actor, a, true); err == nil {
		t.Fatal("read trust implied control")
	}
	if err := s.ReviewControl(ctx, a.Target, executiontarget.Actor{ID: "model", Kind: executiontarget.Model, Origin: executiontarget.Local}); err == nil {
		t.Fatal("model reviewed control")
	}
	if err := s.ReviewControl(ctx, a.Target, e.Enrollment.EnrolledBy); err != nil {
		t.Fatal(err)
	}
	s.Grant(ctx, a.Target, ref, 60)
	if _, err := s.Control(ctx, actor, a, true); err == nil {
		t.Fatal("Inspect granted input")
	}
	if err := s.GrantControl(ctx, a.Target, ref, executiontarget.TextInput, 60); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.Control(ctx, actor, a, true)
			if err != nil || r.Receipt.Delivery != executiontarget.DispatchAcknowledged {
				t.Errorf("%+v %v", r, err)
			}
		}()
	}
	wg.Wait()
	if f.sent != 1 {
		t.Fatal("duplicate dispatch", f.sent)
	}
	a.Input.Text = "different"
	if _, err := s.Control(ctx, actor, a, true); err == nil {
		t.Fatal("digest conflict")
	}
	a.ID = strings.Repeat("b", 32)
	a.Input = tmux.Input{Kind: "interrupt"}
	if _, err := s.Control(ctx, actor, a, true); err == nil {
		t.Fatal("text grant interrupted")
	}
	a.Input = tmux.Input{Kind: "text", Text: "second"}
	f.delivery = executiontarget.OutcomeUnknown
	r, err := s.Control(ctx, actor, a, true)
	if err != nil || r.Receipt.Delivery != executiontarget.OutcomeUnknown {
		t.Fatal(r, err)
	}
	if _, err := s.Receipt(actor, a.Target, a.Generation, a.ID); err != nil {
		t.Fatal(err)
	}
	count := f.sent
	for i := 0; i < 3; i++ {
		s.Control(ctx, actor, a, true)
	}
	if count != f.sent {
		t.Fatal("unknown replay")
	}
	a.ID = strings.Repeat("c", 32)
	if _, err := s.Control(ctx, actor, a, true); err == nil {
		t.Fatal("follow-up after unknown")
	}
	s.UnreviewControl(a.Target)
	if _, err := s.Control(ctx, actor, a, false); err == nil {
		t.Fatal("withdrawn trust accepted")
	}
	if _, err := s.Read(ctx, actor, a.Target, a.Generation, 100, true); err != nil {
		t.Fatal("control withdrawal damaged Inspect", err)
	}
}
func TestTerminalClaimCrashNeverReplays(t *testing.T) {
	s, _, ref, actor, e := setup(t)
	ctx := context.Background()
	s.ReviewControl(ctx, e.Target.Binding.ID, e.Enrollment.EnrolledBy)
	s.GrantControl(ctx, e.Target.Binding.ID, ref, executiontarget.TextInput, 60)
	a := Action{ID: strings.Repeat("d", 32), Target: e.Target.Binding.ID, Generation: e.Target.Binding.Generation, Input: tmux.Input{Kind: "text", Text: "secret"}}
	// A retained claimed record is the durable state after either crash window.
	r, err := s.Control(ctx, actor, a, true)
	if err != nil {
		t.Fatal(err)
	}
	log, _ := actionlog.Open(s.stateDir)
	r.Receipt.ID = strings.Repeat("e", 32)
	r.Receipt.CreatedAt = r.Receipt.UpdatedAt
	claimed, ok, err := log.ClaimTerminal(*r.Receipt)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if claimed.Delivery != executiontarget.OutcomeUnknown {
		t.Fatal("claim was not uncertain")
	}
	got, err := log.ReadTerminal(e.Target.Binding, claimed.ID)
	if err != nil || got.Delivery != executiontarget.OutcomeUnknown {
		t.Fatal(got, err)
	}
	_, dispatch, err := log.ClaimTerminal(*r.Receipt)
	if err != nil || dispatch {
		t.Fatal("claimed receipt replayed", err)
	}
}
func TestControlDenialsAndHistoryLoss(t *testing.T) {
	for _, which := range []string{"expired", "revoked", "rotated", "model", "generation", "scope", "history", "identity"} {
		t.Run(which, func(t *testing.T) {
			s, n, ref, actor, e := setup(t)
			ctx := context.Background()
			f := &controlledFixture{nativeFixture: n, delivery: executiontarget.DispatchAcknowledged}
			s.OpenReader = func(tmux.Config) (NativeReader, error) { return f, nil }
			s.ReviewControl(ctx, e.Target.Binding.ID, e.Enrollment.EnrolledBy)
			s.GrantControl(ctx, e.Target.Binding.ID, ref, executiontarget.TextInput, 60)
			a := Action{ID: strings.Repeat("7", 32), Target: e.Target.Binding.ID, Generation: e.Target.Binding.Generation, Input: tmux.Input{Kind: "text", Text: "private"}}
			switch which {
			case "expired":
				s.transaction(true, func(c *catalog) error {
					c.Entries[0].ControlGrants[0].Grant.ExpiresAt = time.Now().Add(-time.Second)
					return nil
				})
			case "revoked":
				s.GrantControl(ctx, a.Target, ref, executiontarget.TextInput, 0)
			case "rotated":
				os.WriteFile(ref.TokenFile, []byte(strings.Repeat("b", 64)+"\n"), 0600)
			case "model":
				actor.Kind = executiontarget.Model
			case "generation":
				a.Generation = strings.Repeat("f", 32)
			case "scope":
				a.Input = tmux.Input{Kind: "key", Key: "Enter"}
			case "history":
				os.RemoveAll(filepath.Join(s.stateDir, "gator", "external-actions"))
			case "identity":
				n.n.PaneStart = "replaced"
			}
			r, err := s.Control(ctx, actor, a, true)
			if err == nil && (r.Receipt == nil || r.Receipt.Delivery != executiontarget.NotDispatched) {
				t.Fatal("denial dispatched", r)
			}
			if f.sent != 0 {
				t.Fatal("sent", f.sent)
			}
			if which == "history" {
				if err := s.ReviewControl(ctx, a.Target, e.Enrollment.EnrolledBy); err == nil {
					t.Fatal("lost tombstones silently reinitialized")
				}
			}
		})
	}
}
func TestControlWithdrawalAfterAdmissionBeforeSend(t *testing.T) {
	s, n, ref, actor, e := setup(t)
	ctx := context.Background()
	f := &controlledFixture{nativeFixture: n, delivery: executiontarget.DispatchAcknowledged}
	s.OpenReader = func(tmux.Config) (NativeReader, error) { return f, nil }
	s.ReviewControl(ctx, e.Target.Binding.ID, e.Enrollment.EnrolledBy)
	s.GrantControl(ctx, e.Target.Binding.ID, ref, executiontarget.TextInput, 60)
	f.before = func() { s.UnreviewControl(e.Target.Binding.ID) }
	a := Action{ID: strings.Repeat("8", 32), Target: e.Target.Binding.ID, Generation: e.Target.Binding.Generation, Input: tmux.Input{Kind: "text", Text: "denied"}}
	r, err := s.Control(ctx, actor, a, true)
	if err != nil || r.Receipt.Delivery != executiontarget.NotDispatched || f.sent != 0 {
		t.Fatal(r, err)
	}
}
