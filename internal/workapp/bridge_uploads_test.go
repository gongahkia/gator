package workapp

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestBridgeUploadsBindQueueInputsAndAcknowledgeRejections(t *testing.T) {
	var received [][]string
	m := New(Config{CurrentFolder: "/project", Run: func(_, _, _ string, options RunOptions) RunResult {
		received = append(received, options.Uploads)
		return RunResult{FinalText: "Done"}
	}, ValidateUploads: func(_ string, _, _ []string) error { return nil }})
	m.primary = true
	request := BridgeUploadSubmission{RequestID: "first", Context: "workspace:/project", Text: "First message", Files: []string{strings.Repeat("a", 32) + "/first.txt"}}
	wrong := request
	wrong.Context = "conversation:other"
	next, command := m.SubmitBridgeUploads(wrong)
	m = next.(Model)
	if command != nil || m.bridgeUploadResult.Error == "" || m.running {
		t.Fatal("accepted stale conversation")
	}
	next, command = m.SubmitBridgeUploads(request)
	m = next.(Model)
	if command == nil || m.bridgeUploadResult.Error != "" || len(m.options.Uploads) != 0 || !reflect.DeepEqual(m.BridgeState().Messages[0].Inputs, []string{"first.txt"}) {
		t.Fatal("upload was not bound to the first message")
	}
	queued := request
	queued.RequestID = "second"
	queued.Text = "Follow-up"
	queued.Files = []string{strings.Repeat("b", 32) + "/second.txt"}
	next, _ = m.SubmitBridgeUploads(queued)
	m = next.(Model)
	queued.Files[0] = "mutated.txt"
	if !reflect.DeepEqual(m.BridgeState().Queue[0].Inputs, []string{"second.txt"}) || len(m.options.Uploads) != 0 {
		t.Fatal("queued inputs changed or leaked into the next prompt")
	}
	next, following := m.Update(command())
	m = next.(Model)
	if following == nil {
		t.Fatal("queue did not start")
	}
	following()
	if len(received) != 2 || received[0][0] == received[1][0] {
		t.Fatalf("received inputs = %#v", received)
	}
	m.running = false
	m.config.ValidateUploads = func(_ string, _, _ []string) error { return errors.New("invalid PDF") }
	next, command = m.SubmitBridgeUploads(request)
	m = next.(Model)
	if command != nil || !strings.Contains(m.bridgeUploadResult.Error, "invalid PDF") || len(m.options.Uploads) != 0 {
		t.Fatal("invalid attachment consumed a message")
	}
}

func TestBridgeUploadsRefreshModelReadinessBeforeAdmission(t *testing.T) {
	for _, scenario := range []string{"credential-removed", "model-removed", "selection-changed"} {
		t.Run(scenario, func(t *testing.T) {
			selected, unavailable, removed := "first", "", false
			var captured RunOptions
			config, store := requestConfig(t, func(_, _, _ string, options RunOptions) RunResult {
				captured = options
				return RunResult{FinalText: "done"}
			})
			config.SelectedModel = func() (string, string, error) { return "fixture", selected, nil }
			config.ComposerModels = func() ([]ComposerModel, error) {
				if removed {
					return nil, nil
				}
				return []ComposerModel{
					{Provider: "fixture", Model: "first", Unavailable: unavailable},
					{Provider: "fixture", Model: "second"},
				}, nil
			}
			m := New(config)
			before := m.BridgeState()
			switch scenario {
			case "credential-removed":
				unavailable = "Set up this provider in Manage models."
			case "model-removed":
				removed = true
			case "selection-changed":
				selected = "second"
			}
			request := BridgeUploadSubmission{RequestID: "kept-message", Context: "conversation:" + m.conversation, Text: "Keep this draft"}
			next, command := m.SubmitBridgeUploads(request)
			m = next.(Model)
			if command != nil || m.bridgeUploadResult.Error == "" || m.running || len(m.messages) != len(before.Messages) || len(m.queue) != 0 {
				t.Fatal("unavailable or changed selection consumed a message", m.bridgeUploadResult)
			}
			journal, err := store.LoadRequests(m.conversation)
			if err != nil || len(journal.Requests) != 0 {
				t.Fatal("rejected message was admitted", err)
			}
			if scenario != "selection-changed" && m.composerSettings().ModelIssue == "" {
				t.Fatal("the picker did not receive the refreshed setup issue")
			}
			// Restoring access permits the same unconsumed draft to be submitted
			// explicitly, using the model now shown by the picker.
			unavailable, removed = "", false
			next, command = m.SubmitBridgeUploads(request)
			m = next.(Model)
			if command == nil || m.bridgeUploadResult.Error != "" || m.composerSettings().ModelIssue != "" {
				t.Fatal("setup recovery did not permit submission", m.bridgeUploadResult)
			}
			command()
			if captured.ModelProvider != "fixture" || captured.ModelName != selected {
				t.Fatal("request did not use the displayed selection", captured.ModelProvider, captured.ModelName)
			}
		})
	}
}
