package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/worktui"
)

type modelViewProbe chan string
type probedWorkModel struct{ tea.Model }

func (m probedWorkModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if probe, ok := message.(modelViewProbe); ok {
		probe <- m.View()
		return m, nil
	}
	next, command := m.Model.Update(message)
	m.Model = next
	return m, command
}

func TestWorkTUIManagesLocalModelsAndUsesSelectionThroughService(t *testing.T) {
	useGenerousLocalModelHost(t)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	state, source := t.TempDir(), t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	var installed atomic.Bool
	var pulls, deletes, calls atomic.Int32
	cancelled := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/version":
			_, _ = io.WriteString(w, `{"version":"fixture"}`)
		case "/api/tags":
			models := []map[string]any{}
			if installed.Load() {
				models = append(models, map[string]any{"name": "qwen2.5-coder:0.5b", "size": 398000000})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"models": models})
		case "/api/pull":
			var body struct{ Model string }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "qwen2.5-coder:0.5b" {
				t.Error("wrong model download", body, err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, "{\"status\":\"pulling layers\",\"completed\":5,\"total\":10}\n")
			w.(http.Flusher).Flush()
			if pulls.Add(1) == 1 {
				<-r.Context().Done()
				cancelled <- struct{}{}
				return
			}
			installed.Store(true)
			_, _ = io.WriteString(w, "{\"status\":\"success\"}\n")
		case "/api/delete":
			if r.Method != http.MethodDelete {
				t.Error("wrong delete method", r.Method)
			}
			deletes.Add(1)
			installed.Store(false)
		case "/v1/chat/completions":
			var body struct {
				Model  string
				Stream bool
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "qwen2.5-coder:0.5b" {
				t.Error("Work did not use the TUI model selection", body, err)
			}
			message := map[string]any{"content": "Local Work complete"}
			if calls.Add(1) == 1 {
				message = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "write-report", "type": "function", "function": map[string]any{"name": "write_artifact", "arguments": `{"path":"report.md","content":"Local UI report"}`}}}}
			}
			if body.Stream {
				w.Header().Set("Content-Type", "text/event-stream")
				data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": message}}})
				_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": message}}})
			}
		default:
			t.Error("unexpected local request", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	manager := &localModelManager{store: store, runtimeURL: server.URL, host: generousLocalModelHost}
	defer manager.Close()
	application := worktui.New(worktui.Config{
		CurrentFolder: source, FirstRun: true, Live: true,
		Models: workModelPanel(store, state, manager),
		SelectedModel: func() (string, string, error) {
			s, err := store.Load()
			return s.Defaults.Provider, s.Defaults.Model, err
		},
		ModelStatus: func() (worktui.ModelStatus, error) {
			return currentWorkModelStatus(store, state)
		},
		Run: func(source, conversation, prompt string, options worktui.RunOptions) worktui.RunResult {
			return runInteractiveWork(source, conversation, prompt, state, options)
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	program := tea.NewProgram(probedWorkModel{application}, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		final, err := program.Run()
		if err != nil && ctx.Err() == nil {
			t.Error(err)
		}
		if m, ok := final.(probedWorkModel); ok {
			m.Model.(worktui.Model).Close()
		}
	}()
	t.Cleanup(func() { program.Quit(); <-stopped })
	program.Send(tea.WindowSizeMsg{Width: 120, Height: 60})
	key := func(value string) {
		switch value {
		case "enter":
			program.Send(tea.KeyMsg{Type: tea.KeyEnter})
		case "tab":
			program.Send(tea.KeyMsg{Type: tea.KeyTab})
		case "esc":
			program.Send(tea.KeyMsg{Type: tea.KeyEsc})
		default:
			program.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)})
		}
	}
	waitView := func(expected string) {
		t.Helper()
		var view string
		deadline := time.After(5 * time.Second)
		for {
			probe := make(modelViewProbe, 1)
			program.Send(probe)
			select {
			case view = <-probe:
				if strings.Contains(view, expected) {
					return
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			select {
			case <-deadline:
				t.Fatalf("missing %q in Work TUI:\n%s", expected, view)
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	key("write a short report")
	key("enter")
	waitView("amazon-bedrock")
	key("tab")
	waitView("Qwen2.5-Coder 0.5B")
	key("p")
	waitView("Confirm download")
	if pulls.Load() != 0 {
		t.Fatal("download started before confirmation")
	}
	key("y")
	waitView("pulling layers")
	key("esc")
	select {
	case <-cancelled:
	case <-ctx.Done():
		t.Fatal("download was not cancelled")
	}
	key("p")
	key("y")
	waitView("download finished")
	key("u")
	waitView("Local model selected")
	key("esc")
	waitView("write a short report")
	key("enter")
	waitView("Local Work complete")
	waitView("Artifacts:")
	if calls.Load() != 2 {
		t.Fatalf("Work model calls = %d", calls.Load())
	}
	key("/code")
	key("enter")
	waitView("model access: local model configured")
	key("/model")
	key("enter")
	waitView("Qwen2.5-Coder 0.5B")
	key("x")
	waitView("Confirm removal")
	if deletes.Load() != 0 {
		t.Fatal("model deleted before confirmation")
	}
	key("n")
	if deletes.Load() != 0 {
		t.Fatal("declined model deletion executed")
	}
	key("x")
	key("y")
	waitView("Local model removed")
	key("esc")
	waitView("Choose a model with /model")
	settings, err := store.Load()
	if err != nil || installed.Load() || deletes.Load() != 1 || settings.Defaults.Provider != "" {
		t.Fatalf("deleted model remained selected: %+v %v", settings.Defaults, err)
	}
}
