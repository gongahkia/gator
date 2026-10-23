package workrun

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/actionlog"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/connector"
)

func TestExecutorDispatchCrashHelper(t *testing.T) {
	stateDir, endpoint := os.Getenv("GATOR_TEST_DISPATCH_STATE"), os.Getenv("GATOR_TEST_DISPATCH_ENDPOINT")
	if stateDir == "" {
		return
	}
	registry, err := connector.NewRegistry([]connector.Descriptor{{Version: connector.DescriptorVersion, ID: "hook", Name: "Hook", Kind: connector.KindHTTPWebhook, Resource: endpoint, Authentication: connector.AuthNone}})
	if err != nil {
		t.Fatal(err)
	}
	source := os.Getenv("GATOR_TEST_DISPATCH_SOURCE")
 if err:=os.MkdirAll(source,0700);err!=nil{t.Fatal(err)}
 contract := artifact.DefaultContract("receipt.md")
	contract.ExternalActions = action.Approve
	model := &scriptedModel{turns: []agent.Turn{{ToolCalls: []agent.ToolCall{{ID: "send-one", Name: "connector_hook_publish", Arguments: json.RawMessage(`{"payload":{"release":"v1"}}`)}}}}}
	_, err = (Executor{Model: model, StateDir: stateDir, Connectors: connector.Runtime{Registry: registry}}).Execute(context.Background(), Request{RunID: "crash-dispatch", SourcePath: source, Objective: "Publish reviewed update", Mode: action.Act, Contract: contract, ConnectorIDs: []string{"hook"}, MaxSteps: 2, ApproveAction: func(context.Context, action.Proposal) (action.Decision, error) { return action.Allow, nil }})
	if err != nil {
		t.Fatal(err)
	}
}
func TestExecutorCrashDuringExternalDispatchRetainsOutcomeAndNeverReplaysOnRead(t *testing.T) {
	stateDir := t.TempDir()
source := t.TempDir()
	received := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		received <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestExecutorDispatchCrashHelper$")
	child.Env = append(os.Environ(), "GATOR_TEST_DISPATCH_STATE="+stateDir, "GATOR_TEST_DISPATCH_ENDPOINT="+server.URL,"GATOR_TEST_DISPATCH_SOURCE="+source)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { child.Process.Kill(); child.Wait() }()
	select {
	case <-received:
	case <-ctx.Done():
		t.Fatal("child did not dispatch approved request")
	}
	journal, err := actionlog.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := journal.List("crash-dispatch")
	if err != nil || len(entries) != 1 || !entries[0].Live || entries[0].ConversationID == "" || entries[0].Action.Status != action.Unknown {
		t.Fatal("dispatch happened before durable owner/receipt", entries, err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	entries, err = journal.List("crash-dispatch")
	if err != nil || len(entries) != 1 || entries[0].Live || entries[0].Action.Status != action.Unknown {
		t.Fatal("killed request was forgotten or claimed failed", entries, err)
	}
	reopened, err := actionlog.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := reopened.List(""); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("inspection replayed remote effect", calls.Load())
	}
}
