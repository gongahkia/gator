package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/workapp"
	"github.com/gongahkia/gator/internal/workspace"
)

func receiptBundleFixture(t *testing.T, stateDir, resource string) (workspace.Work, workapp.ReceiptRequest, config.Settings) {
	t.Helper()
	settings := config.Default()
	settings.Connectors = []connector.Descriptor{{Version: connector.DescriptorVersion, ID: "google", Name: "Google", Kind: connector.KindGoogle, Resource: resource, Authentication: connector.AuthNone}}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := (connector.Runtime{Registry: registry}).PrepareAction("google", "gmail_send", json.RawMessage(`{"payload":{"from":"me@example.com","to":"venue@example.com","subject":"Venue update","body":"private body"}}`))
	if err != nil {
		t.Fatal(err)
	}
	work, err := workspace.CreateWork(t.TempDir(), stateDir, "receipt-send", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	contract := artifact.DefaultContract("report.md")
	contract.ExternalActions = action.Approve
	if err := artifact.WriteText(work.Output, contract, "report.md", "Prepared.\n"); err != nil {
		t.Fatal(err)
	}
	manifest, err := artifact.Seal(work.Output, contract, artifact.SealOptions{RunID: work.ID, Objective: "Send venue reply", Source: work.Source, Actions: []action.Record{{Proposal: prepared.Proposal, Status: action.Unknown, Error: "outcome unknown"}}, StartedAt: work.CreatedAt, FinishedAt: work.CreatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if err := artifact.WriteManifest(work.ManifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	summary, err := summarizeWorkBundle(work.Path)
	if err != nil {
		t.Fatal(err)
	}
	return work, workapp.ReceiptRequest{BundlePath: work.Path, RunID: work.ID, ManifestSHA256: summary.ManifestSHA256, CorrelationID: prepared.Proposal.CorrelationID, Action: "inspect"}, settings
}

func TestWorkReceiptInspectReadsOnlyOwnedRetainedCheck(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	_, request, _ := receiptBundleFixture(t, stateDir, server.URL)
	check := workReceiptChecker(stateDir)
	result, err := check(context.Background(), request)
	if err != nil || result.Status != "not_checked" || calls.Load() != 0 {
		t.Fatal("opening receipt attempted network", result, err)
	}
	saved := gmailReconciliation{Version: 1, WorkID: request.RunID, ManifestSHA256: request.ManifestSHA256, CorrelationID: request.CorrelationID, Status: "found", GmailID: "sent-1", CheckedAt: time.Now().UTC()}
	if err := writeGmailReconciliation(stateDir, saved); err != nil {
		t.Fatal(err)
	}
	result, err = check(context.Background(), request)
	if err != nil || result.Status != "found" || result.ProviderID != "sent-1" || result.CheckedAt == "" || calls.Load() != 0 {
		t.Fatal("retained check unavailable after reopening", result, err)
	}
	saved.ManifestSHA256 = strings.Repeat("a", 64)
	if err := writeGmailReconciliation(stateDir, saved); err != nil {
		t.Fatal(err)
	}
	if _, err := check(context.Background(), request); err == nil {
		t.Fatal("foreign manifest's check reused")
	}
	request.ManifestSHA256 = strings.Repeat("b", 64)
	request.Action = "check_sent"
	if _, err := check(context.Background(), request); err == nil || calls.Load() != 0 {
		t.Fatal("stale view performed network read", err)
	}
}

func TestWorkReceiptSentCheckUsesExactBundleAndCannotResend(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	var calls atomic.Int32
	var correlation string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet {
			t.Errorf("receipt sent mutation: %s", r.Method)
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/gmail/v1/users/me/messages" {
			if r.URL.Query().Get("q") != "in:sent rfc822msgid:<gator-"+correlation+"@gator.invalid>" {
				t.Error("query not bound to original send")
			}
			_, _ = w.Write([]byte(`{"messages":[{"id":"sent-1"}]}`))
			return
		}
		if r.URL.Path != "/gmail/v1/users/me/messages/sent-1" {
			t.Error("unexpected mailbox read", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"sent-1","labelIds":["SENT"],"payload":{"headers":[{"name":"Message-ID","value":"<gator-` + correlation + `@gator.invalid>"}]}}`))
	}))
	defer server.Close()
	work, request, settings := receiptBundleFixture(t, stateDir, server.URL)
	correlation = request.CorrelationID
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	// A second source with the same run ID makes global lookup ambiguous. The
	// native action must use its already owned exact bundle, never another run.
	_, _, _ = receiptBundleFixture(t, stateDir, server.URL)
	request.Action = "check_sent"
	result, err := workReceiptChecker(stateDir)(context.Background(), request)
	if err != nil || result.Status != "found" || calls.Load() != 2 {
		t.Fatal("owned exact bundle check failed", result, err, calls.Load())
	}
	bundle, err := artifact.OpenBundle(work.Path)
	if err != nil || artifact.VerifyBundle(bundle) != nil || bundle.Manifest.Actions[0].Status != action.Unknown {
		t.Fatal("Sent check rewrote original action", err)
	}
	settings.ConnectorPermissions = []connector.PermissionRule{{ConnectorID: "google", Operation: "gmail_get", Read: connector.PermissionDeny}}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := workReceiptChecker(stateDir)(context.Background(), request); err == nil || calls.Load() != 2 {
		t.Fatal("revoked access attempted mailbox reads", err)
	}
}

func TestWorkReceiptRejectsManifestReplacementDuringSentCheck(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	var manifestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("mutation attempted")
		}
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		if err := os.WriteFile(manifestPath, append(data, '\n'), 0600); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	work, request, settings := receiptBundleFixture(t, stateDir, server.URL)
	manifestPath = work.ManifestPath
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	request.Action = "check_sent"
	if _, err := workReceiptChecker(stateDir)(context.Background(), request); err == nil || !strings.Contains(err.Error(), "changed during") {
		t.Fatal("changed manifest received a receipt check", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "gator", "reconciliations", request.RunID, request.CorrelationID+".json")); !os.IsNotExist(err) {
		t.Fatal("stale check was persisted", err)
	}
}
