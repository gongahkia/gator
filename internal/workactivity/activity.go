// Package workactivity projects public runner events into bounded display data.
// It never includes tool arguments, tool results, provider payloads or reasoning.
package workactivity

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/agent"
)

const maxEntries = 160
const maxTextBytes = 64 * 1024

type Entry struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Text       string     `json:"text,omitempty"`
	Tool       string     `json:"tool,omitempty"`
	State      string     `json:"state"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type Trace struct {
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	State      string     `json:"state"`
	Entries    []Entry    `json:"entries"`
	Truncated  bool       `json:"truncated,omitempty"`
	textIndex  int
	textBytes  int
	tools      map[string]int
}

func New(at time.Time) *Trace {
	return &Trace{StartedAt: at, State: "running", Entries: []Entry{}, textIndex: -1, tools: map[string]int{}}
}

func (t *Trace) append(entry Entry) int {
	if len(t.Entries) >= maxEntries {
		t.Truncated = true
		return -1
	}
	entry.ID = fmt.Sprintf("activity-%d", len(t.Entries)+1)
	t.Entries = append(t.Entries, entry)
	return len(t.Entries) - 1
}

func (t *Trace) closeText(at time.Time) {
	if t.textIndex >= 0 {
		t.Entries[t.textIndex].State = "completed"
		t.Entries[t.textIndex].FinishedAt = &at
	}
	t.textIndex = -1
}

func (t *Trace) text(value string) string {
	remaining := maxTextBytes - t.textBytes
	if len(value) > remaining {
		t.Truncated = true
		value = value[:remaining]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}
	t.textBytes += len(value)
	return value
}

// Record must be called in runner delivery order by one owner. Child-agent
// payloads stay in their existing diagnostics; the chat shows parent narration.
func (t *Trace) Record(event agent.Event) {
	if t == nil || t.State != "running" || event.TaskID != "" {
		return
	}
	at := event.At
	if at.IsZero() {
		at = time.Now()
	}
	switch event.Kind {
	case agent.EventModelAttemptStarted, agent.EventTurnStarted:
		t.closeText(at)
	case agent.EventTextDelta, agent.EventText:
		if event.Text == "" {
			return
		}
		if t.textIndex < 0 {
			t.textIndex = t.append(Entry{Kind: "text", State: "running", StartedAt: at})
		}
		if t.textIndex >= 0 {
			t.Entries[t.textIndex].Text += t.text(event.Text)
		}
		if event.Kind == agent.EventText {
			t.closeText(at)
		}
	case agent.EventToolCalled, agent.EventToolFinished:
		if event.ToolCall == nil {
			return
		}
		t.closeText(at)
		key := fmt.Sprintf("%d/%s", event.Step, event.ToolCall.ID)
		index, found := t.tools[key]
		if !found {
			index = t.append(Entry{Kind: "tool", Tool: boundedName(event.ToolCall.Name), State: "running", StartedAt: at})
			if index < 0 {
				return
			}
			if t.tools == nil {
				t.tools = map[string]int{}
			}
			t.tools[key] = index
		}
		if event.Kind == agent.EventToolFinished {
			t.Entries[index].State = "completed"
			if event.ToolError != "" {
				t.Entries[index].State = "failed"
			}
			t.Entries[index].FinishedAt = &at
		}
	case agent.EventSteeringApplied, agent.EventContextCompacted, agent.EventCompletionBlocked:
		t.closeText(at)
		label := "Applied your guidance"
		if event.Kind == agent.EventContextCompacted {
			label = "Condensed earlier context"
		}
		if event.Kind == agent.EventCompletionBlocked {
			label = "Result needs more work"
		}
		t.append(Entry{Kind: "status", Text: label, State: "completed", StartedAt: at, FinishedAt: &at})
	}
}

func boundedName(name string) string {
	if len(name) > 160 {
		name = name[:160]
		for !utf8.ValidString(name) {
			name = name[:len(name)-1]
		}
	}
	return name
}

// Finish removes the final answer's duplicate streamed text. The retained
// response stays in the transcript outside the collapsed work disclosure.
func (t *Trace) Finish(at time.Time, state, finalText string) {
	if t == nil || t.State != "running" {
		return
	}
	t.closeText(at)
	if n := len(t.Entries); n > 0 && t.Entries[n-1].Kind == "text" && strings.TrimSpace(finalText) != "" && strings.TrimSpace(t.Entries[n-1].Text) == strings.TrimSpace(finalText) {
		t.Entries = t.Entries[:n-1]
	}
	for i := range t.Entries {
		if t.Entries[i].State == "running" {
			t.Entries[i].State = "stopped"
			t.Entries[i].FinishedAt = &at
		}
	}
	t.State, t.FinishedAt = state, &at
}

func (t *Trace) Clone() *Trace {
	if t == nil {
		return nil
	}
	copy := *t
	copy.Entries = append([]Entry{}, t.Entries...)
	copy.tools = nil
	return &copy
}
