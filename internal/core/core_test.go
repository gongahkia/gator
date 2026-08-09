package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicyRoundTripAndValidation(t *testing.T) {
	root := gitRepository(t)
	policy := DefaultPolicy()
	policy.Verification.Commands = []CommandSpec{{ID: "unit", Argv: []string{"sh", "-c", "test -f README.md"}}}
	reference, err := SavePolicy(root, policy)
	if err != nil {
		t.Fatal(err)
	}
	loaded, loadedReference, err := LoadPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != reference.Version || loadedReference.Version != reference.Version {
		t.Fatalf("policy version was not stable: %#v %#v", reference, loadedReference)
	}
	loaded.Verification.Commands = []CommandSpec{{ID: "bad", Argv: nil}}
	if err := loaded.Validate(); err == nil {
		t.Fatal("policy accepted an argv-free verification command")
	}
}

func TestContextIsRedactedBoundedAndMetadataOnly(t *testing.T) {
	root := gitRepository(t)
	path := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(path, []byte("api_key=secret-value\nline two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace, err := DiscoverWorkspace(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	policy := DefaultPolicy()
	policy.Context.IncludeDiff = false
	policy.Context.MaxBytes = 256
	prepared, err := BuildContext(context.Background(), workspace, "inspect secret", []FileReference{{Path: "secret.txt"}}, false, policy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prepared.Prompt, "secret-value") {
		t.Fatal("context prompt retained secret")
	}
	entry := prepared.Manifest.Entries[0]
	if entry.Redactions != 1 || entry.Path != "secret.txt" || entry.Bytes == 0 {
		t.Fatalf("unexpected context metadata: %#v", entry)
	}
	encoded := fmt.Sprintf("%#v", prepared.Manifest)
	if strings.Contains(encoded, "secret-value") {
		t.Fatal("context manifest retained source content")
	}
}

func TestDryRunPersistsRunEventsAndEpisode(t *testing.T) {
	engine := testEngine(t)
	includeDiff := false
	run, err := engine.Run(context.Background(), RunRequest{Objective: "catalog source", Provider: "fake", IncludeDiff: &includeDiff}, nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != "completed" || run.Transport != "terminal" || run.Policy.Version == "" {
		t.Fatalf("unexpected dry run: %#v", run)
	}
	persisted, err := engine.Store().GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Objective != "catalog source" {
		t.Fatalf("objective did not persist: %#v", persisted)
	}
	events, err := engine.Store().Events(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Type != "run.created" || events[2].Type != "run.dry_run" {
		t.Fatalf("event journal is incomplete: %#v", events)
	}
	if _, err := os.Stat(filepath.Join(engine.Store().Dir(), "episodes", "episode-"+run.ID+".json")); err != nil {
		t.Fatalf("episode was not written: %v", err)
	}
}

func TestExecutionAndVerificationPersistEvidence(t *testing.T) {
	engine := testEngine(t)
	policy := DefaultPolicy()
	policy.Context.IncludeDiff = false
	policy.Verification.Commands = []CommandSpec{{ID: "unit", Argv: []string{"sh", "-c", "test -f README.md"}}}
	if _, err := SavePolicy(engine.Workspace().Root, policy); err != nil {
		t.Fatal(err)
	}
	includeDiff := false
	run, err := engine.Run(context.Background(), RunRequest{Objective: "verify fixture", Provider: "fake", IncludeDiff: &includeDiff, Execute: true}, nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != "completed" || run.Verification.State != "passed" || len(run.Verification.EvidenceIDs) != 1 {
		t.Fatalf("expected verified run, got %#v", run)
	}
	path := filepath.Join(engine.Store().Dir(), "evidence", run.ID, run.Verification.EvidenceIDs[0]+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "README.md\n") {
		t.Fatal("evidence persisted command output instead of metadata")
	}
}

func TestWorktreeAndExperimentPlansAreIsolated(t *testing.T) {
	engine := testEngine(t)
	includeDiff := false
	run, err := engine.Run(context.Background(), RunRequest{Objective: "isolated run", Provider: "fake", IncludeDiff: &includeDiff, Worktree: true}, nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if run.Workspace.Kind != "worktree" || run.Workspace.Root == engine.Workspace().Root {
		t.Fatalf("run did not use an isolated worktree: %#v", run.Workspace)
	}
	baseline, candidate := filepath.Join(t.TempDir(), "baseline.json"), filepath.Join(t.TempDir(), "candidate.json")
	policy := DefaultPolicy()
	policy.Context.IncludeDiff = false
	writePolicy(t, baseline, policy)
	policy.Workflow.Topology = "research_writer_reviewer"
	writePolicy(t, candidate, policy)
	result, err := engine.ComparePolicies(context.Background(), ExperimentRequest{Objective: "compare plans", Provider: "fake", BaselinePolicy: baseline, CandidatePolicy: candidate}, nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != "inconclusive" || result.BaselineRunID == "" || result.CandidateRunID == "" {
		t.Fatalf("unexpected experiment: %#v", result)
	}
	for _, id := range []string{result.BaselineRunID, result.CandidateRunID} {
		value, err := engine.Store().GetRun(id)
		if err != nil {
			t.Fatal(err)
		}
		if value.Workspace.Root == engine.Workspace().Root {
			t.Fatalf("experiment run %s used active checkout", id)
		}
	}
}

func testEngine(t *testing.T) *Engine {
	t.Helper()
	root := gitRepository(t)
	workspace, err := DiscoverWorkspace(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(workspace.Root)
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	return NewForTest(workspace, store, NewRegistry(fakeProvider{}))
}

type fakeProvider struct{}

func (fakeProvider) ID() string { return "fake" }

func (fakeProvider) Status(context.Context) ProviderStatus {
	return ProviderStatus{ID: "fake", Available: true, Executable: "sh", Capabilities: ProviderCapabilities{Terminal: true}}
}
func (fakeProvider) Start(ctx context.Context, workspace Workspace, _ string, input io.Reader, output, errors io.Writer) (*exec.Cmd, error) {
	command := exec.CommandContext(ctx, "sh", "-c", "exit 0")
	command.Dir, command.Stdin, command.Stdout, command.Stderr = workspace.Root, input, output, errors
	return command, nil
}

func gitRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Gator Test"}} {
		runCommand(t, root, "git", args...)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runCommand(t, root, "git", "add", "README.md")
	runCommand(t, root, "git", "commit", "-qm", "fixture")
	return root
}

func runCommand(t *testing.T, cwd, name string, args ...string) {
	t.Helper()
	command := exec.Command(name, args...)
	command.Dir = cwd
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v: %s", name, strings.Join(args, " "), err, output)
	}
}
func writePolicy(t *testing.T, path string, policy Policy) {
	t.Helper()
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
