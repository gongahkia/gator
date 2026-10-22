package workapp

import (
	"errors"
	"testing"

	"github.com/gongahkia/gator/internal/worksession"
)

func TestMainConversationResumesWithItsOwnScopeAndOptions(t *testing.T) {
	mainID := "work-main"
	model := New(Config{
		CurrentFolder: "/private-default", PrimaryConversation: func() (string, error) { return mainID, nil },
		Conversations: []worksession.Conversation{{ID: "work-other", SourcePath: "/other", Title: "Unrelated"}},
		LoadConversation: func(id string) (ConversationState, error) {
			if id != mainID {
				t.Fatalf("loaded wrong topic: %s", id)
			}
			return ConversationState{Title: "My plans", SourcePath: "/main-scope", Messages: []TranscriptMessage{{Role: "You", Text: "Keep this separate"}}}, nil
		},
		LoadConversationOptions: func(string) (RunOptions, error) {
			return RunOptions{Mode: "inspect", ConnectorIDs: []string{"chosen"}}, nil
		},
	})
	state := model.BridgeState()
	if !state.Primary || state.ConversationID != mainID || state.Source != "/main-scope" || len(state.Messages) != 1 || state.WorkDetails.ConnectorIDs[0] != "chosen" {
		t.Fatalf("bad resume: %+v", state)
	}
	next, _ := model.NavigateBridge("/new")
	model = next.(Model)
	if model.primary || model.conversation != "" {
		t.Fatal("new topic reused main conversation")
	}
	next, _ = model.NavigateBridge("/gator")
	model = next.(Model)
	if !model.primary || model.conversation != mainID || model.source != "/main-scope" || len(model.messages) != 1 {
		t.Fatal("returning to main lost its independent state")
	}
}

func TestMainConversationBindsOnlyItsOwnFirstRetainedRun(t *testing.T) {
	var bound string
	config := Config{CurrentFolder: "/private", PrimaryConversation: func() (string, error) { return "", nil }, BindPrimaryConversation: func(id string) error { bound = id; return nil }}
	model := New(config)
	next, _ := model.updateRunDone(runDone{ConversationID: "work-main", FinalText: "Hello"})
	model = next.(Model)
	if bound != "work-main" || !model.primary || model.primaryID != bound {
		t.Fatal("didn't retain the main identity")
	}
	bound = ""
	next, _ = model.NavigateBridge("/new")
	model = next.(Model)
	_, _ = model.updateRunDone(runDone{ConversationID: "work-side", FinalText: "Separate"})
	if bound != "" {
		t.Fatal("separate topic replaced main")
	}
	config.BindPrimaryConversation = func(string) error { return errors.New("another window saved first") }
	model = New(config)
	next, _ = model.updateRunDone(runDone{ConversationID: "work-race", FinalText: "Kept"})
	model = next.(Model)
	if model.primary || model.conversation != "work-race" || len(model.messages) != 1 || model.bridgeNotice == "" {
		t.Fatal("failed designation lost the separate retained conversation")
	}
}

func TestMainConversationDoesNotBypassActiveWorkOrAdoptExistingTopics(t *testing.T) {
	model := New(Config{CurrentFolder: "/private", PrimaryConversation: func() (string, error) { return "", nil }, Conversations: []worksession.Conversation{{ID: "work-existing", SourcePath: "/sensitive"}}})
	if model.conversation != "" || model.source != "/private" || !model.primary {
		t.Fatal("adopted an unrelated topic")
	}
	model.running = true
	next, _ := model.NavigateBridge("/gator")
	if !next.(Model).running || next.(Model).bridgeNotice == "" {
		t.Fatal("main navigation bypassed current work")
	}
}

func TestMainConversationUsesResetPrivateFolderAfterDataRemoval(t *testing.T) {
	m := New(Config{CurrentFolder: "/old-private", PrimaryConversation: func() (string, error) { return "", nil }, ResetPrivateState: func() (GoalState, error) { return GoalState{SourcePath: "/fresh-private", Title: "Fresh"}, nil }})
	m.clearRemovedData(ControlScreen{ResetConversation: true, ClearGoal: true})
	next, _ := m.NavigateBridge("/gator")
	m = next.(Model)
	if !m.primary || m.source != "/fresh-private" || m.conversation != "" {
		t.Fatal("main conversation returned to a removed source")
	}
}

func TestClosingRetainsTheFirstMainIdentityWithoutResumingWork(t *testing.T) {
	var bound string
	m := New(Config{CurrentFolder: t.TempDir(), PrimaryConversation: func() (string, error) { return "", nil }, BindPrimaryConversation: func(id string) error { bound = id; return nil }})
	m.running, m.quitting = true, true
	next, quit := m.updateRunDone(runDone{ConversationID: "work-retained-before-close", Error: "cancelled"})
	m = next.(Model)
	if bound != "work-retained-before-close" || m.running || quit == nil {
		t.Fatal("close lost the retained main identity or continued execution")
	}
}
