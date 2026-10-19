package usage

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

type testModel struct {
	calls int
	fail  bool
}

func (m *testModel) Complete(context.Context, agent.TurnRequest) (agent.Turn, error) {
	m.calls++
	if m.fail {
		return agent.Turn{}, errors.New("provider unavailable")
	}
	return agent.Turn{Text: "Done", Usage: agent.Usage{InputTokens: 10, OutputTokens: 2, Reported: true}}, nil
}
func (*testModel) SupportsVisualInput() bool { return false }

type streamingTestModel struct{ testModel }

func (m *streamingTestModel) CompleteStream(ctx context.Context, request agent.TurnRequest, delta func(string)) (agent.Turn, error) {
	delta("Done")
	return m.Complete(ctx, request)
}

func TestStreamAndCancellationShareAdmission(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root)
	if err := s.SavePolicy(Policy{DailyRequests: 1, ConcurrentRequests: 1}); err != nil {
		t.Fatal(err)
	}
	backend := &streamingTestModel{}
	meter := Model(backend, root, "test/stream").(agent.StreamingModel)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := meter.CompleteStream(ctx, agent.TurnRequest{}, func(string) {}); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled request was admitted", err)
	}
	var text string
	if _, err := meter.CompleteStream(context.Background(), agent.TurnRequest{}, func(delta string) { text += delta }); err != nil {
		t.Fatal(err)
	}
	if _, err := meter.CompleteStream(context.Background(), agent.TurnRequest{}, func(string) {}); !errors.Is(err, ErrLimit) {
		t.Fatal("stream bypassed daily cap", err)
	}
	view, err := s.Snapshot(time.Now())
	if err != nil || backend.calls != 1 || text != "Done" || view.Requests != 1 || view.ActiveRequests != 0 || view.Tokens != 12 {
		t.Fatalf("stream accounting: %#v, %v", view, err)
	}
}
func TestModelMetersCategoriesStateOverridesFailuresAndRetries(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root)
	if err := s.SavePolicy(Policy{DailyRequests: 2}); err != nil {
		t.Fatal(err)
	}
	backend := &testModel{}
	meter := Model(backend, t.TempDir(), "test/model")
	ctx := WithState(WithCategory(context.Background(), "routing"), root)
	if _, err := meter.Complete(ctx, agent.TurnRequest{}); err != nil {
		t.Fatal(err)
	}
	backend.fail = true
	if _, err := meter.Complete(WithCategory(ctx, "chat"), agent.TurnRequest{}); err == nil {
		t.Fatal("provider error missing")
	}
	if _, err := meter.Complete(ctx, agent.TurnRequest{}); !errors.Is(err, ErrLimit) || !errors.Is(err, agent.ErrBudget) {
		t.Fatalf("third attempt bypassed the shared cap: %v", err)
	}
	view, _ := s.Snapshot(time.Now())
	if backend.calls != 2 || view.Requests != 2 || view.EstimatedRequests != 1 || view.Failed != 1 || view.Categories["routing"] != 1 || view.Categories["chat"] != 1 {
		t.Fatalf("incorrect call accounting: %#v", view)
	}
	if meter.(agent.VisualInputModel).SupportsVisualInput() {
		t.Fatal("meter changed model capabilities")
	}
	if _, err := meter.Complete(context.Background(), agent.TurnRequest{}); err == nil {
		t.Fatal("test backend should still fail")
	}
	view, _ = s.Snapshot(time.Now())
	if view.Requests != 2 {
		t.Fatal("another state directory consumed this profile's budget")
	}
}
func TestRequestProcessCrashRetainsChargeAndReleasesConcurrency(t *testing.T) {
	if root := os.Getenv("GATOR_USAGE_CRASH_TEST_ROOT"); root != "" {
		s, _ := Open(root)
		r, err := s.Reserve(time.Now(), "background", "test", 100)
		if err != nil {
			os.Exit(2)
		}
		_, _ = os.Stdout.WriteString("reserved\n")
		var b [1]byte
		_, _ = os.Stdin.Read(b[:])
		runtime.KeepAlive(r)
		os.Exit(0)
	}
	root := t.TempDir()
	s, _ := Open(root)
	if err := s.SavePolicy(Policy{DailyRequests: 2, ConcurrentRequests: 1}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRequestProcessCrashRetainsChargeAndReleasesConcurrency$")
	child.Env = append(os.Environ(), "GATOR_USAGE_CRASH_TEST_ROOT="+root)
	stdout, _ := child.StdoutPipe()
	stdin, _ := child.StdinPipe()
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "reserved" {
		t.Fatal("child did not reserve its request")
	}
	if _, err := s.Reserve(time.Now(), "chat", "test", 100); !errors.Is(err, ErrLimit) {
		t.Fatal("another process bypassed concurrency")
	}
	_, _ = stdin.Write([]byte("x"))
	stdin.Close()
	if err := child.Wait(); err != nil {
		t.Fatal(err)
	}
	view, err := s.Snapshot(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if view.Requests != 1 || view.EstimatedRequests != 1 || view.ActiveRequests != 0 || view.Tokens != 100 {
		t.Fatalf("crash lost or refunded a charge: %#v", view)
	}
	r, err := s.Reserve(time.Now(), "chat", "test", 100)
	if err != nil {
		t.Fatal("dead process kept the concurrency slot", err)
	}
	if err = r.Finish(20, true, false, nil); err != nil {
		t.Fatal(err)
	}
}
