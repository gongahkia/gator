package workapp

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/uisnapshot"
)

type recallStatusDone struct {
	Status RecallStatus
	Err    error
}
type recallProgress string
type recallInstallDone struct{ Err error }

func (m *Model) recallStatusCommand() tea.Cmd {
	if m.config.RecallStatus == nil {
		return nil
	}
	if m.recallChecking > 0 {
		return nil
	}
	m.recallChecking++
	read := m.config.RecallStatus
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		status, err := read(ctx)
		return recallStatusDone{status, err}
	}
}

func (m *Model) openRecall() tea.Cmd {
	m.section, m.home, m.launcher, m.sectionNotice = "memory", false, false, "Checking recall readiness…"
	return m.recallStatusCommand()
}

func (m Model) updateRecallKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.recallChecking > 0 || m.config.RecallStatus != nil && m.recall.State == "" {
		switch key.String() {
		case "a", "h", "i", "l", "o", "g", "enter":
			m.sectionNotice = "Memory settings are not ready. Wait for loading to finish, or press R to retry."
			return m, nil
		}
	}
	if m.recallBusy {
		if key.String() == "esc" && m.recallCancel != nil {
			m.recallCancel()
			m.sectionNotice = "Cancelling model download…"
		}
		return m, nil
	}
	if m.recallConfirm != "" {
		switch key.String() {
		case "esc":
			m.recallConfirm = ""
		case "enter":
			choice := m.recallConfirm
			m.recallConfirm = ""
			if choice == "install" {
				return m.startRecallInstall()
			}
			if m.config.ConfigureRecall == nil {
				m.sectionNotice = "Memory configuration unavailable."
				return m, nil
			}
			if err := m.config.ConfigureRecall(choice, choice != "ollama"); err != nil {
				m.sectionNotice = err.Error()
				return m, nil
			}
			m.sectionNotice = "Saved recall selection. Chat processing remains controlled by /model."
			command := m.recallStatusCommand()
			return m, command
		}
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.section = ""
	case "r":
		command := m.recallStatusCommand()
		return m, command
	case "a":
		if m.config.ConfigureChatLearning == nil {
			m.sectionNotice = "Chat learning settings are unavailable."
			return m, nil
		}
		enabled := !m.recall.LearnFromChats
		if err := m.config.ConfigureChatLearning(enabled); err != nil {
			m.sectionNotice = err.Error()
			return m, nil
		}
		m.recall.LearnFromChats = enabled
		m.sectionNotice = "Chat learning updated. Existing saved memories remain available; explicit remember and forget requests still work."
		command := m.recallStatusCommand()
		return m, command
	case "h":
		if m.config.ConfigureChatHistory == nil {
			m.sectionNotice = "Chat history settings are unavailable."
			return m, nil
		}
		enabled := !m.recall.ReferToChats
		if err := m.config.ConfigureChatHistory(enabled); err != nil {
			m.sectionNotice = err.Error()
			return m, nil
		}
		m.recall.ReferToChats = enabled
		m.sectionNotice = "Earlier-chat retrieval updated for new requests. Current chats and saved memories are kept. Requests already running retain their supplied context."
		command := m.recallStatusCommand()
		return m, command
	case "i":
		m.recallConfirm = "install"
	case "l":
		m.recallConfirm = "ollama"
	case "o":
		m.recallConfirm = "openai"
	case "g":
		m.recallConfirm = "gemini"
	case "c":
		return m.openModels()
	case "k":
		return m.openContext("", true)
	}
	return m, nil
}

func (m Model) startRecallInstall() (tea.Model, tea.Cmd) {
	if m.config.InstallRecall == nil {
		m.sectionNotice = "Model installation unavailable."
		return m, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan tea.Msg, 16)
	m.recallBusy, m.recallCancel, m.recallEvents = true, cancel, events
	m.sectionNotice = "Downloading enhanced local recall… Esc cancels."
	install := m.config.InstallRecall
	go func() {
		err := install(ctx, func(status string) {
			if len(events) >= cap(events)-1 {
				return
			}
			select {
			case events <- recallProgress(status):
			default:
			}
		})
		events <- recallInstallDone{err}
	}()
	return m, waitWorkEvent(events)
}

func (m Model) recallPanel() uisnapshot.Panel {
	p := uisnapshot.Panel{Kind: "memory", Title: "Memory & privacy", Notice: m.sectionNotice, Subtitle: "Choose how Gator finds saved knowledge. You control what is stored and supplied."}
	if m.recallConfirm != "" {
		p.Kind, p.Title = "confirmation", "Review memory setup"
		lines := []string{}
		if m.recallConfirm == "install" {
			lines = []string{"Download embeddinggemma:latest from Ollama's model library (about 622 MB).", "Ollama must be installed and running on this computer.", "Enhanced recall runs locally. Esc can cancel the download; basic word matching stays available.", "This does not change the selected chat model or where that model processes supplied context."}
		} else if m.recallConfirm == "ollama" {
			lines = []string{"Use local Ollama for enhanced retrieval when available, with basic word matching as fallback.", "Saved knowledge remains in local Gator storage. Selected context still goes to your configured chat model."}
		} else {
			lines = []string{fmt.Sprintf("Use %s for enhanced retrieval.", m.recallConfirm), "This uploads eligible saved memory and note text, plus search queries, to that embedding provider.", "A separate API key is required. Local Gator storage remains on this computer.", "Chat processing uses the model chosen in /model and may use a different provider."}
		}
		p.Blocks = []uisnapshot.Block{{Title: "Data and processing", Lines: lines}}
		label := map[string]string{"install": "download local recall model", "ollama": "use local recall", "openai": "use OpenAI retrieval", "gemini": "use Gemini retrieval"}[m.recallConfirm]
		p.Hints = []uisnapshot.Hint{{Shortcut: "Enter", Action: label}, {Shortcut: "Esc", Action: "cancel"}}
		return p
	}
	p.Blocks = []uisnapshot.Block{
		{Title: "Recall readiness", Lines: []string{valueOrNone(m.recall.State), m.recall.Detail}},
		{Title: "Storage", Collapsible: true, Lines: []string{valueOrDefault(m.recall.Storage, "Memories, notes, transcripts, and Work records are stored in your private Gator state directory on this computer.")}},
		{Title: "Retrieval", Collapsible: true, Lines: []string{valueOrDefault(m.recall.Retrieval, "Basic word matching works without a model download. Enhanced recall uses local embeddings or your separately selected cloud provider.")}},
		{Title: "Chat processing", Collapsible: true, Lines: []string{valueOrDefault(m.recall.Chat, "Selected saved context and task sources are supplied to the model chosen in /model. A cloud chat model processes that context remotely even when storage and retrieval are local.")}},
		{Title: "Across chats", Lines: []string{"Personal memories are shared by chats in this local profile. New chat starts a fresh working context; it keeps your saved memory.", "Useful stable facts and preferences can be saved from your own statements, with their source chat. One-off task details and file/tool instructions are not personal memories.", "Turning chat learning off stops automatic saves. Existing memories remain available; explicit remember/forget requests still work. Earlier-chat retrieval separately controls whether new requests can search other conversations. Turning it off keeps this conversation and saved memories available. Devices do not sync automatically."}},
		{Title: "Your controls", Collapsible: true, Lines: []string{"Browse saved knowledge to correct, forget, or omit it from the next run.", "Forgetting excludes tracked source chats from agent-driven history retrieval. It does not delete old messages or files, or revoke context already supplied to a running request. Data & backups manages retained data; switching recall off is not deletion.", "Feedback ratings are retained for review. They do not automatically retrain your model.", "Local recall can fall back to basic word matching; each result's Context and sources shows the observed route."}},
	}
	fallback := "Readiness not available"
	if m.recallBusy {
		fallback = "Checking readiness…"
	}
	learning := "Off"
	if m.recall.LearnFromChats {
		learning = "On"
	}
	history := "Off"
	if m.recall.ReferToChats {
		history = "On"
	}
	if m.recallChecking > 0 && m.recall.State == "" {
		learning, history = "Checking…", "Checking…"
	}
	p.Summary = []uisnapshot.Field{{Label: "Recall readiness", Value: valueOrDefault(m.recall.State, fallback)}, {Label: "Learn from chats", Value: learning}, {Label: "Refer to earlier chats", Value: history}}
	p.Blocks[0].Lines = []string{m.recall.Detail}
	if len(m.recall.PersonalSummary) != 0 {
		p.Blocks = append(p.Blocks, uisnapshot.Block{Title: "What Gator knows about you", Collapsible: true, Lines: append(append([]string{}, m.recall.PersonalSummary...), "Press K to inspect, correct or forget saved knowledge; R rebuilds this summary.")})
	}
	if m.recallBusy {
		p.Hints = []uisnapshot.Hint{{Shortcut: "Esc", Action: "cancel download"}}
	} else {
		p.Hints = []uisnapshot.Hint{{Shortcut: "A", Action: map[bool]string{true: "turn chat learning off", false: "turn chat learning on"}[m.recall.LearnFromChats]}, {Shortcut: "H", Action: map[bool]string{true: "turn earlier-chat retrieval off", false: "turn earlier-chat retrieval on"}[m.recall.ReferToChats]}, {Shortcut: "I", Action: "install local recall", Group: "more"}, {Shortcut: "L", Action: "use local"}, {Shortcut: "O", Action: "use OpenAI", Group: "more"}, {Shortcut: "G", Action: "use Gemini", Group: "more"}, {Shortcut: "K", Action: "saved knowledge"}, {Shortcut: "C", Action: "chat model", Group: "more"}, {Shortcut: "R", Action: "refresh"}, {Shortcut: "Esc", Action: "back"}}
	}
	return p
}
func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
