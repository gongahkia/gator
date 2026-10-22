package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/followup"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	sharedusage "github.com/gongahkia/gator/internal/usage"
	"github.com/gongahkia/gator/internal/workhome"
)

func TestRecoveryServiceOwnershipAndReviewedChanges(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	for _, platform := range []string{"darwin", "linux"} {
		t.Run(platform, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "service")
			contents := launchAgentPlist("gator", "/gator", root, "/log")
			if platform == "linux" {
				contents, _ = systemdJobUnit("/gator", root, "")
			}
			if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			installed, owns, digest, err := recoveryServiceReview(path, root, platform)
			if err != nil || !installed || !owns || digest == "absent" {
				t.Fatal("own service unrecognized", err)
			}
			_, owns, _, err = recoveryServiceReview(path, other, platform)
			if err != nil || owns {
				t.Fatal("another profile claimed the service", err)
			}
		})
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GATOR_STATE_DIR", root)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	path, err := recoveryServicePath()
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	runner := func(_ context.Context, selected string, args ...string) (string, error) {
		if selected != root {
			t.Fatal("wrong service state directory")
		}
		calls = append(calls, append([]string(nil), args...))
		return "configured", nil
	}
	screen, err := recoveryControlWith(context.Background(), root, "status", nil, runner)
	if err != nil {
		t.Fatal(err)
	}
	var review []string
	for _, a := range screen.Actions {
		if a.Action == "install" {
			review = a.Args
		}
	}
	if len(review) != 2 || review[1] != "absent" {
		t.Fatal("no bound service review")
	}
	if _, err = recoveryControlWith(context.Background(), root, "install", review, runner); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || strings.Join(calls[0], " ") != "stop" || calls[1][1] != "install" {
		t.Fatalf("service actions: %#v", calls)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	contents := launchAgentPlist("gator", "/gator", root, "/log")
	if runtime.GOOS == "linux" {
		contents, _ = systemdJobUnit("/gator", root, "")
	}
	if err = os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, ownDigest, err := recoveryServiceReview(path, root, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = recoveryControlWith(context.Background(), root, "remove", []string{"", ownDigest}, runner); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || strings.Join(calls[2][1:], " ") != "remove --yes" {
		t.Fatal("service removal missing reviewed confirmation", calls)
	}
	contents = launchAgentPlist("gator", "/gator", other, "/log")
	if runtime.GOOS == "linux" {
		contents, _ = systemdJobUnit("/gator", other, "")
	}
	if err = os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = recoveryControlWith(context.Background(), root, "install", review, runner); err == nil {
		t.Fatal("stale service review accepted")
	}
	screen, err = recoveryControlWith(context.Background(), root, "status", nil, runner)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range screen.Actions {
		if a.Action == "install" || a.Action == "remove" {
			t.Fatal("offered to alter another profile's service")
		}
	}
	if len(calls) != 3 {
		t.Fatal("unreviewed service action was called")
	}
}

func TestSharedPauseLeavesChecksLimitedWithoutModelCallsOrAlerts(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", root)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	draft, _ := followup.New(t.TempDir(), "")
	_, definition, err := saveFollowup(root, draft)
	if err != nil {
		t.Fatal(err)
	}
	ledger, _ := sharedusage.Open(root)
	if err = ledger.SavePolicy(sharedusage.Policy{Paused: true}); err != nil {
		t.Fatal(err)
	}
	store, _ := jobs.Open(root)
	attempt, err := store.Begin(definition, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	result := executeJobContextWith(context.Background(), definition, attempt, root, false, func(context.Context, jobs.Definition, string, ...string) (jobResult, error) {
		calls++
		return jobResult{}, nil
	})
	if calls != 0 || result.Status != "limited" || result.LimitReason != "shared" || len(result.Runs) != 0 {
		t.Fatalf("paused work was started: %#v", result)
	}
	if err = store.Record(result); err != nil {
		t.Fatal(err)
	}
	entry, err := recordJobInbox(root, definition, result)
	if err != nil || entry.ID != "" {
		t.Fatal("shared pause produced an alert", err)
	}
	checks, err := workhome.ReadChecks(root, true, time.Now())
	if err != nil || len(checks) != 1 || checks[0].State != "shared limit" {
		t.Fatalf("shared-limit health: %#v %v", checks, err)
	}
}
func TestRecoveryTokenIsBoundToReviewedConnectorResource(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", root)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	var output bytes.Buffer
	if err := connectorCommandWithIO([]string{"add", "planning", "--url", "https://example.com/planning", "--auth", "bearer"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	goals, _ := goal.Open(root)
	g, err := goals.Add("Project", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	draft, _ := followup.New(g.SourcePath, g.ID)
	draft.ConnectorIDs = []string{"planning"}
	definition, err := draft.Definition(g.ID, []jobs.WatchSource{{ConnectorID: "planning", Operation: "fetch"}})
	if err != nil {
		t.Fatal(err)
	}
	store, _ := jobs.Open(root)
	definition, err = store.Save(definition)
	if err != nil {
		t.Fatal(err)
	}
	screen, err := recoveryControl(context.Background(), root, "status", []string{definition.ID})
	if err != nil {
		t.Fatal(err)
	}
	controls := &connectionControls{stateDir: root}
	routed := false
	for _, a := range screen.Actions {
		if a.Family == "connections" && a.Action == "select" && len(a.Args) == 1 && a.Args[0] == "planning" {
			routed = true
			screen, err = controls.call(context.Background(), a.Action, a.Args)
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if !routed {
		t.Fatal("recovery did not route to native connection management")
	}
	var args []string
	for _, a := range screen.Actions {
		if a.Action == "token" {
			args = append(append([]string(nil), a.Args...), "private-token")
		}
	}
	if len(args) != 4 {
		t.Fatal("native connection did not offer a reviewed token form")
	}
	screen, err = controls.call(context.Background(), "token", args)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(screen)
	if strings.Contains(string(encoded), "private-token") {
		t.Fatal("credential leaked into a control screen")
	}
	descriptor, _, _ := configuredConnector("planning")
	credentials, _ := auth.New(root)
	stored, found, err := credentials.Read(descriptor.CredentialRef())
	if err != nil || !found || stored.Access != "private-token" {
		t.Fatal("resource-bound credential not stored", err)
	}
	settingsStore, _ := config.DefaultStore()
	settings, err := settingsStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	settings.Connectors[0].Resource = "https://example.com/another-project"
	if err = settingsStore.Save(settings); err != nil {
		t.Fatal(err)
	}
	if _, err = controls.call(context.Background(), "token", args); err == nil {
		t.Fatal("stale reconnect review accepted a different resource")
	}
	if err = loginConnector("planning", []string{"--review-digest", connectorLoginReview(descriptor)}, strings.NewReader(""), &output); err == nil || !strings.Contains(err.Error(), "changed during review") {
		t.Fatal("login handoff accepted a stale connector agreement", err)
	}
}
