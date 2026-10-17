package connector

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/action"
)

func TestGmailSendPreparesPinnedPlainTextAndSendsOnce(t *testing.T) {
	requests := 0
	sentMessages := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodPost || request.URL.Path != "/gmail/v1/users/me/messages/send" {
			t.Errorf("send request = %s %s", request.Method, request.URL.Path)
		}
		var body struct {
			Raw string `json:"raw"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		message, err := base64.RawURLEncoding.DecodeString(body.Raw)
		if err != nil {
			t.Error(err)
		}
		for _, expected := range []string{"From: me@example.com\r\n", "To: venue@example.com\r\n", "Subject: Venue update", "Content-Type: text/plain; charset=UTF-8", "Content-Transfer-Encoding: base64"} {
			if !bytes.Contains(message, []byte(expected)) {
				t.Errorf("MIME message missing %q: %q", expected, message)
			}
		}
		sentMessages <- message
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"sent-1"}`))
	}))
	defer server.Close()
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Registry: registry, HTTPClient: server.Client()}
	input := json.RawMessage(`{"payload":{"from":"me@example.com","to":"venue@example.com","subject":"Venue update","body":"Can we use the new date?"}}`)
	prepared, err := runtime.PrepareAction(descriptor.ID, "gmail_send", input)
	if err != nil || requests != 0 {
		t.Fatalf("preparation = %#v, %v, requests=%d", prepared, err, requests)
	}
	if prepared.Proposal.Target != server.URL+"/gmail/v1/users/me/messages/send" || !strings.Contains(prepared.Proposal.Preview, "venue@example.com") || prepared.Proposal.Capability != action.Publish {
		t.Fatalf("proposal = %#v", prepared.Proposal)
	}
	messageID, err := gmailMessageID(prepared.Proposal.CorrelationID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(prepared.Proposal.Preview), []byte("Venue update")) {
		t.Fatal("approved Gmail preview was lost")
	}
	second, err := runtime.PrepareAction(descriptor.ID, "gmail_send", input)
	if err != nil || second.Proposal.CorrelationID == prepared.Proposal.CorrelationID {
		t.Fatalf("two sends were given the same reconciliation identity: %v", err)
	}
	tampered := prepared
	tampered.Proposal.CorrelationID = second.Proposal.CorrelationID
	if _, err := runtime.ExecutePreparedWithReceipt(context.Background(), action.Act, tampered); err == nil || requests != 0 {
		t.Fatalf("tampered reconciliation identity was accepted: %v, requests=%d", err, requests)
	}
	receipt, err := runtime.ExecutePreparedWithReceipt(context.Background(), action.Act, prepared)
	if err != nil || requests != 1 || receipt != "sent-1" {
		t.Fatalf("send = %v, receipt=%q, requests=%d", err, receipt, requests)
	}
	if sent := <-sentMessages; !bytes.Contains(sent, []byte("Message-ID: "+messageID+"\r\n")) {
		t.Fatal("Gmail message did not carry its reconciliation identity")
	}
}

func TestGmailSendRejectsHeaderInjectionAndExtraRecipients(t *testing.T) {
	for _, input := range []string{
		`{"from":"me@example.com","to":"x@example.com\r\nBcc: bad@example.com","subject":"Hi","body":"Text"}`,
		`{"from":"me@example.com","to":"x@example.com","subject":"Hi\r\nBcc: bad@example.com","body":"Text"}`,
		`{"from":"me@example.com","to":"x@example.com","subject":"Hi","body":"Text","cc":"bad@example.com"}`,
		`{"from":"me@example.com","to":"x@example.com, bad@example.com","subject":"Hi","body":"Text"}`,
	} {
		if _, err := prepareGmailMessage([]byte(input), strings.Repeat("0", 32)); err == nil {
			t.Fatalf("unsafe Gmail send input accepted: %s", input)
		}
	}
}

func TestGmailSendWithoutProviderReceiptIsUncertain(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{}`))
	}))
	defer server.Close()
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Registry: registry, HTTPClient: server.Client()}
	prepared, err := runtime.PrepareAction(descriptor.ID, "gmail_send", json.RawMessage(`{"payload":{"from":"me@example.com","to":"venue@example.com","subject":"Venue update","body":"Hello"}}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.ExecutePreparedWithReceipt(context.Background(), action.Act, prepared)
	var uncertain interface{ OutcomeUncertain() bool }
	if err == nil || !errors.As(err, &uncertain) || !uncertain.OutcomeUncertain() || requests != 1 {
		t.Fatalf("missing receipt outcome = %v, requests=%d", err, requests)
	}
}
