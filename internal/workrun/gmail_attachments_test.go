package workrun

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/connector"
)

func TestWorkGmailAttachmentsUseSelectedBytesAndExactReview(t *testing.T) {
	for _, c := range []struct {
		name        string
		mode        action.Mode
		disposition action.Disposition
		decision    action.Decision
		want        action.Status
		calls       int
	}{
		{"approved", action.Act, action.Approve, action.Allow, action.Executed, 1},
		{"denied", action.Act, action.Approve, action.Deny, action.Denied, 0},
		{"draft", action.Draft, action.Propose, action.Deny, action.Pending, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			source := t.TempDir()
			original := []byte("selected original attachment\x00\xff")
			if err := os.WriteFile(filepath.Join(source, "original.bin"), original, 0600); err != nil {
				t.Fatal(err)
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var request struct{ Raw string }
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				data, err := base64.RawURLEncoding.DecodeString(request.Raw)
				if err != nil {
					t.Error(err)
					return
				}
				message, err := mail.ReadMessage(bytes.NewReader(data))
				if err != nil {
					t.Error(err)
					return
				}
				_, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
				if err != nil {
					t.Error(err)
					return
				}
				parts := multipart.NewReader(message.Body, params["boundary"])
				var bodies [][]byte
				for {
					part, err := parts.NextPart()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Error(err)
						break
					}
					body, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, part))
					if err != nil {
						t.Error(err)
					}
					bodies = append(bodies, body)
				}
				if len(bodies) != 3 || string(bodies[0]) != "Ready for review." || !bytes.Equal(bodies[1], original) || string(bodies[2]) != "Generated report" {
					t.Error("Work attachments did not carry exact source/output bytes")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"sent-1"}`)
			}))
			defer server.Close()
			d := connector.Descriptor{Version: 1, ID: "google", Name: "Google", Kind: connector.KindGoogle, Resource: server.URL, Authentication: connector.AuthNone}
			registry, err := connector.NewRegistry([]connector.Descriptor{d})
			if err != nil {
				t.Fatal(err)
			}
			model := &scriptedModel{turns: []agent.Turn{
				{ToolCalls: []agent.ToolCall{{ID: "write", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"Generated report"}`)}}},
				{ToolCalls: []agent.ToolCall{{ID: "send", Name: "connector_google_gmail_send", Arguments: json.RawMessage(`{"payload":{"from":"me@example.com","to":["one@example.com","two@example.com"],"subject":"Report","body":"Ready for review.","attachments":[{"source":"source/original.bin"},{"source":"output/report.md"}]}}`)}}},
				{Text: "Request handled; review the recorded outcome."},
			}}
			contract := artifact.DefaultContract("report.md")
			contract.ExternalActions = c.disposition
			approvals := 0
			out, err := (Executor{Model: model, StateDir: t.TempDir(), Connectors: connector.Runtime{Registry: registry, HTTPClient: server.Client()}}).Execute(context.Background(), Request{
				SourcePath: source, Objective: "Send the selected original and generated report to these recipients", Mode: c.mode, Contract: contract, ConnectorIDs: []string{"google"}, DisableBrowser: true, DisableDelegation: true, MaxSteps: 5,
				ApproveAction: func(_ context.Context, p action.Proposal) (action.Decision, error) {
					approvals++
					if calls != 0 || !strings.Contains(p.Preview, `"sha256":`) || strings.Contains(p.Preview, "selected original attachment") {
						t.Fatal("attachment dispatched or bytes exposed before approval")
					}
					return c.decision, nil
				},
			})
			if err != nil || calls != c.calls || len(out.Manifest.Actions) != 1 || out.Manifest.Actions[0].Status != c.want {
				t.Fatalf("Work send outcome: calls=%d actions=%+v err=%v", calls, out.Manifest.Actions, err)
			}
			if c.mode == action.Draft && approvals != 0 || c.mode == action.Act && approvals != 1 {
				t.Fatal("wrong approval lifecycle", approvals)
			}
			data, _ := json.Marshal(out.Manifest)
			if bytes.Contains(data, []byte("selected original attachment")) || bytes.Contains(data, []byte(base64.StdEncoding.EncodeToString(original))) {
				t.Fatal("original attachment bytes leaked to manifest")
			}
		})
	}
}
