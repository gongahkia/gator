package rpc

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestClientSendsCurrentVersionAndReceivesMessage(t *testing.T) {
	var outgoing bytes.Buffer
	incoming := bytes.NewBufferString(`{"version":1,"type":"response","id":"request-1","result":{"ok":true},"at":"2026-08-17T00:00:00Z"}` + "\n")
	client, err := NewClient(incoming, &outgoing)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if err := client.Send(Request{ID: "request-1", Method: MethodStatus}); err != nil {
		t.Fatalf("send: %v", err)
	}
	var sent Request
	if err := json.Unmarshal(outgoing.Bytes(), &sent); err != nil {
		t.Fatalf("decode sent request: %v", err)
	}
	if sent.Version != Version || sent.Method != MethodStatus {
		t.Fatalf("sent request = %#v", sent)
	}
	message, err := client.Receive()
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if message.Type != "response" || message.ID != "request-1" {
		t.Fatalf("message = %#v", message)
	}
}
