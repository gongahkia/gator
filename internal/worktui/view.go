package worktui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gongahkia/gator/internal/rattles"
)

func (m Model) View() string {
	if m.models != nil {
		return m.models.View()
	}
	width := m.width
	if width <= 0 {
		width = 80
	} else if width < 20 {
		width = 20
	}
	height := m.height
	if height <= 0 {
		height = 24
	} else if height < 8 {
		height = 8
	}
	accent, dim, selectedStyle := workStyles(m.theme)
	if m.launcher {
		if m.launcherMode == "status-line" {
			return m.renderStatusLineEditor(width, height, accent, dim, selectedStyle)
		}
		return m.renderPalette(width, height, accent, dim, selectedStyle)
	}
	if m.section != "" {
		return m.renderSection(width, accent, dim)
	}
	if m.home {
		return m.renderHome(width, height, accent, dim)
	}
	var view strings.Builder
	view.WriteString(accent.Render(gatorWordmark))
	if m.title != "" {
		view.WriteString(dim.Render("  " + m.title))
	}
	if m.status != "" {
		view.WriteString(dim.Render(" · " + m.status))
	}
	view.WriteString("\n\n")
	var transcript strings.Builder
	messageStyle := lipgloss.NewStyle().Width(max(20, width-4))
	for _, item := range m.messages {
		transcript.WriteString(messageStyle.Render(accent.Render(item.role+":") + " " + item.text))
		transcript.WriteString("\n\n")
	}
	footer := wrapStatusLine(m.statusLineItems(), width)
	footerRows := 0
	if footer != "" {
		footerRows = strings.Count(footer, "\n") + 1
	}
	lines := strings.Split(strings.TrimSuffix(transcript.String(), "\n"), "\n")
	available := max(4, height-9-footerRows)
	maxScroll := max(0, len(lines)-available)
	scroll := min(m.scroll, maxScroll)
	end := len(lines) - scroll
	start := max(0, end-available)
	if len(lines) > 0 && lines[0] != "" {
		view.WriteString(strings.Join(lines[start:end], "\n") + "\n")
	}
	if m.running {
		view.WriteString(accent.Render(rattles.BrailleDots.Frame(m.loadingFrame)+" Working…") + "\n\n")
	}
	view.WriteString(m.renderComposer(width, !m.running))
	if footer != "" {
		view.WriteString("\n" + dim.Render(footer))
	}
	return view.String()
}

func (m Model) renderHome(width, height int, accent, dim lipgloss.Style) string {
	title := accent.Copy().Bold(true).Render(gatorWordmark)
	question := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Render("What do you want to accomplish?")
	body := title + "\n\n" + question + "\n\n" + m.renderComposer(width, true)
	if footer := wrapStatusLine(m.statusLineItems(), width); footer != "" {
		body += "\n" + dim.Render(footer)
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, body)
}

func (m Model) renderPalette(width, height int, accent, dim, selectedStyle lipgloss.Style) string {
	panelWidth := min(76, max(16, width-4))
	titleWidth := min(28, max(8, panelWidth/2-1))
	subtitleWidth := max(0, panelWidth-titleWidth-3)
	visible := m.filteredEntries()
	selected := min(m.selected, max(0, len(visible)-1))
	maximum := max(1, height-9)
	start := 0
	if selected >= maximum {
		start = selected - maximum + 1
	}
	end := min(len(visible), start+maximum)
	var panel strings.Builder
	title := "Commands"
	if m.launcherMode == "conversations" {
		title = "Conversations"
	} else if m.launcherMode == "setup" {
		title = "Choose default provider"
	} else if m.launcherMode == "connect" {
		title = "Connect provider"
	} else if m.launcherMode == "login" {
		title = "Log in to provider"
	} else if m.launcherMode == "logout" {
		title = "Log out of provider"
	}
	panel.WriteString(accent.Render(title) + "\n")
	query := m.paletteQuery
	if query == "" {
		query = dim.Render("type to filter")
	}
	panel.WriteString("Search  " + query + accent.Render("█") + "\n\n")
	if len(visible) == 0 {
		empty := "No matching commands."
		if m.launcherMode == "conversations" && m.paletteQuery == "" {
			empty = "No retained conversations yet."
		} else if m.launcherMode != "commands" {
			empty = "No matching providers."
		}
		panel.WriteString(dim.Render(empty) + "\n")
	}
	rowStyle := lipgloss.NewStyle().Width(panelWidth)
	for index := start; index < end; index++ {
		item := visible[index]
		title := truncate(item.title, titleWidth)
		subtitle := truncate(item.subtitle, subtitleWidth)
		line := rowStyle.Render(fmt.Sprintf("  %-*s %s", titleWidth, title, dim.Render(subtitle)))
		if index == selected {
			line = selectedStyle.Width(panelWidth).Render("› " + fmt.Sprintf("%-*s %s", titleWidth, title, subtitle))
		}
		panel.WriteString(line + "\n")
	}
	panel.WriteString("\n" + dim.Render("type filter  ·  ↑/↓ choose  ·  enter run  ·  esc close"))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel.String())
}

func (m Model) renderSection(width int, accent, dim lipgloss.Style) string {
	var view strings.Builder
	title := "Inbox"
	if m.section == "jobs" {
		title = "Scheduled jobs"
	}
	view.WriteString(accent.Render(gatorWordmark) + dim.Render("  "+title) + "\n\n")
	messageStyle := lipgloss.NewStyle().Width(max(20, width-4))
	if m.section == "inbox" {
		if len(m.config.Inbox) == 0 {
			view.WriteString(dim.Render("No recent results.") + "\n")
		}
		for _, item := range m.config.Inbox {
			view.WriteString(messageStyle.Render(accent.Render(item.Status+":") + " " + item.Title + "\n" + item.Summary))
			view.WriteString("\n\n")
		}
	} else {
		if len(m.config.Jobs) == 0 {
			view.WriteString(dim.Render("No scheduled jobs. Create one with `gator job add`.") + "\n")
		}
		for _, job := range m.config.Jobs {
			state := "disabled"
			if job.Enabled {
				state = "enabled"
			}
			view.WriteString(messageStyle.Render(accent.Render(state+":") + " " + job.Name + "\n" + job.Schedule + " · " + job.Timezone))
			view.WriteString("\n\n")
		}
	}
	footer := wrapStatusLine([]string{"esc back", "ctrl+p commands", "ctrl+x conversations", "ctrl+b inbox", "ctrl+j jobs"}, width)
	view.WriteString("\n" + dim.Render(footer))
	return view.String()
}

func (m Model) renderComposer(width int, focused bool) string {
	composerWidth := min(68, max(12, width-8))
	border := lipgloss.Color("238")
	cursor := lipgloss.Color("42")
	if m.theme == "contrast" {
		border, cursor = lipgloss.Color("250"), lipgloss.Color("46")
	} else if m.theme == "mono" {
		border, cursor = lipgloss.Color("245"), lipgloss.Color("255")
	}
	if focused {
		border = cursor
	}
	value := m.input
	if value == "" {
		value = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("Ask Gator to work on something…")
	}
	if focused {
		value += lipgloss.NewStyle().Foreground(cursor).Render("█")
	}
	return lipgloss.NewStyle().Width(composerWidth).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(border).Render(value)
}

func normalizeTheme(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "contrast":
		return "contrast"
	case "mono":
		return "mono"
	default:
		return "gator"
	}
}

func workStyles(theme string) (lipgloss.Style, lipgloss.Style, lipgloss.Style) {
	switch normalizeTheme(theme) {
	case "contrast":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true),
			lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
			lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("226"))
	case "mono":
		return lipgloss.NewStyle().Bold(true), lipgloss.NewStyle().Faint(true), lipgloss.NewStyle().Reverse(true)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true),
			lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
			lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("236"))
	}
}

func truncate(value string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	if maximum == 1 {
		return "…"
	}
	return string(runes[:maximum-1]) + "…"
}
