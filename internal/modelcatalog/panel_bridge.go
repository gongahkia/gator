package modelcatalog

import "github.com/gongahkia/gator/internal/uisnapshot"

// SelectBridgeRow changes focus only. Desktop activation still calls Update
// with the same key as the terminal, preserving confirmations and validation.
func (p *ModelCatalogPanel) SelectBridgeRow(index int) {
	panel := p.UISnapshot()
	if index < 0 || index >= len(panel.Rows) {
		return
	}
	state := &p.model.localModels
	switch {
	case state.cloudSetup != nil:
		state.cloudSetup.focus = state.cloudSetup.fields()[index]
		state.cloudSetup.focusInput()
	case state.customSetup != nil:
		state.customSetup.focus = state.customSetup.fields()[index]
		state.customSetup.focusInput()
	case p.model.screen == chooseModelScreen:
		state.section = cloudModelSection
		if index == 1 {
			state.section = localModelSection
		}
	case state.section == cloudModelSection:
		state.cloudIndex = index
	default:
		state.selected = index
	}
}

func (p *ModelCatalogPanel) SetBridgeField(index int, value string) {
	state := &p.model.localModels
	panel := p.UISnapshot()
	if index < 0 || index >= len(panel.Rows) || panel.Rows[index].Input == nil {
		return
	}
	switch {
	case state.cloudSetup != nil && !state.cloudSetup.saving:
		_, _, input := state.cloudSetup.fieldPresentation(state.cloudSetup.fields()[index])
		input.SetValue(value)
	case state.customSetup != nil && !state.customSetup.saving:
		form := state.customSetup
		switch form.fields()[index] {
		case customProviderIDField:
			form.id.SetValue(value)
		case customProviderURLField:
			form.baseURL.SetValue(value)
		case customProviderEnvField:
			form.apiKeyEnv.SetValue(value)
		case customProviderModelsField:
			form.models.SetValue(value)
		default:
			form.defaultModel.SetValue(value)
		}
	case state.renaming != nil:
		state.renaming.input.SetValue(value)
	}
}

func (form *customProviderSetupForm) bridgeInput(field customProviderField) *uisnapshot.Input {
	input := &uisnapshot.Input{Limit: 2048}
	switch field {
	case customProviderIDField:
		input.Value, input.Placeholder = form.id.Value(), form.id.Placeholder
	case customProviderURLField:
		input.Value, input.Placeholder = form.baseURL.Value(), form.baseURL.Placeholder
	case customProviderEnvField:
		input.Value, input.Placeholder = form.apiKeyEnv.Value(), form.apiKeyEnv.Placeholder
	case customProviderModelsField:
		input.Value, input.Multiline, input.Limit = form.models.Value(), true, 64*1024
	default:
		input.Value, input.Placeholder = form.defaultModel.Value(), form.defaultModel.Placeholder
	}
	return input
}
