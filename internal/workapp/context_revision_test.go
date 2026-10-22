package workapp

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSavedKnowledgeKeepsDraftOnConflictAndBindsNextEditToSavedVersion(t *testing.T) {
	var received []string
	reject := true
	m := New(Config{ChangeKnowledge: func(id, action, text, goal, revision string) (string, error) {
		received = append(received, revision)
		if reject {
			return "", errors.New("this memory changed; reopen its current saved version before editing")
		}
		return "revision-2", nil
	}})
	m.section = "context"
	m.contextState.Items = []KnowledgeItem{{ID: "memory", Kind: "memory", CurrentText: "Original", Text: "Original", Revision: "revision-1", Available: true}}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m.contextEdit = "Edited text"
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.contextEditing || m.contextEdit != "Edited text" || m.contextDetail.CurrentText != "Original" || !strings.Contains(m.sectionNotice, "changed") {
		t.Fatal("conflict discarded draft or claimed success")
	}
	reject = false
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.contextEditing || m.contextDetail.Revision != "revision-2" || m.contextState.Items[0].Revision != "revision-2" {
		t.Fatal("save did not retain new exact revision")
	}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if strings.Join(received, ",") != "revision-1,revision-1,revision-2" {
		t.Fatalf("reviewed revisions = %v", received)
	}
}

func TestSavedKnowledgeKeepsForgetReviewOnConflict(t *testing.T) {
	m := New(Config{ChangeKnowledge: func(id, action, text, goal, revision string) (string, error) {
		if action != "forget" || revision != "displayed" {
			t.Fatalf("wrong review: %s %s", action, revision)
		}
		return "", errors.New("this note changed; reopen its current saved version before editing")
	}})
	m.section = "context"
	m.contextState.Items = []KnowledgeItem{{ID: "note", Kind: "note", Revision: "displayed", Available: true}}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.contextConfirm || !m.contextDetail.Available || len(m.contextState.Items) != 1 || !strings.Contains(m.sectionNotice, "changed") {
		t.Fatal("failed forget lost review or claimed deletion")
	}
}

func TestHistoricalContextKeepsExcerptWhileAdvancingSavedRevision(t *testing.T) {
	m := New(Config{ChangeKnowledge: func(_, _, _, _, _ string) (string, error) { return "revision-2", nil }})
	m.section, m.contextWorkID = "context", "earlier-run"
	m.contextState.Items = []KnowledgeItem{{ID: "memory", Kind: "memory", Text: "Earlier excerpt", CurrentText: "Current text", Revision: "revision-1", Available: true}}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m.contextEdit = "Corrected text"
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.contextDetail.Text != "Earlier excerpt" || m.contextDetail.CurrentText != "Corrected text" || m.contextDetail.Revision != "revision-2" {
		t.Fatal("reopened historical context lost either the excerpt or the corrected saved revision")
	}
}
