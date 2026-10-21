package modelcatalog

import (
	"strings"

	"github.com/gongahkia/gator/internal/uisnapshot"
)

// UISnapshot exposes model-management state to the terminal and desktop.
// Model selection, credential handling, and confirmations still run in Go.
func (p *ModelCatalogPanel) UISnapshot() uisnapshot.Panel {
	panel := uisnapshot.Panel{Kind: "models", Title: "Models", Notice: p.noticeText()}
	if p.help {
		panel.Title = "Model help"
		panel.Blocks = []uisnapshot.Block{
			{Title: "Browse", Lines: []string{"↑/↓ choose · Enter use · Esc choose another setup · R refresh"}},
			{Title: "Cloud API key", Lines: []string{"C configure API key · N new provider · G discover", "D remove credential · X remove provider · E rename"}},
			{Title: "Local", Lines: []string{"P download · X remove · E rename · S start Ollama", "I installation help"}},
		}
		panel.Footer = "F1 or Esc close help"
		panel.Hints = []uisnapshot.Hint{{Shortcut: "F1/Esc", Action: "close"}}
		return panel
	}
	if p.model.screen == chooseModelScreen {
		panel.Subtitle = "Choose how Gator runs models"
		panel.Rows = []uisnapshot.Row{
			{Label: "Cloud API key", Detail: "Choose a cloud model and configure its API key."},
			{Label: "Local model", Detail: "Browse reviewed Ollama models on this machine."},
		}
		panel.Blocks = []uisnapshot.Block{
			{Title: "Storage", Lines: []string{"Memories, notes, and Work records are saved in private local Gator storage."}},
			{Title: "Retrieval", Lines: []string{"Saved context is selected using basic matching or a separately configured embedding provider. /memory shows readiness and upload settings."}},
			{Title: "Chat processing", Lines: []string{"A cloud chat model receives selected task sources and saved context. Local storage or local retrieval does not keep a cloud chat request on this computer.", "A local Ollama chat model processes its requests here; connected tools contact their selected services."}},
		}
		if p.model.localModels.section == localModelSection {
			panel.Selected = 1
		}
		panel.Footer = "↑/↓ choose · Enter continue · Esc back · F1 help"
		panel.Hints = []uisnapshot.Hint{{Shortcut: "↑/↓", Action: "choose"}, {Shortcut: "Enter", Action: "continue"}, {Shortcut: "Esc", Action: "back"}, {Shortcut: "F1", Action: "help"}}
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
		panel.Hints = []uisnapshot.Hint{{Shortcut: "↑/↓", Action: "choose"}, {Shortcut: "Enter", Action: "use"}, {Shortcut: "C", Action: "configure"}, {Shortcut: "N", Action: "new provider"}, {Shortcut: "G", Action: "discover"}, {Shortcut: "E", Action: "rename"}, {Shortcut: "D", Action: "forget credential"}, {Shortcut: "X", Action: "remove provider"}, {Shortcut: "Esc", Action: "back"}}
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
		panel.Hints = []uisnapshot.Hint{{Shortcut: "↑/↓", Action: "choose"}, {Shortcut: "Enter", Action: "use"}, {Shortcut: "P", Action: "download"}, {Shortcut: "S", Action: "start Ollama"}, {Shortcut: "I", Action: "setup"}, {Shortcut: "E", Action: "rename"}, {Shortcut: "X", Action: "remove model"}, {Shortcut: "R", Action: "refresh"}, {Shortcut: "Esc", Action: "back"}}
	}
	if p.model.localModels.action != localModelIdle {
		panel.Notice = p.model.localModelActionLabel()
		if p.model.localModels.action == localModelPulling || p.model.localModels.action == localModelInstalling {
			panel.Notice = p.model.localModelProgressLabel()
		}
		if p.model.localModels.action == localModelInstalling && len(p.model.localModels.installerLogs) > 0 {
			panel.Blocks = append(panel.Blocks, uisnapshot.Block{Title: "Installer output", Lines: append([]string(nil), p.model.localModels.installerLogs...)})
		}
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
		title, hint, detail := p.model.modelCatalogConfirmationContent()
		panel.Blocks = []uisnapshot.Block{snapshotContent(title, hint, detail)}
		panel.Footer = p.confirmationFooter(state.confirmation) + " · F1 help"
		panel.Hints = []uisnapshot.Hint{{Shortcut: "Enter", Action: "confirm"}, {Shortcut: "Esc", Action: "cancel"}}
		switch state.confirmation {
		case localModelConfirmStart:
			panel.Hints = []uisnapshot.Hint{{Shortcut: "Enter", Action: "start with Gator"}, {Shortcut: "Esc", Action: "start myself"}}
		case localModelConfirmInstall:
			panel.Hints = []uisnapshot.Hint{{Shortcut: "Enter", Action: "installation help"}, {Shortcut: "Esc", Action: "install myself"}}
		case localModelConfirmRunInstallation:
			panel.Hints = []uisnapshot.Hint{{Shortcut: "Enter", Action: "run shown action"}, {Shortcut: "Esc", Action: "cancel"}}
		case localModelConfirmPull:
			panel.Hints = []uisnapshot.Hint{{Shortcut: "Enter", Action: "download"}, {Shortcut: "Esc", Action: "cancel"}}
		}
	case state.cloudSetup != nil:
		panel.Title = "Configure " + state.cloudSetup.provider
		panel.Subtitle = state.cloudSetup.configurationHint()
		for index, field := range state.cloudSetup.fields() {
			label, _, input := state.cloudSetup.fieldPresentation(field)
			value := snapshotValue(input.Value(), input.Placeholder)
			if field == cloudSetupCredentialField && input.Value() != "" {
				value = "••••••••"
			}
			fieldInput := &uisnapshot.Input{Value: input.Value(), Placeholder: input.Placeholder, Limit: input.CharLimit}
			if field == cloudSetupCredentialField {
				fieldInput.Secret, fieldInput.Value = true, ""
			}
			panel.Rows = append(panel.Rows, uisnapshot.Row{Label: label, Detail: value, Input: fieldInput})
			if field == state.cloudSetup.focus {
				panel.Selected = index
			}
		}
		panel.Footer = "Tab next field · Enter save · Esc cancel"
		panel.Hints = []uisnapshot.Hint{{Shortcut: "Tab", Action: "next field"}, {Shortcut: "Enter", Action: "save"}, {Shortcut: "Esc", Action: "cancel"}}
	case state.customSetup != nil:
		panel.Title = "Custom provider"
		panel.Subtitle = "Endpoint metadata only; secrets remain in environment variables."
		for index, field := range state.customSetup.fields() {
			label, value := state.customSetup.snapshotField(field)
			panel.Rows = append(panel.Rows, uisnapshot.Row{Label: label, Detail: value, Input: state.customSetup.bridgeInput(field)})
			if field == state.customSetup.focus {
				panel.Selected = index
			}
		}
		panel.Footer = "Tab next field · Enter review · Esc cancel"
		panel.Hints = []uisnapshot.Hint{{Shortcut: "Tab", Action: "next field"}, {Shortcut: "Enter", Action: "review"}, {Shortcut: "Esc", Action: "cancel"}}
	case state.renaming != nil:
		panel.Title = "Rename model"
		panel.Subtitle = "The provider model ID does not change."
		panel.Rows = []uisnapshot.Row{{Label: "Display name", Detail: state.renaming.input.Value(), Input: &uisnapshot.Input{Value: state.renaming.input.Value(), Limit: 2048}}}
		panel.Footer = "Enter save · Esc cancel"
		panel.Hints = []uisnapshot.Hint{{Shortcut: "Enter", Action: "save"}, {Shortcut: "Esc", Action: "cancel"}}
	case state.dependencyHelp:
		panel.Title = "Local model setup"
		title, hint, detail := p.model.localDependencyHelpContent()
		panel.Blocks = []uisnapshot.Block{snapshotContent(title, hint, detail)}
		panel.Footer = "Enter review installation · I or Esc close help"
		panel.Hints = []uisnapshot.Hint{{Shortcut: "Enter", Action: "review"}, {Shortcut: "Esc", Action: "close"}}
	}
	return panel
}

func snapshotContent(title, hint, detail string) uisnapshot.Block {
	lines := []string{}
	if hint != "" {
		lines = append(lines, hint)
	}
	if detail != "" {
		lines = append(lines, strings.Split(detail, "\n")...)
	}
	return uisnapshot.Block{Title: title, Lines: lines}
}

func snapshotValue(value, placeholder string) string {
	if value == "" {
		return placeholder
	}
	return value
}

func (form *customProviderSetupForm) snapshotField(field customProviderField) (string, string) {
	switch field {
	case customProviderIDField:
		return "Provider ID", snapshotValue(form.id.Value(), form.id.Placeholder)
	case customProviderURLField:
		return "Chat Completions URL", snapshotValue(form.baseURL.Value(), form.baseURL.Placeholder)
	case customProviderEnvField:
		return "API key environment variable", snapshotValue(form.apiKeyEnv.Value(), form.apiKeyEnv.Placeholder)
	case customProviderModelsField:
		return "Model IDs", snapshotValue(form.models.Value(), form.models.Placeholder)
	default:
		return "Default model", snapshotValue(form.defaultModel.Value(), form.defaultModel.Placeholder)
	}
}
