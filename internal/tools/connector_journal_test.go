package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/actionlog"
	"github.com/gongahkia/gator/internal/connector"
)

type failingActionAttempt struct{ closed bool }

func (f *failingActionAttempt) Finish(action.Record) error {
	return errors.New("receipt disk unavailable")
}
func (f *failingActionAttempt) Close() error { f.closed = true; return nil }

func TestConnectorJournalGatesDispatchAndDoesNotTurnPersistenceFailureIntoRetry(t *testing.T) {
	for _, stage := range []string{"begin-error", "begin-nil", "finish-error", "denied", "draft"} {
		t.Run(stage, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(http.StatusNoContent) }))
			defer server.Close()
			descriptor := connector.Descriptor{Version: connector.DescriptorVersion, ID: "hook", Name: "Hook", Kind: connector.KindHTTPWebhook, Resource: server.URL, Authentication: connector.AuthNone}
			registry, err := connector.NewRegistry([]connector.Descriptor{descriptor})
			if err != nil {
				t.Fatal(err)
			}
			began, approved := 0, false
			var record action.Record
			fake := new(failingActionAttempt)
			policy := ConnectorPolicy{Mode: action.Act, ExternalActions: action.Approve, Approve: func(context.Context, action.Proposal) (action.Decision, error) {
				approved = true
				if stage == "denied" {
					return action.Deny, nil
				}
				return action.Allow, nil
			}, OnAction: func(r action.Record) { record = r }, BeginAction: func(action.Proposal) (ActionAttempt, error) {
				began++
				if !approved {
					t.Fatal("journal opened before approval")
				}
				if stage == "begin-error" {
					return nil, errors.New("disk unavailable")
				}
				if stage == "begin-nil" {
					return nil, nil
				}
				return fake, nil
			}}
			if stage == "draft" {
				policy.Mode = action.Draft
				policy.ExternalActions = action.Propose
			}
			surface, err := ConnectorTools(connector.Runtime{Registry: registry, HTTPClient: server.Client()}, []string{"hook"}, policy)
			if err != nil {
				t.Fatal(err)
			}
			result, err := surface[0].Execute(context.Background(), json.RawMessage(`{"payload":{"release":"v1"}}`))
			if err != nil {
				t.Fatal(err)
			}
			switch stage {
			case "finish-error":
				if requests.Load() != 1 || record.Status != action.Unknown || !fake.closed || !strings.Contains(result.Content, "durable outcome") {
					t.Fatal("uncertain retention claimed failure/success or retried", record, requests.Load(), result)
				}
			case "denied":
				if requests.Load() != 0 || began != 0 || record.Status != action.Denied {
					t.Fatal("denied request dispatched or logged", record)
				}
			case "draft":
				if requests.Load() != 0 || began != 0 || approved || record.Status != action.Pending {
					t.Fatal("draft request dispatched or logged", record)
				}
			default:
				if requests.Load() != 0 || began != 1 || record.Status != action.Failed {
					t.Fatal("dispatch occurred without durable admission", record, requests.Load())
				}
			}
			if err := record.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConnectorDispatchHasDurableJournalBeforeProviderAndExactReceiptAfter(t *testing.T) {
	store, err := actionlog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		entries, err := store.List("run-one")
		if err != nil || len(entries) != 1 || !entries[0].Live || entries[0].Action.Status != action.Unknown || entries[0].ConversationID != "chat-one" {
			t.Errorf("provider called without owned flushed receipt: %v %v", entries, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"gmail-one"}`))
	}))
	defer server.Close()
	descriptor := connector.Descriptor{Version: connector.DescriptorVersion, ID: "google", Name: "Google", Kind: connector.KindGoogle, Resource: server.URL, Authentication: connector.AuthNone}
	registry, err := connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	surface, err := ConnectorTools(connector.Runtime{Registry: registry, HTTPClient: server.Client()}, []string{"google"}, ConnectorPolicy{Mode: action.Act, ExternalActions: action.Approve, Approve: func(context.Context, action.Proposal) (action.Decision, error) { return action.Allow, nil }, BeginAction: func(p action.Proposal) (ActionAttempt, error) {
		return store.Begin("chat-one", "run-one", p, time.Now())
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range surface {
		if tool.Definition().Name == "connector_google_gmail_send" {
			if _, err := tool.Execute(context.Background(), json.RawMessage(`{"payload":{"from":"me@example.com","to":"venue@example.com","subject":"Hello","body":"Hi"}}`)); err != nil {
				t.Fatal(err)
			}
		}
	}
	entries, err := store.List("run-one")
	if err != nil || calls.Load() != 1 || len(entries) != 1 || entries[0].Live || entries[0].Phase != "settled" || entries[0].Action.Status != action.Executed || entries[0].Action.ReceiptID != "gmail-one" || entries[0].Action.Proposal.CorrelationID == "" {
		t.Fatal("provider outcome was not retained exactly", entries, err, calls.Load())
	}
}
