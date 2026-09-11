package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/gongahkia/gator/internal/journal"
)

func TestModelCatalogCompatibilityAdapterExcludesHistoricalDraft(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	draft := journal.Draft{Repository: source, Task: "private Code draft", Provider: "anthropic", Model: "legacy"}
	if err := journal.SaveDraft(state, draft); err != nil {
		t.Fatal(err)
	}

	panel := NewModelCatalogPanel(Config{
		RepositoryPath: source,
		StateDir:       state,
		Provider:       "gemini",
		Model:          "gemini-3.5-flash",
		LocalModels:    &fakeLocalModelManager{},
	})
	defer panel.Close()
	panel.Init()

	view := ansi.Strip(panel.View())
	if !strings.Contains(view, "Models") || !strings.Contains(view, "gemini") {
		t.Fatalf("compatibility adapter did not render the focused catalog:\n%s", view)
	}
	stored, found, err := journal.LoadDraft(state, source)
	if err != nil || !found || stored.Repository != draft.Repository || stored.Task != draft.Task ||
		stored.Provider != draft.Provider || stored.Model != draft.Model {
		t.Fatalf("model catalog changed the historical Code draft: %+v found=%v err=%v", stored, found, err)
	}
}
