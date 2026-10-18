package modelcatalog

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/gongahkia/gator/internal/uisnapshot"
)

// UISnapshot exposes model-management state to the React terminal renderer.
// Model selection, credential handling, and confirmations still run in Go.
func (p *ModelCatalogPanel) UISnapshot() uisnapshot.Panel {
	panel := uisnapshot.Panel{Kind: "models", Title: "Models", Notice: ansi.Strip(p.notice())}
	if p.help {
		panel.Title = "Model help"
		panel.Blocks = []uisnapshot.Block{
			{Title: "Browse", Lines: []string{"↑/↓ choose · Enter use · Esc choose another setup · R refresh"}},
			{Title: "Cloud API key", Lines: []string{"C configure API key · N new provider · G discover", "D remove credential · X remove provider · E rename"}},
			{Title: "Local", Lines: []string{"P download · X remove · E rename · S start Ollama", "I installation help"}},
		}
		panel.Footer = "F1 or Esc close help"
		return panel
	}
	if p.model.screen == chooseModelScreen {
		panel.Subtitle = "Choose how Gator runs models"
		panel.Rows = []uisnapshot.Row{
			{Label: "Cloud API key", Detail: "Choose a cloud model and configure its API key."},
			{Label: "Local model", Detail: "Browse reviewed Ollama models on this machine."},
		}
		if p.model.localModels.section == localModelSection {
			panel.Selected = 1
		}
		panel.Footer = "↑/↓ choose · Enter continue · Esc back · F1 help"
		return panel
	}
	if p.focused() {
		return p.focusedSnapshot(panel)
	}
	if p.model.localModels.section == cloudModelSection {
		panel.Title = "Cloud models"
		panel.Selected = p.model.localModels.cloudIndex
		for _, entry := range p.model.cloudModels() {
			panel.Rows = append(panel.Rows, uisnapshot.Row{Label: entry.name, Detail: entry.status})
		}
		panel.Footer = "↑/↓ choose · Enter use · C configure key · D forget · N provider · G discover · Esc back · F1 help"
	} else {
		panel.Title = "Local models"
		panel.Selected = p.model.localModels.selected
		catalog := p.model.localModels.catalog
		if catalog.RuntimeVersion != "" && catalog.RuntimeError == "" {
			panel.Subtitle = "Ollama " + catalog.RuntimeVersion
		} else if p.model.ollamaMissing() {
			panel.Subtitle = "Ollama not installed · I for setup"
		} else {
			panel.Subtitle = "Ollama unavailable · S to start"
		}
		for _, model := range catalog.Models {
			state := "available · " + model.Download
			if model.BlockedReason != "" {
				state = "unavailable · " + model.BlockedReason
			} else if model.Installed {
				state = "installed · " + model.Download
			}
			panel.Rows = append(panel.Rows, uisnapshot.Row{Label: model.Name, Detail: state, State: model.Category})
		}
		panel.Footer = "↑/↓ choose · Enter use · P download · X remove · S start · I setup · R refresh · Esc back · F1 help"
	}
	if p.model.localModels.action != localModelIdle {
		panel.Notice = p.model.localModelActionLabel()
	}
	if len(panel.Rows) == 0 && panel.Notice == "" {
		panel.Notice = "No models available yet."
	}
	return panel
}

func (p *ModelCatalogPanel) focusedSnapshot(panel uisnapshot.Panel) uisnapshot.Panel {
	panel.Kind = "model-form"
	state := p.model.localModels
	switch {
	case state.confirmation != localModelNoConfirmation:
		panel.Title = "Confirm model action"
		panel.Blocks = []uisnapshot.Block{{Lines: cleanPanelLines(p.model.modelCatalogConfirmationView())}}
		panel.Footer = p.confirmationFooter(state.confirmation) + " · F1 help"
	case state.cloudSetup != nil:
		panel.Title = "Configure " + state.cloudSetup.provider
		panel.Subtitle = state.cloudSetup.configurationHint()
		for index, field := range state.cloudSetup.fields() {
			label, _, input := state.cloudSetup.fieldPresentation(field)
			panel.Rows = append(panel.Rows, uisnapshot.Row{Label: label, Detail: ansi.Strip(input.View())})
			if field == state.cloudSetup.focus {
				panel.Selected = index
			}
		}
		panel.Footer = "Tab next field · Enter save · Esc cancel"
	case state.customSetup != nil:
		panel.Title = "Custom provider"
		panel.Subtitle = "Endpoint metadata only; secrets remain in environment variables."
		for index, field := range state.customSetup.fields() {
			label, _, input := state.customSetup.fieldPresentation(field)
			panel.Rows = append(panel.Rows, uisnapshot.Row{Label: label, Detail: ansi.Strip(input)})
			if field == state.customSetup.focus {
				panel.Selected = index
			}
		}
		panel.Footer = "Tab next field · Enter review · Esc cancel"
	case state.renaming != nil:
		panel.Title = "Rename model"
		panel.Subtitle = "The provider model ID does not change."
		panel.Rows = []uisnapshot.Row{{Label: "Display name", Detail: state.renaming.input.Value()}}
		panel.Footer = "Enter save · Esc cancel"
	case state.dependencyHelp:
		panel.Title = "Local model setup"
		panel.Blocks = []uisnapshot.Block{{Lines: cleanPanelLines(p.model.localDependencyHelpView())}}
		panel.Footer = "Enter review installation · I or Esc close help"
	}
	return panel
}

func cleanPanelLines(value string) []string {
	value = strings.TrimSpace(ansi.Strip(value))
	if value == "" {
		return nil
	}
	return strings.Split(value, "\n")
}
