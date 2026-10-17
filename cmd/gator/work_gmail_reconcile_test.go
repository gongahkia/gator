package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestWorkGmailReconcileChecksSentWithoutRetryingOrChangingSealedAction(t *testing.T) {
	for _, test := range []struct {
		name, list, detail, status string
		get                        bool
	}{
		{"found", `{"messages":[{"id":"sent-1"}]}`, "matching", "found", true},
		{"missing", `{}`, "", "unconfirmed", false},
		{"ambiguous", `{"messages":[{"id":"sent-1"},{"id":"sent-2"}]}`, "", "ambiguous", false},
		{"wrong-header", `{"messages":[{"id":"sent-1"}]}`, "wrong", "unconfirmed", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			stateDir := t.TempDir()
			t.Setenv("GATOR_STATE_DIR", stateDir)
			t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
			var requests []string
			correlationID := ""
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requests = append(requests, request.Method+" "+request.URL.Path)
				if request.Method != http.MethodGet {
					t.Errorf("reconciliation attempted mutation %s", request.Method)
				}
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case "/gmail/v1/users/me/messages":
					want := "in:sent rfc822msgid:<gator-" + correlationID + "@gator.invalid>"
					if request.URL.Query().Get("q") != want || request.URL.Query().Get("maxResults") != "10" {
						t.Errorf("Gmail search = %s", request.URL.String())
					}
					_, _ = writer.Write([]byte(test.list))
				case "/gmail/v1/users/me/messages/sent-1":
					header := "<gator-" + correlationID + "@gator.invalid>"
					if test.detail == "wrong" {
						header = "<other@example.com>"
					}
					if request.URL.Query().Get("format") != "full" {
						t.Errorf("Gmail detail = %s", request.URL.String())
					}
					_, _ = writer.Write([]byte(`{"id":"sent-1","labelIds":["SENT"],"payload":{"headers":[{"name":"Message-ID","value":"` + header + `"}]}}`))
				default:
					t.Errorf("unexpected Gmail request %s", request.URL.Path)
					writer.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			settings := config.Default()
			settings.Connectors = []connector.Descriptor{{Version: connector.DescriptorVersion, ID: "google", Name: "Google", Kind: connector.KindGoogle, Resource: server.URL, Authentication: connector.AuthNone}}
			configStore, err := config.DefaultStore()
			if err != nil {
				t.Fatal(err)
			}
			if err := configStore.Save(settings); err != nil {
				t.Fatal(err)
			}
			registry, err := connector.NewRegistry(settings.Connectors)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := (connector.Runtime{Registry: registry}).PrepareAction("google", "gmail_send", json.RawMessage(`{"payload":{"from":"me@example.com","to":"venue@example.com","subject":"Venue update","body":"Hello"}}`))
			if err != nil {
				t.Fatal(err)
			}
			correlationID = prepared.Proposal.CorrelationID
			work, err := workspace.CreateWork(t.TempDir(), stateDir, "reconcile-send", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			contract := artifact.DefaultContract("report.md")
			contract.ExternalActions = action.Approve
			if err := artifact.WriteText(work.Output, contract, "report.md", "Ready.\n"); err != nil {
				t.Fatal(err)
			}
			manifest, err := artifact.Seal(work.Output, contract, artifact.SealOptions{
				RunID: work.ID, Objective: "Send venue reply", Source: work.Source,
				Actions:   []action.Record{{Proposal: prepared.Proposal, Status: action.Unknown, Error: "outcome unknown"}},
				StartedAt: work.CreatedAt, FinishedAt: work.CreatedAt,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := artifact.WriteManifest(work.ManifestPath, manifest); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if err := reconcileGmailSend(context.Background(), stateDir, work.ID, correlationID, &output); err != nil {
				t.Fatal(err)
			}
			if len(requests) != 1+boolInt(test.get) || !strings.Contains(requests[0], "GET /gmail/v1/users/me/messages") {
				t.Fatalf("requests = %#v", requests)
			}
			if !strings.Contains(output.String(), "No send was retried") && !strings.Contains(output.String(), "no send was retried") {
				t.Fatalf("reconciliation output = %q", output.String())
			}
			data, err := os.ReadFile(filepath.Join(stateDir, "gator", "reconciliations", work.ID, correlationID+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var check gmailReconciliation
			if err := json.Unmarshal(data, &check); err != nil || check.Status != test.status || check.ManifestSHA256 == "" {
				t.Fatalf("reconciliation = %#v, %v", check, err)
			}
			bundle, err := artifact.OpenBundle(work.Path)
			if err != nil || artifact.VerifyBundle(bundle) != nil || bundle.Manifest.Actions[0].Status != action.Unknown {
				t.Fatalf("sealed action changed: %#v, %v", bundle.Manifest.Actions, err)
			}
			if test.name == "found" {
				var review bytes.Buffer
				if err := reviewCommand([]string{work.ID}, &review); err != nil || !strings.Contains(review.String(), "gator work reconcile "+work.ID+" "+correlationID) {
					t.Fatalf("review lacks reconciliation path: %v, %q", err, review.String())
				}
				priorRequests := len(requests)
				settings.ConnectorPermissions = []connector.PermissionRule{{ConnectorID: "google", Operation: "gmail_get", Read: connector.PermissionDeny}}
				if err := configStore.Save(settings); err != nil {
					t.Fatal(err)
				}
				if err := reconcileGmailSend(context.Background(), stateDir, work.ID, correlationID, &bytes.Buffer{}); err == nil || len(requests) != priorRequests {
					t.Fatalf("revoked read attempted network: %v, requests=%#v", err, requests)
				}
			}
		})
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
