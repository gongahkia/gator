package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
)

func TestLinearEdges(t *testing.T) {
	nodes := []domain.GraphNode{{ID: "input", Kind: "input", Label: "Input"}, {ID: "output", Kind: "output", Label: "Output"}}
	edges := linearEdges(nodes)
	if len(edges) != 1 || edges[0].Source != "input" || edges[0].Target != "output" {
		t.Fatalf("bad edges %#v", edges)
	}
}

func TestCreateModeAcceptsTextInput(t *testing.T) {
	input := textinput.New()
	input.Focus()
	model := Model{mode: createMode, input: input, selections: map[domain.Stage]string{}}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	updated := next.(Model)
	if updated.input.Value() != "a" {
		t.Fatalf("input=%q", updated.input.Value())
	}
}

func TestProviderSelectionCyclesWithinStage(t *testing.T) {
	model := Model{selections: map[domain.Stage]string{domain.StagePlanner: "one"}, providers: []config.Provider{
		{ID: "one", Stages: []domain.Stage{domain.StagePlanner}},
		{ID: "two", Stages: []domain.Stage{domain.StagePlanner}},
	}}
	model.cycleProvider(1)
	if model.selections[domain.StagePlanner] != "two" {
		t.Fatalf("provider=%q", model.selections[domain.StagePlanner])
	}
}
