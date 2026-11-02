package workapp

import (
	"testing"

	"github.com/gongahkia/gator/internal/worksession"
)

func TestBranchOwnerPreservesRunningParentAndRequiresExactBoundary(t *testing.T) {
	calls := 0
	m := Model{conversation: "work-parent", running: true, revision: "rev-newer", messages: []message{{role: "Gator", runID: "run-saved", revisionID: "rev-saved", text: "Retained answer"}, {role: "You", text: "Still running"}}, config: Config{ManagedSessions: true, ForkConversation: func(parent, revision, run, request string) (worksession.Conversation, error) {
		calls++
		if parent != "work-parent" || revision != "rev-saved" || run != "run-saved" {
			t.Fatal("owner redirected boundary")
		}
		return worksession.Conversation{ID: "work-child"}, nil
	}}}
	a := BranchAction{ConversationID: "work-parent", RevisionID: "rev-saved", RunID: "run-saved", RequestID: "branch-request-123456"}
	for _, change := range []BranchAction{{ConversationID: "work-other", RevisionID: a.RevisionID, RunID: a.RunID, RequestID: a.RequestID}, {ConversationID: a.ConversationID, RevisionID: "rev-newer", RunID: a.RunID, RequestID: a.RequestID}, {ConversationID: a.ConversationID, RevisionID: a.RevisionID, RunID: "run-other", RequestID: a.RequestID}} {
		next, _ := m.BranchConversation(change)
		if calls != 0 || next.(Model).branchResult.Error == "" {
			t.Fatal("invalid boundary reached fork")
		}
	}
	next, cmd := m.BranchConversation(a)
	child := next.(Model)
	if cmd != nil || calls != 1 || child.branchResult.ConversationID != "work-child" || child.sessionRequest.ConversationID != "work-child" {
		t.Fatal("branch was not durably routed")
	}
	if !child.running || child.conversation != m.conversation || child.revision != m.revision || len(child.messages) != 2 {
		t.Fatal("parent state replaced")
	}
	background := m.FocusBridgeSession(false)
	next, _ = background.BranchConversation(a)
	if calls != 1 || next.(Model).branchResult.Error == "" {
		t.Fatal("background stale action created branch")
	}
}
