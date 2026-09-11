package modelcatalog

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	applyWorkSurfaceTheme(p.model.config.Theme)
	if p.help {
		return p.place(p.helpView())
	}
	if p.focused() {
		return p.place(p.focusedView())
	}
	return p.place(p.catalogView())
}

func (p *ModelCatalogPanel) focused() bool {
	state := p.model.localModels
	return state.confirmation != localModelNoConfirmation || state.renaming != nil ||
		state.cloudSetup != nil || state.customSetup != nil || state.dependencyHelp ||
		p.model.oauthLogin != nil
}

func (p *ModelCatalogPanel) catalogView() string {
	width := p.panelWidth()
	var sections []string
	sections = append(sections, p.title("Models"), p.tabs())

	if p.model.localModels.manager == nil {
		sections = append(sections, "", errorStyle.Render("Model management is unavailable."))
		return strings.Join(sections, "\n")
	}
	if p.model.localModels.section == localModelSection {
		sections = append(sections, "", p.localRows(width))
	} else {
		sections = append(sections, "", p.cloudRows(width))
	}
	if p.model.localModels.action != localModelIdle {
		detail := p.model.localModelActionLabel()
		if p.model.localModels.action == localModelPulling {
			detail = p.model.localModelProgressLabel()
		}
		sections = append(sections, "", keyStyle.Render(p.model.localModels.spinner.View()+" "+detail))
	}
	if notice := p.notice(); notice != "" {
		sections = append(sections, "", notice)
	}
	sections = append(sections, "", p.catalogFooter())
	return strings.Join(sections, "\n")
}

func (p *ModelCatalogPanel) cloudRows(width int) string {
	entries := p.model.cloudModels()
	if len(entries) == 0 {
		return dimStyle.Render("No cloud providers are available in this build.")
	}
	limit := p.rowLimit()
	start, end := p.model.visibleRange(len(entries), p.model.localModels.cloudIndex, limit)
	lines := make([]string, 0, end-start+1)
	for index := start; index < end; index++ {
		entry := entries[index]
		lines = append(lines, p.row(entry.name, entry.status, width, index == p.model.localModels.cloudIndex, catalogReady(entry.status)))
	}
	if end < len(entries) {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  … %d more; keep moving to reveal", len(entries)-end)))
	}
	return strings.Join(lines, "\n")
}

func (p *ModelCatalogPanel) localRows(width int) string {
	catalog := p.model.localModels.catalog
	status := "Local runtime unavailable"
	ready := false
	if catalog.RuntimeVersion != "" && catalog.RuntimeError == "" {
		status = "Ollama " + catalog.RuntimeVersion
		ready = true
	} else if p.model.ollamaMissing() {
		status = "Ollama not installed · press i for setup"
	} else if catalog.RuntimeError != "" {
		status = "Ollama unavailable · press s to start"
	}
	lines := []string{p.status(status, ready), ""}
	if len(catalog.Models) == 0 {
		return strings.Join(append(lines, dimStyle.Render(p.model.localModels.spinner.View()+" Loading reviewed models…")), "\n")
	}
	start, end := p.model.visibleRange(len(catalog.Models), p.model.localModels.selected, max(1, p.rowLimit()-2))
	for index := start; index < end; index++ {
		model := catalog.Models[index]
		state := "available · " + model.Download
		modelReady := false
		if model.BlockedReason != "" {
			state = "unavailable · " + model.BlockedReason
		} else if model.Installed {
			state = "installed · " + model.Download
			modelReady = true
		}
		lines = append(lines, p.row(model.Name, state, width, index == p.model.localModels.selected, modelReady))
	}
	if end < len(catalog.Models) {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  … %d more; keep moving to reveal", len(catalog.Models)-end)))
	}
	return strings.Join(lines, "\n")
}

func (p *ModelCatalogPanel) row(name, state string, width int, selected, ready bool) string {
	if width < 36 {
		line := compact(name+" · "+state, max(1, width-2))
		if selected {
			return p.selectedStyle().Width(width).Render("› " + line)
		}
		return "  " + p.status(line, ready)
	}
	nameWidth := min(34, max(14, width/2))
	stateWidth := max(8, width-nameWidth-5)
	name = compact(name, nameWidth)
	state = compact(state, stateWidth)
	if selected {
		return p.selectedStyle().Width(width).Render("› " + fmt.Sprintf("%-*s  %s", nameWidth, name, state))
	}
	return "  " + labelStyle.Copy().Bold(false).Render(fmt.Sprintf("%-*s", nameWidth, name)) + "  " + p.status(state, ready)
}

func (p *ModelCatalogPanel) status(value string, ready bool) string {
	if ready {
		return okStyle.Render(value)
	}
	return dimStyle.Render(value)
}

func catalogReady(status string) bool {
	status = strings.ToLower(status)
	for _, marker := range []string{"signed in", "configured", "stored", "api key set", "ambient credential"} {
		if strings.Contains(status, marker) {
			return true
		}
	}
	return false
}

func (p *ModelCatalogPanel) focusedView() string {
	model := p.model
	model.width = p.panelWidth()
	sections := []string{p.title("Models"), p.tabs(), ""}
	var body, footer string
	state := model.localModels
	switch {
	case state.confirmation != localModelNoConfirmation:
		body = model.modelCatalogConfirmationView()
		footer = p.confirmationFooter(state.confirmation)
	case state.cloudSetup != nil:
		body = p.cloudSetupView(state.cloudSetup)
		footer = "tab next field  ·  enter save  ·  esc cancel"
		if state.cloudSetup.canUseBearerToken() {
			footer = "tab next field  ·  a auth type  ·  enter save  ·  esc cancel"
		}
	case state.customSetup != nil:
		body = p.customSetupView(state.customSetup)
		footer = "tab next field  ·  enter review  ·  esc cancel"
	case state.renaming != nil:
		body = model.fieldView("Rename model", "The provider model ID does not change.", state.renaming.input.View())
		footer = "enter save  ·  esc cancel"
	case state.dependencyHelp:
		body = model.localDependencyHelpView()
		footer = "i/esc close help"
	case model.oauthLogin != nil:
		body = model.fieldView("Cloud sign-in", "Complete sign-in in your browser.", model.commandOutput)
		footer = "ctrl+c cancel sign-in"
	}
	sections = append(sections, body)
	if notice := p.notice(); notice != "" {
		sections = append(sections, "", notice)
	}
	sections = append(sections, "", dimStyle.Render(footer+"  ·  f1 help"))
	return strings.Join(sections, "\n")
}

func (p *ModelCatalogPanel) cloudSetupView(form *cloudModelSetupForm) string {
	sections := []string{
		labelStyle.Render("Configure " + form.provider),
		dimStyle.Render(compact(form.configurationHint(), p.panelWidth())),
	}
	for _, field := range form.fields() {
		label, _, input := form.fieldPresentation(field)
		sections = append(sections, "", labelStyle.Render(label), input.View())
	}
	if form.saving {
		sections = append(sections, "", keyStyle.Render(p.model.localModels.spinner.View()+" Saving…"))
	}
	return strings.Join(sections, "\n")
}

func (p *ModelCatalogPanel) customSetupView(form *customProviderSetupForm) string {
	title := "New custom provider"
	if !form.creating {
		title = "Edit " + form.id.Value()
	}
	sections := []string{
		labelStyle.Render(title),
		dimStyle.Render("Endpoint metadata only; secrets remain in environment variables."),
	}
	for _, field := range form.fields() {
		label, _, input := form.fieldPresentation(field)
		sections = append(sections, "", labelStyle.Render(label), input)
	}
	if form.saving {
		sections = append(sections, "", keyStyle.Render(p.model.localModels.spinner.View()+" Saving…"))
	}
	return strings.Join(sections, "\n")
}

func (p *ModelCatalogPanel) confirmationFooter(confirmation localModelConfirmation) string {
	switch confirmation {
	case localModelConfirmStart:
		return "enter/y start with Gator  ·  n/esc start myself"
	case localModelConfirmInstall:
		return "enter/y installation help  ·  n/esc install myself"
	case localModelConfirmPull:
		return "enter/y download  ·  esc/n cancel"
	default:
		return "enter/y confirm  ·  esc/n cancel"
	}
}

func (p *ModelCatalogPanel) helpView() string {
	return strings.Join([]string{
		p.title("Models"),
		"",
		labelStyle.Render("Browse"),
		"↑/↓ choose  ·  enter use  ·  tab cloud/local  ·  r refresh",
		"",
		labelStyle.Render("Cloud"),
		"c configure  ·  l sign in  ·  n new provider  ·  g discover",
		"d remove credential  ·  x remove provider  ·  e rename",
		"",
		labelStyle.Render("Local"),
		"p download  ·  x remove  ·  e rename  ·  s start Ollama",
		"i installation help",
		"",
		dimStyle.Render("f1/esc close help  ·  ctrl+c cancel operation"),
	}, "\n")
}

func (p *ModelCatalogPanel) tabs() string {
	cloud, local := dimStyle.Render("Cloud"), dimStyle.Render("Local")
	if p.model.localModels.section == cloudModelSection {
		cloud = keyStyle.Render("Cloud")
	} else {
		local = keyStyle.Render("Local")
	}
	return lipgloss.NewStyle().Width(p.panelWidth()).Align(lipgloss.Center).Render(cloud + dimStyle.Render("  /  ") + local)
}

func (p *ModelCatalogPanel) title(value string) string {
	return lipgloss.NewStyle().Width(p.panelWidth()).Align(lipgloss.Center).Render(headerStyle.Render(value))
}

func (p *ModelCatalogPanel) catalogFooter() string {
	if p.model.localModels.section == localModelSection {
		return dimStyle.Render("↑/↓ choose  ·  enter use  ·  p download  ·  tab cloud\n" +
			"s start  ·  i setup  ·  r refresh  ·  esc back  ·  f1 help")
	}
	return dimStyle.Render("↑/↓ choose  ·  enter use  ·  c configure  ·  l sign in\n" +
		"tab local  ·  n provider  ·  g discover  ·  esc back  ·  f1 help")
}

func (p *ModelCatalogPanel) notice() string {
	text := strings.TrimSpace(p.model.notice.text)
	if text == "" || strings.HasPrefix(text, "Choose a cloud or local model") ||
		text == "Local model catalog refreshed." ||
		strings.HasPrefix(text, "Ollama is not installed. Open the Local section") ||
		strings.HasPrefix(text, "Local runtime is unavailable. Open the Local section") {
		return ""
	}
	switch p.model.notice.kind {
	case noticeError:
		return errorStyle.Render(compact(text, p.panelWidth()))
	case noticeSuccess:
		return okStyle.Render(compact(text, p.panelWidth()))
	default:
		return dimStyle.Render(compact(text, p.panelWidth()))
	}
}

func (p *ModelCatalogPanel) selectedStyle() lipgloss.Style {
	switch p.model.config.Theme {
	case "contrast":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("226"))
	case "mono":
		return lipgloss.NewStyle().Reverse(true)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("236"))
	}
}

func (p *ModelCatalogPanel) panelWidth() int {
	if p.model.width <= 0 {
		return 72
	}
	return min(72, max(16, p.model.width-4))
}

func (p *ModelCatalogPanel) rowLimit() int {
	if p.model.height <= 0 {
		return 10
	}
	return max(2, p.model.height-11)
}

func (p *ModelCatalogPanel) place(content string) string {
	width, height := p.model.width, p.model.height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

func (p *ModelCatalogPanel) Closed() bool { return p.closed }

func (p *ModelCatalogPanel) Close() {
	if p.closed {
		return
	}
	p.closed = true
	p.model.cancelLocalOperation()
	if p.model.oauthCancel != nil {
		p.model.oauthCancel()
	}
	if p.model.oauthLogin != nil {
		p.model.oauthLogin.Cancel()
	}
}
