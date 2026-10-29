package modelcatalog

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ModelCatalogPanel is a focused model-management component for Work.
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
	m := newModel(config)
	m.notice = notice{text: "Choose a local model or a cloud API key. Esc returns to Work.", kind: noticeInfo}
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
	if p.model.screen == closedScreen {
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

// View satisfies Bubble Tea; the React terminal UI renders UISnapshot instead.
func (p *ModelCatalogPanel) View() string { return "" }

func (p *ModelCatalogPanel) focused() bool {
	state := p.model.localModels
	return state.confirmation != localModelNoConfirmation || state.renaming != nil ||
		state.cloudSetup != nil || state.customSetup != nil || state.dependencyHelp
}

func (p *ModelCatalogPanel) confirmationFooter(confirmation localModelConfirmation) string {
	switch confirmation {
	case localModelConfirmStart:
		return "enter/y start with Gator  ·  n/esc start myself"
	case localModelConfirmInstall:
		return "enter/y installation help  ·  n/esc install myself"
	case localModelConfirmRunInstallation:
		return "enter/y run shown action  ·  n/esc cancel"
	case localModelConfirmPull:
		return "enter/y download  ·  esc/n cancel"
	default:
		return "enter/y confirm  ·  esc/n cancel"
	}
}

func (p *ModelCatalogPanel) noticeText() string {
	text := strings.TrimSpace(p.model.notice.text)
	if text == "" || strings.HasPrefix(text, "Choose a local model or a cloud API key") ||
		text == "Local model catalog refreshed." ||
		strings.HasPrefix(text, "Ollama is not installed. Open the Local section") ||
		strings.HasPrefix(text, "Local runtime is unavailable. Open the Local section") {
		return ""
	}
	// The frontend wraps notices. Cutting them to a terminal row can remove
	// the failure reason or the recovery action from the shared snapshot.
	return compact(text, 4096)
}

func (p *ModelCatalogPanel) panelWidth() int {
	if p.model.width <= 0 {
		return 72
	}
	return min(72, max(16, p.model.width-4))
}

func (p *ModelCatalogPanel) Closed() bool { return p.closed }

func (p *ModelCatalogPanel) Close() {
	if p.closed {
		return
	}
	p.closed = true
	p.model.cancelLocalOperation()
}
