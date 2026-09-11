package worktui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/rattles"
	"github.com/gongahkia/gator/internal/workrun"
)

func waitWorkEvent(events <-chan tea.Msg) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg { return <-events }
}

func (m Model) nextLoadingTick() tea.Cmd {
	run := m.loadingRun
	return tea.Tick(rattles.BrailleDots.Interval, func(time.Time) tea.Msg {
		return loadingTickMsg{run: run}
	})
}

func (m Model) startWork(source, conversation, prompt string, options RunOptions) (tea.Model, tea.Cmd) {
	if !m.config.Live {
		return m, func() tea.Msg { return runDone(m.config.Run(source, conversation, prompt, options)) }
	}
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan tea.Msg, 128)
	m.cancel = cancel
	m.tasks = map[string]string{}
	m.live = events
	m.loadingFrame = 0
	m.loadingRun++
	send := func(value tea.Msg) {
		select {
		case events <- value:
		case <-ctx.Done():
		}
	}
	options.Context = ctx
	options.OnEvent = func(event agent.Event) { send(event) }
	options.OnOperation = func(operation *workrun.Operation) {
		send(operation)
		go func() {
			for interaction := range operation.Interactions {
				send(interaction)
			}
		}()
	}
	run := m.config.Run
	return m, tea.Batch(func() tea.Msg {
		result := run(source, conversation, prompt, options)
		events <- runDone(result)
		return nil
	}, waitWorkEvent(events), m.nextLoadingTick())
}
