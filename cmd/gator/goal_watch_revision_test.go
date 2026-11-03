package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/followup"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/workspace"
)

type watchRevisionModel struct {
	step  int
	write bool
	text  string
}

func (m *watchRevisionModel) Complete(context.Context, agent.TurnRequest) (agent.Turn, error) {
	m.step++
	call := func(name string, args string) (agent.Turn, error) {
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: fmt.Sprintf("step-%d", m.step), Name: name, Arguments: json.RawMessage(args)}}}, nil
	}
	switch m.step {
	case 1:
		return call("read_file", `{"path":"source/project.md"}`)
	case 2:
		if m.write {
			args, _ := json.Marshal(map[string]string{"path": "update.md", "content": m.text})
			return call("write_artifact", string(args))
		}
	case 3:
		if m.write {
			return call("artifact_status", `{}`)
		}
	}
	text, _ := json.Marshal(map[string]any{"attention": m.write, "summary": m.text, "next_step": "Review the deadline", "rationale": "Compared current project facts", "evidence": []string{"source/project.md:1"}, "continuation": "idle"})
	return agent.Turn{Text: string(text)}, nil
}

func TestGoalWatchInheritedDraftAndCurrentEvidenceAcrossRevisions(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	writeSource := func(value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(source, "project.md"), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeSource("Deadline: 20 November\n")
	draft, err := followup.New(source, "")
	if err != nil {
		t.Fatal(err)
	}
	_, definition, err := saveFollowup(state, draft)
	if err != nil {
		t.Fatal(err)
	}
	store, _ := jobs.Open(state)
	var outcomes []workrun.Outcome
	run := func(write bool, text string) jobResult {
		t.Helper()
		model := &watchRevisionModel{write: write, text: text}
		result, err := runJobProcessWith(context.Background(), definition, state, func(ctx context.Context, _, _, _ string, request workrun.Request) (workrun.Outcome, error) {
			request.DisableBrowser, request.DisableDelegation, request.DisableChatHistory = true, true, true
			o, err := (workrun.Service{Executor: workrun.Executor{Model: model, StateDir: state}}).Execute(ctx, request)
			outcomes = append(outcomes, o)
			return o, err
		})
		if err != nil || result.Status != "completed" {
			t.Fatalf("run failed: %+v %v", result, err)
		}
		a, err := store.Begin(definition, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		a.Status, a.Conversation, a.Revision = result.Status, result.ConversationID, result.RevisionID
		a.FinishedAt, a.WatchDecision, a.NotificationPolicySHA256 = time.Now().UTC(), result.WatchDecision, jobNotificationPolicyHash(definition)
		if err := store.Record(a); err != nil {
			t.Fatal(err)
		}
		return result
	}
	initial := run(true, "Deadline is 20 November")
	if !initial.WatchAttention || initial.WatchDecision.Status != "attention" {
		t.Fatalf("new draft hidden: %+v", initial.WatchDecision)
	}
	writeSource("Deadline: 20 November\n<!-- cosmetic edit -->\n")
	quiet := run(false, "No material change")
	if quiet.WatchAttention || quiet.WatchDecision.Status != "quiet" {
		t.Fatalf("inherited output caused alert: %+v", quiet.WatchDecision)
	}
	if len(outcomes[1].Manifest.Artifacts) != 1 || len(outcomes[1].ParentArtifacts) != 1 {
		t.Fatal("quiet check lost the retained draft")
	}
	writeSource("Deadline: 17 November\n")
	changed := run(true, "Deadline is now 17 November")
	if !changed.WatchAttention || changed.WatchDecision.Status != "attention" || len(changed.WatchDecision.Evidence) != 1 {
		t.Fatalf("new draft/current citation not accepted: %+v", changed.WatchDecision)
	}
	// Prior evidence and output are immutable; current citation selection cannot
	// depend on which historical evidence ID sorts last.
	if outcomes[0].Manifest.Artifacts[0].SHA256 == outcomes[2].Manifest.Artifacts[0].SHA256 {
		t.Fatal("changed draft not actually revised")
	}
}

func TestGoalWatchEffectsDoNotReplayHistoricalSuggestions(t *testing.T) {
	file := artifact.File{Path: "draft.md", Bytes: 5, SHA256: "same"}
	o := workrun.Outcome{ParentArtifacts: []artifact.File{file}, Manifest: artifact.Manifest{Artifacts: []artifact.File{file}},
		Result: agent.Result{Messages: []agent.Message{{Role: agent.RoleTool, ToolName: "suggest_change", Content: `{"ok":true}`}}},
		Events: []agent.Event{{Kind: agent.EventRunFinished}}}
	files, messages := currentWatchEffects(o)
	if attention, _ := classifyGoalWatchResult(`{"attention":false,"summary":"No change"}`, messages, files, watchReferenceSet{}); attention {
		t.Fatal("historical suggestion or inherited file forced attention")
	}
	o.Events = append(o.Events, agent.Event{Kind: agent.EventToolFinished, ToolCall: &agent.ToolCall{Name: "suggest_change"}, ToolResult: `{"ok":true}`})
	files, messages = currentWatchEffects(o)
	if attention, _ := classifyGoalWatchResult(`{"attention":false,"summary":"No change"}`, messages, files, watchReferenceSet{}); !attention {
		t.Fatal("current suggestion hidden")
	}
	o.Manifest.Artifacts = nil
	files, _ = currentWatchEffects(o)
	if len(files) != 1 {
		t.Fatal("removed output hidden")
	}
}

func TestGoalWatchOutputReferencesBindExactRetainedBytes(t *testing.T) {
	root, _ := workspace.Open(t.TempDir())
	previous, _ := workspace.Open(t.TempDir())
	data := []byte("Retained draft\n")
	digest := sha256.Sum256(data)
	file := artifact.File{Path: "draft.md", Bytes: int64(len(data)), SHA256: hex.EncodeToString(digest[:])}
	for _, r := range []workspace.Root{root, previous} {
		if err := r.WriteRegularFileAtomic(file.Path, data, 1024); err != nil {
			t.Fatal(err)
		}
	}
	refs := newWatchReferenceSet(artifact.Manifest{Artifacts: []artifact.File{file}}, nil, goal.Plan{}, workspace.Work{Output: root})
	refs.addOutputReferences("previous", []artifact.File{file}, previous)
	for _, ref := range []string{"output/draft.md:1", "previous/draft.md:1-1"} {
		if !refs.allows(ref) {
			t.Fatal("retained reference rejected", ref)
		}
	}
	for _, ref := range []string{"output/draft.md:2", "previous/missing.md:1", "output/../draft.md:1"} {
		if refs.allows(ref) {
			t.Fatal("unowned reference accepted", ref)
		}
	}
	if err := previous.WriteRegularFileAtomic(file.Path, []byte("Replaced\n"), 1024); err != nil {
		t.Fatal(err)
	}
	if refs.allows("previous/draft.md:1") {
		t.Fatal("changed parent output accepted")
	}
	if !refs.allows("output/draft.md:1") {
		t.Fatal("unrelated current output invalidated")
	}
}
