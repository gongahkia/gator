package worksession

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

func branchFixture(t *testing.T) (Store, string, string) {
	t.Helper()
	s, id, dir := requestTestStore(t)
	parent := ""
	for _, name := range []string{"one", "two", "three"} {
		_, err := s.AddRevision(id, Revision{ID: "rev-" + name, ParentRevisionID: parent, SnapshotID: "snap-branch", Objective: "Question " + name, FinalText: "Answer " + name, BundlePath: "/bundles/run-" + name, Status: "completed", AcceptedCode: map[string]string{"candidate": "old-accepted-authority"}, ReplyToUpdateID: "old-update", InputNames: []string{"notes.txt"}, InputIDs: []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/notes.txt"}, Replay: &ReplayState{Version: 1, Provider: "private/model", Messages: []agent.Message{{Role: agent.RoleAgent, ToolCalls: []agent.ToolCall{{ID: "consumed-call", Name: "run_command", Arguments: json.RawMessage(`{"secret":"do-not-copy"}`)}}}, {Role: agent.RoleTool, ToolCallID: "consumed-call", Content: "consumed-action-token"}}, Configuration: json.RawMessage(`{"Connectors":["private-account"],"BrowserSession":"old-session"}`)}})
		if err != nil {
			t.Fatal(err)
		}
		parent = "rev-" + name
	}
	return s, id, dir
}
func TestBranchExactBoundaryCopiesContextWithoutExecutionAuthority(t *testing.T) {
	s, id, dir := branchFixture(t)
	before, _ := os.ReadFile(filepath.Join(s.root, id, "conversation.json"))
	journal, _ := s.LoadRequests(id)
	journal.Requests = []Request{retainedRequest(t, "parent queue remains")}
	if _, err := s.SaveRequests(id, journal); err != nil {
		t.Fatal(err)
	}
	lease, err := s.AcquireExecution(id)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	branch, err := s.ForkChatOnce(id, "rev-two", "run-two", "branch-request-one-123456", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if branch.ID == id || branch.HeadRevision == "rev-two" {
		t.Fatal("branch reused parent identity")
	}
	after, _ := os.ReadFile(filepath.Join(s.root, id, "conversation.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("parent head changed")
	}
	again, err := s.ForkChatOnce(id, "rev-two", "run-two", "branch-request-one-123456", time.Now())
	if err != nil || again.ID != branch.ID {
		t.Fatal("duplicate creation changed identity", err)
	}
	if _, err := s.ForkChatOnce(id, "rev-three", "run-three", "branch-request-one-123456", time.Now()); err == nil {
		t.Fatal("conflicting boundary reused request")
	}
	reopened, _ := Open(dir)
	turns, err := reopened.Lineage(branch.ID, branch.HeadRevision)
	if err != nil || len(turns) != 2 {
		t.Fatal("wrong retained boundary", len(turns), err)
	}
	if turns[0].FinalText != "Answer one" || turns[1].FinalText != "Answer two" {
		t.Fatal("lost visible context")
	}
	for _, turn := range turns {
		raw, _ := json.Marshal(turn)
		for _, secret := range []string{"consumed-call", "consumed-action-token", "old-accepted-authority", "do-not-copy", "old-session", "private-account", "old-update"} {
			if bytes.Contains(raw, []byte(secret)) {
				t.Fatalf("copied authority %s", secret)
			}
		}
		if turn.Replay != nil || turn.AcceptedCode != nil {
			t.Fatal("native replay copied")
		}
	}
	info, err := reopened.Branch(branch.ID)
	if err != nil || info.ParentConversationID != id || info.ParentRevisionID != "rev-two" || len(info.Revisions) != 2 {
		t.Fatal("ancestry lost", err)
	}
	if len(turns[0].InputIDs) != 1 || turns[0].InputNames[0] != "notes.txt" {
		t.Fatal("retained input references lost")
	}
	childRequests, err := s.LoadRequests(branch.ID)
	if err != nil || len(childRequests.Requests) != 0 {
		t.Fatal("queue inherited", err)
	}
	originalRequests, _ := s.LoadRequests(id)
	if len(originalRequests.Requests) != 1 {
		t.Fatal("parent queue affected")
	}
	childLease, err := s.AcquireExecution(branch.ID)
	if err != nil {
		t.Fatal("parent worker stole branch ownership", err)
	}
	childLease.Close()
	prefs, err := s.LoadPreferences(branch.ID)
	if err != nil || prefs.ConnectorIDs == nil || len(*prefs.ConnectorIDs) != 0 {
		t.Fatal("external accounts inherited", err)
	}
	nested, err := s.ForkChatOnce(branch.ID, branch.HeadRevision, "run-two", "nested-branch-request-123456", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	nestedInfo, _ := s.Branch(nested.ID)
	if nestedInfo.ParentConversationID != branch.ID || nestedInfo.Revisions[1].ConversationID != id || nestedInfo.Revisions[1].SourceRevisionID != "rev-two" {
		t.Fatal("nested source provenance did not resolve original")
	}
}
func TestBranchConcurrentClaimsAndDeletionNeverRecreate(t *testing.T) {
	s, id, _ := branchFixture(t)
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := s.ForkChatOnce(id, "rev-one", "run-one", "concurrent-branch-request-123456", time.Now())
			if err != nil {
				failures <- err
			} else {
				ids <- c.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	selected := ""
	for id := range ids {
		if selected != "" && selected != id {
			t.Fatal("duplicate publication")
		}
		selected = id
	}
	if err := os.RemoveAll(filepath.Join(s.root, selected)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ForkChatOnce(id, "rev-one", "run-one", "concurrent-branch-request-123456", time.Now()); err == nil {
		t.Fatal("deleted branch resurrected")
	}
}
func TestBranchRejectsMissingStaleAndUnfinishedBoundaries(t *testing.T) {
	s, id, _ := branchFixture(t)
	for _, test := range []struct{ parent, revision, run string }{{id, "missing", "run-one"}, {id, "rev-two", "run-one"}, {"missing", "rev-one", "run-one"}} {
		if _, err := s.ForkChatOnce(test.parent, test.revision, test.run, "invalid-branch-request-123456", time.Now()); err == nil {
			t.Fatal("wrong boundary accepted")
		}
	}
	_, err := s.AddRevision(id, Revision{ID: "rev-failed", ParentRevisionID: "rev-three", SnapshotID: "snap-branch", Objective: "Failed", FinalText: "Partial", BundlePath: "/bundles/run-failed", Status: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ForkChatOnce(id, "rev-failed", "run-failed", "failed-branch-request-123456", time.Now()); err == nil {
		t.Fatal("unfinished branch accepted")
	}
}
