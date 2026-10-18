package modelcatalog

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModelPanelSnapshotFollowsChoiceSelection(t *testing.T) {
	panel := NewModelCatalogPanel(Config{})
	initial := panel.UISnapshot()
	if initial.Title != "Models" || len(initial.Rows) != 2 || initial.Selected != 0 {
		t.Fatalf("initial model panel = %#v", initial)
	}
	if _, command := panel.Update(tea.KeyMsg{Type: tea.KeyDown}); command != nil {
		t.Fatalf("unexpected command while moving model choice")
	}
	selected := panel.UISnapshot()
	if selected.Selected != 1 || selected.Rows[1].Label != "Local model" {
		t.Fatalf("selected model panel = %#v", selected)
	}
}

func TestModelPanelSnapshotKeepsDownloadConfirmation(t *testing.T) {
	panel := NewModelCatalogPanel(Config{LocalModels: &fakeLocalManager{}})
	defer panel.Close()
	panel.model.screen = localModelsScreen
	panel.model.localModels.section = localModelSection
	panel.model.localModels.catalog = LocalCatalog{Models: []LocalModel{{
		Name: "Reviewed model", Download: "398 MB", SourceURL: "https://ollama.com/library/example",
	}}}
	panel.model.localModels.confirmation = localModelConfirmPull
	snapshot := panel.UISnapshot()
	if snapshot.Kind != "model-form" || len(snapshot.Blocks) != 1 || !strings.Contains(strings.Join(snapshot.Blocks[0].Lines, " "), "398 MB") || !strings.Contains(snapshot.Footer, "download") {
		t.Fatalf("download confirmation snapshot = %#v", snapshot)
	}
}
