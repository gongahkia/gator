package tui

import tea "github.com/charmbracelet/bubbletea"

// ModelCatalogPanel reuses model management inside Work without exposing the
// historical Code composer or its private draft state.
type ModelCatalogPanel struct {
	model  Model
	closed bool
	help   bool
}

type catalogPanelMessage struct {
	panel   *ModelCatalogPanel
	message tea.Msg
}

func NewModelCatalogPanel(config Config) *ModelCatalogPanel {
	config.NewExecutor = nil
	config.NewDelegateCommand = nil
	config.NewConnectCommand = nil
	config.ResumeStatePath, config.ForkStatePath = "", ""
	config.StartInRecent = false
	m := newModel(config, true)
	m.provider.SetValue(config.Provider)
	m.screen = localModelsScreen
	m.notice = notice{text: "Choose a cloud or local model. Esc returns to Work.", kind: noticeInfo}
	return &ModelCatalogPanel{model: m}
}

func (p *ModelCatalogPanel) Init() tea.Cmd {
	next, command := p.model.openModelCatalog()
	p.model = next.(Model)
	return p.command(command)
}

func (p *ModelCatalogPanel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if p.closed {
		return p, nil
	}
	if wrapped, ok := message.(catalogPanelMessage); ok {
		if wrapped.panel != p {
			return p, nil
		}
		message = wrapped.message
	}
	var next tea.Model
	var command tea.Cmd
	if key, ok := message.(tea.KeyMsg); ok {
		if key.String() == "f1" {
			p.help = !p.help
			return p, nil
		}
		if p.help {
			if key.String() == "esc" {
				p.help = false
			}
			return p, nil
		}
		if key.String() == "ctrl+q" {
			p.Close()
			return p, nil
		}
		next, command = p.model.updateLocalModels(key)
	} else {
		next, command = p.model.Update(message)
	}
	p.model = next.(Model)
	if p.model.screen != localModelsScreen {
		p.Close()
		return p, nil
	}
	return p, p.command(command)
}

func (p *ModelCatalogPanel) command(command tea.Cmd) tea.Cmd {
	if command == nil {
		return nil
	}
	return func() tea.Msg {
		message := command()
		if batch, ok := message.(tea.BatchMsg); ok {
			commands := make(tea.BatchMsg, len(batch))
			for i, item := range batch {
				commands[i] = p.command(item)
			}
			return commands
		}
		return catalogPanelMessage{panel: p, message: message}
	}
}

func (p *ModelCatalogPanel) View() string {
	if p.help {
		return p.model.fitToTerminal("Models\n\nTab: switch Cloud / Local\n↑/↓: choose a model\nEnter/u: select model\np: download local model\nx: delete local model (confirmation required)\ne: rename display label\ns: start local runtime\ni: installation help\nr: refresh\nc: configure cloud model\nl: cloud sign-in\nn: add custom provider\ng: discover custom provider models\nd: remove stored cloud credential\n\nEsc/Ctrl+C: cancel a model operation\nEsc: return to Work when idle\nF1/Esc: close this help")
	}
	return p.model.fitToTerminal(p.model.localModelsView())
}

func (p *ModelCatalogPanel) Closed() bool { return p.closed }

func (p *ModelCatalogPanel) Close() {
	p.closed = true
	p.model.cancelLocalOperation()
	if p.model.oauthCancel != nil {
		p.model.oauthCancel()
	}
	if p.model.oauthLogin != nil {
		p.model.oauthLogin.Cancel()
	}
	// the Work application owns the local runtime across catalog visits.
	p.model.terminalRegistry.Close()
	p.model.lspRegistry.Close()
}
