package workapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestComposerSourcesChangeOnlyFutureOptionsAndKeepQueueBinding(t *testing.T) {
	saved := []string{}
	m := New(Config{ComposerSources: func() ([]ComposerSource, error) {
		return []ComposerSource{{ID: "first", Label: "First", Binding: "private-secret-account-fingerprint"}, {ID: "second", Label: "Second"}}, nil
	}, SelectComposerSources: func(id string, expected, ids []string) error {
		if id != "chat" || !slices.Equal(expected, []string{"first"}) {
			t.Fatal("wrong selection scope")
		}
		saved = slices.Clone(ids)
		return nil
	}})
	m.conversation = "chat"
	m.options.ConnectorIDs = []string{"first"}
	m.running = true
	m.queue = []queuedRun{{id: 1, prompt: "queued", options: cloneRunOptions(m.options)}}
	m.input = "keep my draft"
	action := SourceAction{ID: m.composerSourceSettings().ID, RequestID: "selection", Action: "select", IDs: []string{"second"}}
	next, cmd := m.ApplySourceSelection(action)
	m = next.(Model)
	if cmd != nil || m.sourcesResult.Error != "" || !slices.Equal(saved, []string{"second"}) || !slices.Equal(m.queue[0].options.ConnectorIDs, []string{"first"}) || m.input != "keep my draft" || !m.running {
		t.Fatalf("source selection altered execution: %#v", m.sourcesResult)
	}
	encoded, _ := json.Marshal(m.BridgeState())
	if strings.Contains(string(encoded), "private-secret") {
		t.Fatal("source inventory leaked private binding")
	}
}
func TestComposerSourcesRejectStaleUnavailableWrongChatAndFailedSaves(t *testing.T) {
	for _, scenario := range []string{"binding", "removed", "unavailable", "wrong-chat", "duplicate", "save-error", "unknown", "panel"} {
		t.Run(scenario, func(t *testing.T) {
			choice := ComposerSource{ID: "first", Binding: "original"}
			writes := 0
			m := New(Config{ComposerSources: func() ([]ComposerSource, error) {
				if scenario == "removed" && choice.ID == "gone" {
					return nil, nil
				}
				return []ComposerSource{choice}, nil
			}, SelectComposerSources: func(string, []string, []string) error { writes++; return errors.New("save refused") }})
			m.conversation = "chat"
			action := SourceAction{ID: m.composerSourceSettings().ID, RequestID: "request", Action: "select", IDs: []string{"first"}}
			switch scenario {
			case "binding":
				choice.Binding = "revoked"
			case "removed":
				choice.ID = "gone"
			case "unavailable":
				choice.Unavailable = "expired"
			case "wrong-chat":
				m.conversation = "other"
			case "duplicate":
				action.IDs = append(action.IDs, "first")
			case "unknown":
				action.Action = "grant"
			case "panel":
				m.section = "settings"
			}
			next, _ := m.ApplySourceSelection(action)
			result := next.(Model)
			if result.sourcesResult.Error == "" || len(result.options.ConnectorIDs) > 0 {
				t.Fatal("invalid source selection succeeded")
			}
			if scenario != "save-error" && writes != 0 {
				t.Fatal("rejection reached persistence")
			}
		})
	}
}
func TestComposerSourcesRefreshIsReadOnlyAndBounded(t *testing.T) {
	calls := 0
	m := New(Config{ComposerSources: func() ([]ComposerSource, error) { calls++; return make([]ComposerSource, 140), nil }})
	before := calls
	next, cmd := m.ApplySourceSelection(SourceAction{RequestID: "refresh", Action: "refresh"})
	m = next.(Model)
	if cmd != nil || calls != before+1 || len(m.composerSourceSettings().Choices) != 128 || m.sourcesIssue == "" || len(m.messages) > 0 {
		t.Fatal("refresh was not a bounded local read")
	}
}

func TestComposerSourcesRespectNativeSixteenConnectorLimit(t *testing.T) {
	choices := []ComposerSource{}
	ids := []string{}
	for i := 0; i < 17; i++ {
		id := fmt.Sprintf("source-%d", i)
		choices = append(choices, ComposerSource{ID: id})
		ids = append(ids, id)
	}
	m := New(Config{ComposerSources: func() ([]ComposerSource, error) { return choices, nil }})
	next, _ := m.ApplySourceSelection(SourceAction{ID: m.composerSourceSettings().ID, RequestID: "too-many", Action: "select", IDs: ids})
	if next.(Model).sourcesResult.Error == "" {
		t.Fatal("seventeen selected sources exceed the native tool surface")
	}
}
