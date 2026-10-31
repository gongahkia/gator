package actionlog

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/executiontarget"
)

func controlFixture(t *testing.T) ControlReceipt {
	t.Helper()
	b, err := executiontarget.NewBinding(executiontarget.Work, "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	return ControlReceipt{ID: strings.Repeat("a", 32), RunID: "run", Target: b, Actor: executiontarget.Actor{ID: "inspection-" + strings.Repeat("b", 64), Kind: executiontarget.Human, Origin: executiontarget.Remote}, Operation: executiontarget.Cancel, Digest: strings.Repeat("c", 64), CreatedAt: time.Now().UTC()}
}
func TestSemanticAdmissionConcurrentDuplicatesConflictAndUncertainty(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := controlFixture(t)
	var admissions atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			prior, first, err := store.ClaimControl(r)
			if err != nil {
				t.Error(err)
				return
			}
			if first {
				admissions.Add(1)
			}
			if prior.Delivery != executiontarget.OutcomeUnknown {
				t.Error("claim invented outcome")
			}
		}()
	}
	wg.Wait()
	if admissions.Load() != 1 {
		t.Fatal("duplicate dispatch permission")
	}
	changed := r
	changed.Digest = strings.Repeat("d", 64)
	if _, _, err := store.ClaimControl(changed); err == nil {
		t.Fatal("conflicting ID accepted")
	}
	prior, err := store.ReadControl(r.RunID, r.Target, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	settled, err := store.FinishControl(prior, executiontarget.DispatchAcknowledged, "Cancellation requested, not stopped.")
	if err != nil {
		t.Fatal(err)
	}
	duplicate, first, err := store.ClaimControl(r)
	if err != nil || first || duplicate != settled {
		t.Fatal("settled receipt changed or redispatched", err)
	}
}
func TestSemanticReceiptProcessDeathNeverReclaims(t *testing.T) {
	if os.Getenv("GATOR_SEMANTIC_CRASH") == "1" {
		store, err := Open(os.Getenv("GATOR_SEMANTIC_STATE"))
		if err != nil {
			os.Exit(2)
		}
		r := controlFixture(t)
		r.Target.ID = "target-" + strings.Repeat("a", 32)
		r.Target.Generation = strings.Repeat("a", 32)
		if _, first, err := store.ClaimControl(r); err != nil || !first {
			os.Exit(3)
		}
		os.Exit(17)
	}
	state := t.TempDir()
	child := exec.Command(os.Args[0], "-test.run=^TestSemanticReceiptProcessDeathNeverReclaims$")
	child.Env = append(os.Environ(), "GATOR_SEMANTIC_CRASH=1", "GATOR_SEMANTIC_STATE="+state)
	if err := child.Run(); err == nil {
		t.Fatal("crash fixture did not exit")
	}
	store, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	r, err := store.LookupControl("run", "target-"+strings.Repeat("a", 32), strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	if r.Phase != "claimed" || r.Delivery != executiontarget.OutcomeUnknown {
		t.Fatal("death erased uncertainty")
	}
	_, first, err := store.ClaimControl(r)
	if err != nil || first {
		t.Fatal("restart replayed admitted action", err)
	}
}
