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
