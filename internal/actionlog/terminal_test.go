package actionlog

import (
	"encoding/json"
	"fmt"
	"github.com/gongahkia/gator/internal/executiontarget"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestTerminalClaimHelper(t *testing.T) {
	state := os.Getenv("GATOR_T4_CLAIM_STATE")
	if state == "" {
		return
	}
	var r TerminalReceipt
	if json.Unmarshal([]byte(os.Getenv("GATOR_T4_CLAIM_RECEIPT")), &r) != nil {
		t.Fatal("fixture")
	}
	s, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	got, winner, err := s.ClaimTerminal(r)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("winner=%v delivery=%s\n", winner, got.Delivery)
	if winner && os.Getenv("GATOR_T4_CRASH") == "1" {
		os.WriteFile(state+"/possible-dispatch", []byte("sentinel"), 0600)
		os.Exit(23)
	}
}
func TestTerminalCrossProcessClaimsAndCrashRecovery(t *testing.T) {
	dir := t.TempDir()
	binding, err := executiontarget.NewBinding(executiontarget.Tmux, "trusted-instance", "native-incarnation")
	if err != nil {
		t.Fatal(err)
	}
	r := TerminalReceipt{ID: strings.Repeat("a", 32), Target: binding, Actor: executiontarget.Actor{ID: "inspection-" + strings.Repeat("b", 64), Kind: executiontarget.Human, Origin: executiontarget.Remote}, Operation: executiontarget.TextInput, Scope: "text", Digest: strings.Repeat("c", 64), CreatedAt: time.Now().UTC()}
	data, _ := json.Marshal(r)
	child := func(crash bool) *exec.Cmd {
		c := exec.Command(os.Args[0], "-test.run=^TestTerminalClaimHelper$")
		c.Env = append(os.Environ(), "GATOR_T4_CLAIM_STATE="+dir, "GATOR_T4_CLAIM_RECEIPT="+string(data))
		if crash {
			c.Env = append(c.Env, "GATOR_T4_CRASH=1")
		}
		return c
	}
	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 8)
	for i := 0; i < 8; i++ {
		go func() { out, err := child(false).CombinedOutput(); done <- result{out, err} }()
	}
	winners := 0
	for i := 0; i < 8; i++ {
		v := <-done
		if v.err != nil {
			t.Fatal(v.err, string(v.out))
		}
		if strings.Contains(string(v.out), "winner=true") {
			winners++
		}
	}
	if winners != 1 {
		t.Fatal("cross-process winners", winners)
	}
	s, _ := Open(dir)
	if listed, err := s.List(""); err != nil || len(listed) != 0 {
		t.Fatal("terminal namespace broke Work receipts", listed, err)
	}
	got, err := s.ReadTerminal(binding, r.ID)
	if err != nil || got.Delivery != executiontarget.OutcomeUnknown {
		t.Fatal(got, err)
	}
	r.ID = strings.Repeat("d", 32)
	if _, _, err := s.ClaimTerminal(r); err == nil {
		t.Fatal("unknown predecessor allowed follow-up")
	}
	// A second isolated target reproduces a process exiting after possible dispatch.
	binding2, _ := executiontarget.NewBinding(executiontarget.Tmux, "other-instance", "native2")
	r.Target = binding2
	original := r.ID
	r.ID = strings.Repeat("a", 32)
	if _, _, err := s.ClaimTerminal(r); err == nil {
		t.Fatal("action ID crossed targets")
	}
	r.ID = original
	data, _ = json.Marshal(r)
	if out, err := child(true).CombinedOutput(); err == nil {
		t.Fatal("fixture did not crash", string(out))
	}
	if _, err := os.Stat(dir + "/possible-dispatch"); err != nil {
		t.Fatal(err)
	}
	if out, err := child(false).CombinedOutput(); err != nil || strings.Contains(string(out), "winner=true") {
		t.Fatal("crashed claim replayed", err, string(out))
	}
	reopened, _ := Open(dir)
	got, err = reopened.ReadTerminal(binding2, r.ID)
	if err != nil || got.Phase != "claimed" || got.Delivery != executiontarget.OutcomeUnknown {
		t.Fatal(got, err)
	}
}
