package workrun

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/actionlog"
	"github.com/gongahkia/gator/internal/cliagent"
	"github.com/gongahkia/gator/internal/executiontarget"
	"github.com/gongahkia/gator/internal/orchestrator"
)

func TestRemoteSemanticExistingOwnerAcrossProcesses(t *testing.T) {
	for _, kind := range []string{"steer", "decision"} {
		t.Run(kind, func(t *testing.T) {
			f := remoteFixtureFor(t)
			source := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			testName, flag, run := "TestWorkSpecialistSteerHelperProcess", "GATOR_TASK_STEER_HELPER=1", "cross-process-steering"
			if kind == "decision" {
				testName = "TestWorkDecisionHelperProcess"
				flag = "GATOR_DECISION_HELPER=1"
				run = "cross-process-decision"
			}
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+testName+"$")
			child.Env = append(os.Environ(), flag, "GATOR_CONTROL_STATE="+f.state, "GATOR_CONTROL_SOURCE="+source)
			input, err := child.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			output, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			child.Stderr = os.Stderr
			if err = child.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				input.Close()
				if child.Process != nil {
					child.Process.Kill()
				}
			}()
			reader := bufio.NewReader(output)
			line, err := reader.ReadString('\n')
			if err != nil || strings.TrimSpace(line) != "READY" {
				t.Fatalf("owner not ready: %q %v", line, err)
			}
			_, owner, err := connectWorkControl(ctx, f.state, run)
			if err != nil || owner.Inspection == nil {
				t.Fatal("no actual owner", err)
			}
			op, scope := executiontarget.Steer, "subagent-001"
			if kind == "decision" {
				op = executiontarget.Decision
				scope = "1"
			}
			if err := GrantRemoteControl(ctx, f.state, run, ControlAccess{Actor: f.actor.ID, Operation: op, Scope: scope, Seconds: 60, Authority: f.authority}); err != nil {
				t.Fatal(err)
			}
			a := RemoteAction{ID: strings.Repeat("d", 32), RunID: run, Target: *owner.Inspection, Operation: op}
			if kind == "steer" {
				a.Specialist = scope
				a.Instruction = "focus on amber"
			} else {
				if err := GrantWorkInspection(ctx, f.state, run, f.actor.ID, 60); err != nil {
					t.Fatal(err)
				}
				page, err := InspectOwnedWork(ctx, f.state, f.actor.ID, a.Target.ID, a.Target.Generation)
				if err != nil || len(page.Targets) != 1 || len(page.Targets[0].Controls) != 1 {
					t.Fatal("missing exact review", err)
				}
				o := page.Targets[0].Controls[0]
				if len(o.Preview) != 1 || o.Preview[0] != "reviewed-command" {
					t.Fatal("review changed")
				}
				a.DecisionID = o.DecisionID
				a.PreviewDigest = o.PreviewDigest
				yes := true
				a.Approved = &yes
			}
			reply, err := RouteRemoteControl(ctx, f.state, f.actor.ID, "dispatch", a)
			if err != nil || reply.Receipt == nil || reply.Receipt.Delivery != executiontarget.DispatchAcknowledged {
				t.Fatal("semantic owner refused", err)
			}
			if kind == "steer" {
				fmt.Fprintln(input, "role")
				line, err = reader.ReadString('\n')
				if err != nil || strings.TrimSpace(line) != "STEERED" {
					t.Fatalf("steering not applied: %q %v", line, err)
				}
				fmt.Fprintln(input, "parent")
				line, err = reader.ReadString('\n')
				if err != nil || strings.TrimSpace(line) != "DONE" {
					t.Fatal("parent did not finish", line, err)
				}
			} else {
				line, err = reader.ReadString('\n')
				if err != nil || strings.TrimSpace(line) != "APPROVED" {
					t.Fatalf("exact approval not applied: %q %v", line, err)
				}
			}
			if err := child.Wait(); err != nil {
				t.Fatal(err)
			}
			// The original owner is gone; reading/retrying its admitted action is safe.
			repeated, err := RouteRemoteControl(ctx, f.state, f.actor.ID, "dispatch", a)
			if err != nil || repeated.Receipt.Digest != reply.Receipt.Digest {
				t.Fatal("offline duplicate changed action", err)
			}
		})
	}
}

type remoteFixture struct {
	c              *remoteControls
	authority      CompanionAuthority
	actor          executiontarget.Actor
	binding        executiontarget.Binding
	state          string
	task           *orchestrator.Task
	calls          atomic.Int32
	reviewConsumed bool
}

func remoteFixtureFor(t *testing.T) *remoteFixture {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(root, "owner.token")
	token := strings.Repeat("a", 64)
	if err := os.WriteFile(tokenPath, []byte(token+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	authority := CompanionAuthority{TokenFile: tokenPath, Device: "owner"}
	id, err := authority.actor()
	if err != nil {
		t.Fatal(err)
	}
	b, err := executiontarget.NewBinding(executiontarget.Work, "owner-instance", "")
	if err != nil {
		t.Fatal(err)
	}
	f := &remoteFixture{authority: authority, actor: executiontarget.Actor{ID: id, Kind: executiontarget.Human, Origin: executiontarget.Remote}, binding: b, state: root}
	f.task = &orchestrator.Task{ID: "subagent-001", GlobalID: "run/subagent-001", ParentRun: "run", Role: "cli_codex", Status: "running"}
	f.c = newRemoteControls(root, "run", func() { f.calls.Add(1) }, func(id, text string) error { f.calls.Add(1); return nil }, func(id, q string, answers map[string][]string) error {
		f.calls.Add(1)
		f.task.Status = "answering"
		return nil
	}, func(id int, d string, b bool) error {
		if f.reviewConsumed {
			return errors.New("consumed")
		}
		f.reviewConsumed = true
		f.calls.Add(1)
		return nil
	}, func(id string) (orchestrator.Task, error) {
		if id != f.task.ID {
			return orchestrator.Task{}, errors.New("no task")
		}
		return *f.task, nil
	}, func(id int) (Interaction, string, error) {
		if id != 1 || f.reviewConsumed {
			return Interaction{}, "", errors.New("consumed")
		}
		return Interaction{ID: 1, Kind: "command", Preview: []string{"go", "test", "./..."}}, strings.Repeat("c", 64), nil
	})
	f.c.binding = b
	return f
}
func (f *remoteFixture) action(op executiontarget.Operation) RemoteAction {
	return RemoteAction{ID: strings.Repeat("d", 32), RunID: "run", Target: f.binding, Operation: op}
}
func (f *remoteFixture) grant(t *testing.T, op executiontarget.Operation, scope string) {
	t.Helper()
	if err := f.c.grant(context.Background(), ControlAccess{Actor: f.actor.ID, Operation: op, Scope: scope, Seconds: 60, Authority: f.authority}); err != nil {
		t.Fatal(err)
	}
}
func TestRemoteSemanticAuthorizationAndDuplicateDispatch(t *testing.T) {
	f := remoteFixtureFor(t)
	a := f.action(executiontarget.Cancel)
	if _, err := f.c.dispatch(context.Background(), f.actor, a); err == nil {
		t.Fatal("inspect/no mutation grant cancelled")
	}
	f.grant(t, executiontarget.Cancel, "")
	steer := a
	steer.Operation = executiontarget.Steer
	steer.Specialist = "subagent-001"
	steer.Instruction = "review this"
	if _, err := f.c.dispatch(context.Background(), f.actor, steer); err == nil {
		t.Fatal("cancel grant steered")
	}
	model := f.actor
	model.Kind = executiontarget.Model
	if _, err := f.c.dispatch(context.Background(), model, a); err == nil {
		t.Fatal("model inherited human grant")
	}
	wrong := a
	wrong.Target.Generation = strings.Repeat("f", 32)
	if _, err := f.c.dispatch(context.Background(), f.actor, wrong); err == nil {
		t.Fatal("wrong generation accepted")
	}
	wrong = a
	wrong.RunID = "other-conversation"
	if _, err := f.c.dispatch(context.Background(), f.actor, wrong); err == nil {
		t.Fatal("cross-run mutation")
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reply, err := f.c.dispatch(context.Background(), f.actor, a)
			if err != nil || reply.Receipt == nil || reply.Receipt.Delivery != executiontarget.DispatchAcknowledged {
				t.Error("unexpected receipt", err)
			}
		}()
	}
	wg.Wait()
	if f.calls.Load() != 1 {
		t.Fatal("duplicate semantic dispatch")
	}
	changed := a
	changed.Operation = executiontarget.Steer
	changed.Specialist = "subagent-001"
	changed.Instruction = "different"
	f.grant(t, executiontarget.Steer, "subagent-001")
	if _, err := f.c.dispatch(context.Background(), f.actor, changed); err == nil {
		t.Fatal("ID reused with different payload")
	}
}
func TestRemoteSemanticRecheckRevocationAndExpiry(t *testing.T) {
	for _, kind := range []string{"expired", "owner-rotation", "after-claim-revocation", "local-grant-revocation"} {
		t.Run(kind, func(t *testing.T) {
			f := remoteFixtureFor(t)
			f.grant(t, executiontarget.Cancel, "")
			a := f.action(executiontarget.Cancel)
			switch kind {
			case "expired":
				key := controlGrantKey(f.actor.ID, a.Operation, "")
				g := f.c.grants[key]
				g.Grant.ExpiresAt = time.Now().Add(-time.Second)
				f.c.grants[key] = g
			case "owner-rotation":
				os.WriteFile(f.authority.TokenFile, []byte(strings.Repeat("b", 64)+"\n"), 0600)
			case "after-claim-revocation":
				f.c.afterClaim = func() { os.Remove(f.authority.TokenFile) }
			case "local-grant-revocation":
				f.c.afterClaim = func() {
					if err := f.c.grant(context.Background(), ControlAccess{Actor: f.actor.ID, Operation: executiontarget.Cancel, Seconds: 0, Authority: f.authority}); err != nil {
						t.Error(err)
					}
				}
			}
			reply, err := f.c.dispatch(context.Background(), f.actor, a)
			if kind == "after-claim-revocation" || kind == "local-grant-revocation" {
				if err != nil || reply.Receipt.Delivery != executiontarget.NotDispatched {
					t.Fatal("post-admission revocation not retained", err)
				}
			} else if err == nil {
				t.Fatal("revoked/expired authority accepted")
			}
			if f.calls.Load() != 0 {
				t.Fatal("revocation dispatched")
			}
		})
	}
}
func TestRemoteSemanticSteerAnswerAndExactDecision(t *testing.T) {
	t.Run("steer", func(t *testing.T) {
		f := remoteFixtureFor(t)
		f.grant(t, executiontarget.Steer, "subagent-001")
		a := f.action(executiontarget.Steer)
		a.Specialist = "subagent-001"
		a.Instruction = "sensitive instruction"
		reply, err := f.c.dispatch(context.Background(), f.actor, a)
		if err != nil || reply.Receipt.Delivery != executiontarget.DispatchAcknowledged {
			t.Fatal(err)
		}
		data, _ := json.Marshal(reply)
		if strings.Contains(string(data), a.Instruction) {
			t.Fatal("input persisted in receipt")
		}
	})
	t.Run("answer", func(t *testing.T) {
		f := remoteFixtureFor(t)
		f.task.Status = "awaiting_input"
		f.task.Native = &cliagent.NativeState{Status: "awaiting_input", QuestionID: strings.Repeat("e", 64), Questions: []cliagent.NativeQuestion{{ID: "format", Text: "Choose format"}}}
		f.grant(t, executiontarget.Answer, "subagent-001")
		a := f.action(executiontarget.Answer)
		a.Specialist = "subagent-001"
		a.Question = f.task.Native.QuestionID
		a.Answers = map[string][]string{"format": {"Markdown"}}
		reply, err := f.c.dispatch(context.Background(), f.actor, a)
		if err != nil || reply.Receipt.Delivery != executiontarget.DispatchAcknowledged {
			t.Fatal(err)
		}
		a.ID = strings.Repeat("e", 32)
		reply, err = f.c.dispatch(context.Background(), f.actor, a)
		if err != nil || reply.Receipt.Delivery != executiontarget.NotDispatched || f.calls.Load() != 1 {
			t.Fatal("consumed question redirected", err)
		}
	})
	t.Run("approval", func(t *testing.T) {
		f := remoteFixtureFor(t)
		f.grant(t, executiontarget.Decision, "1")
		a := f.action(executiontarget.Decision)
		yes := true
		a.DecisionID = 1
		a.PreviewDigest = strings.Repeat("c", 64)
		a.Approved = &yes
		wrong := a
		wrong.PreviewDigest = strings.Repeat("f", 64)
		if _, err := f.c.dispatch(context.Background(), f.actor, wrong); err == nil {
			t.Fatal("preview mismatch approved")
		}
		reply, err := f.c.dispatch(context.Background(), f.actor, a)
		if err != nil || reply.Receipt.Delivery != executiontarget.DispatchAcknowledged {
			t.Fatal(err)
		}
		a.ID = strings.Repeat("e", 32)
		reply, err = f.c.dispatch(context.Background(), f.actor, a)
		if err != nil || reply.Receipt.Delivery != executiontarget.NotDispatched || f.calls.Load() != 1 {
			t.Fatal("consumed decision approved", err)
		}
	})
}
func TestRemoteSemanticLateErrorRemainsUnknown(t *testing.T) {
	f := remoteFixtureFor(t)
	f.grant(t, executiontarget.Steer, "subagent-001")
	f.c.steer = func(string, string) error { return errors.New("native delivery may have started") }
	a := f.action(executiontarget.Steer)
	a.Specialist = "subagent-001"
	a.Instruction = "bounded"
	reply, err := f.c.dispatch(context.Background(), f.actor, a)
	if err != nil || reply.Receipt.Delivery != executiontarget.OutcomeUnknown {
		t.Fatal("late failure labeled not dispatched", err)
	}
}

func TestRemoteApprovalReviewRejectsSyntheticChangedAndUnsupportedPreviews(t *testing.T) {
	for _, argv := range [][]string{{"terminal_write", "term-001", strings.Repeat("a", 64)}, {"mcp", "server", "tool"}} {
		i := Interaction{ID: 1, Kind: "command", Preview: argv}
		d, _ := InteractionDigest(i)
		o := &Operation{pending: map[int]pendingInteraction{1: {digest: d, interaction: i}}}
		if _, _, err := o.reviewRemoteDecision(1); err == nil {
			t.Fatal("incomplete tool preview accepted")
		}
	}
	argv := []string{"git", "status"}
	i := Interaction{ID: 1, Kind: "command", Preview: argv}
	d, _ := InteractionDigest(i)
	o := &Operation{pending: map[int]pendingInteraction{1: {digest: d, interaction: i}}}
	if _, _, err := o.reviewRemoteDecision(1); err != nil {
		t.Fatal(err)
	}
	argv[1] = "push"
	if _, _, err := o.reviewRemoteDecision(1); err == nil {
		t.Fatal("changed preview accepted")
	}
	delete(o.pending, 1)
	if _, _, err := o.reviewRemoteDecision(1); err == nil {
		t.Fatal("consumed preview accepted")
	}
}
func TestRemoteSemanticCrashWindows(t *testing.T) {
	if os.Getenv("GATOR_REMOTE_CRASH") != "" {
		state := os.Getenv("GATOR_REMOTE_STATE")
		var input struct {
			Binding   executiontarget.Binding
			Authority CompanionAuthority
			Actor     executiontarget.Actor
		}
		data, _ := os.ReadFile(filepath.Join(state, "fixture.json"))
		json.Unmarshal(data, &input)
		c := newRemoteControls(state, "run", func() { os.WriteFile(filepath.Join(state, "effect"), []byte("one"), 0600) }, nil, nil, nil, nil, nil)
		c.binding = input.Binding
		if err := c.grant(context.Background(), ControlAccess{Actor: input.Actor.ID, Operation: executiontarget.Cancel, Seconds: 60, Authority: input.Authority}); err != nil {
			os.Exit(3)
		}
		if os.Getenv("GATOR_REMOTE_CRASH") == "before" {
			c.afterClaim = func() { os.Exit(19) }
		} else {
			c.afterDispatch = func() { os.Exit(19) }
		}
		c.dispatch(context.Background(), input.Actor, RemoteAction{ID: strings.Repeat("d", 32), RunID: "run", Target: input.Binding, Operation: executiontarget.Cancel})
		os.Exit(4)
	}
	for _, window := range []string{"before", "after"} {
		t.Run(window, func(t *testing.T) {
			f := remoteFixtureFor(t)
			data, _ := json.Marshal(struct {
				Binding   executiontarget.Binding
				Authority CompanionAuthority
				Actor     executiontarget.Actor
			}{f.binding, f.authority, f.actor})
			os.WriteFile(filepath.Join(f.state, "fixture.json"), data, 0600)
			child := exec.Command(os.Args[0], "-test.run=^TestRemoteSemanticCrashWindows$")
			child.Env = append(os.Environ(), "GATOR_REMOTE_CRASH="+window, "GATOR_REMOTE_STATE="+f.state)
			if err := child.Run(); err == nil {
				t.Fatal("no crash")
			}
			store, err := actionlog.Open(f.state)
			if err != nil {
				t.Fatal(err)
			}
			r, err := store.ReadControl("run", f.binding, strings.Repeat("d", 32))
			if err != nil || r.Delivery != executiontarget.OutcomeUnknown || r.Phase != "claimed" {
				t.Fatal("uncertainty not retained", err)
			}
			_, effect := os.Stat(filepath.Join(f.state, "effect"))
			if window == "before" && !errors.Is(effect, os.ErrNotExist) || window == "after" && effect != nil {
				t.Fatal("wrong crash effect")
			}
			f.grant(t, executiontarget.Cancel, "")
			reply, err := f.c.dispatch(context.Background(), f.actor, f.action(executiontarget.Cancel))
			if err != nil || reply.Receipt.Delivery != executiontarget.OutcomeUnknown || f.calls.Load() != 0 {
				t.Fatal("restart replayed uncertain mutation", err)
			}
		})
	}
}
