// Opt-in live tests of the production memory/history tools with synthetic,
// disposable state. No files or credentials from a real Gator profile are read.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/privatefs"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

type historyCase struct {
	ID, User, Assistant, Title, Folder string
	Blocked                            bool
}

type scenario struct {
	ID, Split, Capture, Paraphrase, Correction, Restore, Forget string
	History                                                     []historyCase
}

// Freeze these cases before running the provider. Split is an evaluation label,
// not a statistical generalization claim; this is a small authored corpus.
var scenarios = []scenario{
	{ID: "reply-detail", Split: "development", Capture: "I prefer concise answers.", Paraphrase: "I prefer brief replies.", Correction: "I prefer detailed replies instead.", Restore: "I prefer medium-length replies.", Forget: "Forget my response-length preference.", History: []historyCase{
		{ID: "paraphrase", User: "Keep my explanations expansive; I enjoy all the detail.", Blocked: true},
		{ID: "translation", User: "我喜欢详细的回答。", Blocked: true},
		{ID: "assistant", User: "How should you write for me?", Assistant: "You like answers with plenty of explanation.", Blocked: true},
		{ID: "title", User: "The workshop is at Orchard Hall.", Title: "My preference for long answers", Blocked: true},
		{ID: "folder", User: "The workshop is at Orchard Hall.", Folder: "my-detailed-reply-preference", Blocked: true},
		{ID: "unrelated", User: "The workshop is at Orchard Hall."},
		{ID: "other-owner", User: "My colleague prefers detailed replies."},
	}},
	{ID: "home-city", Split: "held-out", Capture: "I live in Harbor City.", Paraphrase: "I live in Harbor City, my permanent home.", Correction: "I live in Birch City now.", Restore: "I live in Cedar City.", Forget: "Forget where I live.", History: []historyCase{
		{ID: "paraphrase", User: "My permanent base is Birch City.", Blocked: true},
		{ID: "translation", User: "我的家在Birch City。", Blocked: true},
		{ID: "assistant", User: "Where is my home?", Assistant: "You are based in Birch City.", Blocked: true},
		{ID: "title", User: "The workshop is at Orchard Hall.", Title: "Home in Birch City", Blocked: true},
		{ID: "folder", User: "The workshop is at Orchard Hall.", Folder: "my-home-in-Birch-City", Blocked: true},
		{ID: "unrelated", User: "The workshop is at Orchard Hall."},
		{ID: "other-owner", User: "My colleague lives in Birch City."},
	}},
	{ID: "keyboard", Split: "held-out", Capture: "I prefer split keyboards.", Paraphrase: "I prefer keyboards with separate left and right halves.", Correction: "I prefer conventional single-piece keyboards instead.", Restore: "I prefer compact keyboards.", Forget: "Forget my keyboard preference.", History: []historyCase{
		{ID: "paraphrase", User: "For my own desk I favor a standard keyboard in one piece.", Blocked: true},
		{ID: "translation", User: "我喜欢传统的一体式键盘。", Blocked: true},
		{ID: "assistant", User: "What keyboard suits my preference?", Assistant: "You favor a conventional one-piece board.", Blocked: true},
		{ID: "title", User: "The workshop is at Orchard Hall.", Title: "My preference for traditional keyboards", Blocked: true},
		{ID: "folder", User: "The workshop is at Orchard Hall.", Folder: "my-keyboard-preferences", Blocked: true},
		{ID: "unrelated", User: "The workshop is at Orchard Hall."},
		{ID: "other-owner", User: "My colleague prefers single-piece keyboards."},
	}},
}

type check struct {
	Name         string           `json:"name"`
	Passed       bool             `json:"passed"`
	Milliseconds int64            `json:"milliseconds"`
	Calls        []agent.ToolCall `json:"tool_calls,omitempty"`
	Result       string           `json:"result,omitempty"`
	Error        string           `json:"error,omitempty"`
}

type trial struct {
	ID      string  `json:"id"`
	Split   string  `json:"split"`
	Passed  bool    `json:"passed"`
	Checks  []check `json:"checks"`
	Reviews []any   `json:"reviews,omitempty"`
	Error   string  `json:"error,omitempty"`
}

type evaluation struct {
	state    string
	backend  agent.Model
	current  *trial
	sequence int
}

func (e *evaluation) chat(ctx context.Context, name, message string, want int, contains, sameID string) (record memory.Record, finalErr error) {
	started := time.Now()
	c := check{Name: name}
	defer func() {
		c.Milliseconds = time.Since(started).Milliseconds()
		c.Passed = finalErr == nil
		if finalErr != nil {
			c.Error = finalErr.Error()
		}
		e.current.Checks = append(e.current.Checks, c)
	}()
	// Reopen every surface and use a fresh conversation/context, so a previous
	// in-memory transcript cannot hide a broken durable association.
	memories, err := memory.Open(e.state)
	if err != nil {
		return record, err
	}
	notes, err := knowledge.Open(e.state)
	if err != nil {
		return record, err
	}
	sessions, err := worksession.Open(e.state)
	if err != nil {
		return record, err
	}
	conversation, err := sessions.Create("Evaluation", filepath.Join(e.state, "workspace"), "snap-eval", time.Now())
	if err != nil {
		return record, err
	}
	e.sequence++
	runID := fmt.Sprintf("rev-step-%d", e.sequence)
	available := (tools.ChatMemory{Store: memories, UserMessage: message, ConversationID: conversation.ID, RunID: runID, Automatic: true, Semantics: e.backend, Review: func(_ context.Context, preview any) (bool, error) {
		e.current.Reviews = append(e.current.Reviews, preview)
		p, ok := preview.(map[string]any)
		// Only explicit synthetic abstract-topic review is simulated. Ordinary
		// capture, conflicting groups and unfamiliar consent are never approved.
		return ok && p["action"] == "withdrawal_topic", nil
	}}).Tools()
	available = append(available, (tools.PersonalKnowledge{Notes: notes, Memories: memories}).Tools()[:2]...)
	profile, err := memories.Profile(nil)
	if err != nil {
		return record, err
	}
	profileText, _ := memory.RenderSelection(profile)
	bounded, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	result, err := (agent.Runner{Model: e.backend, Tools: available}).Run(bounded, agent.RunOptions{System: "You are Gator. Respond concisely.\n" + workrun.MemoryPrompt(true, true) + "\nCurrent selected memory:\n" + profileText, Task: message, MaxSteps: 6, OnEvent: func(event agent.Event) {
		if event.Kind == agent.EventToolCalled && event.ToolCall != nil {
			c.Calls = append(c.Calls, *event.ToolCall)
		}
	}})
	c.Result = result.FinalText
	if err != nil {
		return record, err
	}
	_, err = sessions.AddRevision(conversation.ID, worksession.Revision{ID: runID, SnapshotID: "snap-eval", Objective: message, FinalText: result.FinalText, BundlePath: "/synthetic/bundle", Status: "completed"})
	if err != nil {
		return record, err
	}
	records, err := memories.List("", true)
	if err != nil {
		return record, err
	}
	if len(records) != want {
		return record, fmt.Errorf("saved count = %d; want %d", len(records), want)
	}
	if want == 1 {
		record = records[0]
		if !strings.Contains(record.Text, contains) {
			return record, errors.New("current user statement was not retained")
		}
		if sameID != "" && record.ID != sameID {
			return record, errors.New("equivalence/correction changed identity")
		}
		if len(record.Evidence) == 0 || record.Evidence[len(record.Evidence)-1].ConversationID != conversation.ID {
			return record, errors.New("current observation lacks source attribution")
		}
	}
	return record, nil
}

func (e *evaluation) history(ctx context.Context, name string, item historyCase, conversation worksession.Conversation) (finalErr error) {
	started := time.Now()
	c := check{Name: name + "/" + item.ID}
	defer func() {
		c.Milliseconds = time.Since(started).Milliseconds()
		c.Passed = finalErr == nil
		if finalErr != nil {
			c.Error = finalErr.Error()
		}
		e.current.Checks = append(e.current.Checks, c)
	}()
	h := tools.ChatHistory{StateDir: e.state, CurrentConversation: "work-current", Semantics: e.backend}
	raw, _ := json.Marshal(map[string]string{"conversation_id": conversation.ID})
	result, err := h.Tools()[1].Execute(ctx, raw)
	if err != nil {
		return err
	}
	c.Result = result.Content
	var data struct {
		Turns []struct {
			ConversationID string `json:"conversation_id"`
			RevisionID     string `json:"revision_id"`
		} `json:"turns_newest_first"`
		Withheld int  `json:"withheld_historical_turns"`
		Pending  bool `json:"withdrawal_migration_pending"`
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(result.Content), &envelope); err != nil {
		return err
	}
	if err := json.Unmarshal(envelope.Result, &data); err != nil {
		return err
	}
	if data.Pending {
		return errors.New("abstract withdrawal metadata was not resolved")
	}
	if item.Blocked {
		if len(data.Turns) != 0 || data.Withheld != 1 {
			return errors.New("withdrawn historical context reached the foreground tool result")
		}
	} else if len(data.Turns) != 1 || data.Turns[0].ConversationID != conversation.ID || data.Turns[0].RevisionID != "rev-archive" {
		return errors.New("unrelated history or its provenance was lost")
	}
	return nil
}

func (e *evaluation) run(ctx context.Context, item scenario) error {
	first, err := e.chat(ctx, "passive capture", item.Capture, 1, item.Capture, "")
	if err != nil {
		return err
	}
	if _, err := e.chat(ctx, "equivalent paraphrase", item.Paraphrase, 1, item.Paraphrase, first.ID); err != nil {
		return err
	}
	if _, err := e.chat(ctx, "inspected correction", item.Correction, 1, item.Correction, first.ID); err != nil {
		return err
	}
	sessions, _ := worksession.Open(e.state)
	chats := map[string]worksession.Conversation{}
	for _, h := range item.History {
		title := h.Title
		if title == "" {
			title = "Archive notes"
		} else {
			title = "Archive notes: " + title
		}
		folder := h.Folder
		if folder == "" {
			folder = "neutral-workspace"
		}
		conversation, err := sessions.Create(title, filepath.Join(e.state, folder), "snap-eval", time.Now())
		if err != nil {
			return err
		}
		if _, err := sessions.AddRevision(conversation.ID, worksession.Revision{ID: "rev-archive", SnapshotID: "snap-eval", Objective: h.User, FinalText: h.Assistant, BundlePath: "/synthetic/archive", Status: "completed"}); err != nil {
			return err
		}
		chats[h.ID] = conversation
	}
	if _, err := e.chat(ctx, "explicit forget", item.Forget, 0, "", ""); err != nil {
		return err
	}
	if _, err := e.chat(ctx, "passive relearning refused", item.Correction, 0, "", ""); err != nil {
		return err
	}
	var failures []error
	for _, h := range item.History {
		if err := e.history(ctx, "historical privacy", h, chats[h.ID]); err != nil {
			failures = append(failures, err)
		}
	}
	// Test search separately: a correct direct-read path cannot mask raw excerpts
	// leaking through titles, workspace metadata, or global result serialization.
	started := time.Now()
	raw := json.RawMessage(`{"query":"Archive"}`)
	result, searchErr := (tools.ChatHistory{StateDir: e.state, Semantics: e.backend}).Tools()[0].Execute(ctx, raw)
	searchCheck := check{Name: "search privacy and benign recall", Milliseconds: time.Since(started).Milliseconds(), Result: result.Content}
	if searchErr == nil {
		var data struct {
			Matches []struct {
				ConversationID string `json:"conversation_id"`
			} `json:"matches"`
		}
		var envelope struct {
			Result json.RawMessage `json:"result"`
		}
		searchErr = json.Unmarshal([]byte(result.Content), &envelope)
		if searchErr == nil {
			searchErr = json.Unmarshal(envelope.Result, &data)
		}
		if searchErr == nil {
			ids := map[string]bool{}
			for _, match := range data.Matches {
				ids[match.ConversationID] = true
			}
			for _, h := range item.History {
				if ids[chats[h.ID].ID] == h.Blocked {
					searchErr = errors.Join(searchErr, fmt.Errorf("unexpected search visibility for %s", h.ID))
				}
			}
		}
	}
	searchCheck.Passed = searchErr == nil
	if searchErr != nil {
		searchCheck.Error = searchErr.Error()
		failures = append(failures, searchErr)
	}
	e.current.Checks = append(e.current.Checks, searchCheck)
	if _, err := e.chat(ctx, "explicit restoration", "Please remember: "+item.Restore, 1, item.Restore, ""); err != nil {
		return errors.Join(append(failures, err)...)
	}
	// Restoration must not reopen historical copies. Check one body and the
	// revealing metadata; body and metadata have independent time barriers.
	for _, index := range []int{0, 3, 4} {
		h := item.History[index]
		if err := e.history(ctx, "old history after restore", h, chats[h.ID]); err != nil {
			failures = append(failures, err)
		}
	}
	// A brand new source after restoration is usable. Its metadata date is
	// separate from the body date and does not inherit an old revealing title.
	newChat, err := sessions.Create("Restored preference", filepath.Join(e.state, "neutral-workspace"), "snap-eval", time.Now())
	if err != nil {
		return errors.Join(append(failures, err)...)
	}
	if _, err := sessions.AddRevision(newChat.ID, worksession.Revision{ID: "rev-archive", SnapshotID: "snap-eval", Objective: item.Restore, BundlePath: "/synthetic/new-source", Status: "completed"}); err != nil {
		return errors.Join(append(failures, err)...)
	}
	if err := e.history(ctx, "new context after restore", historyCase{ID: "new-current-value"}, newChat); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func executorCheck(ctx context.Context, backend agent.Model, root string) (c check) {
	c.Name = "production Work executor capture"
	started := time.Now()
	defer func() { c.Milliseconds = time.Since(started).Milliseconds() }()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0700); err != nil {
		c.Error = err.Error()
		return c
	}
	result, err := (workrun.Executor{Model: backend, StateDir: root}).Execute(ctx, workrun.Request{SourcePath: workspace, Objective: "I prefer concise answers.", ForegroundApp: true, LearnFromChats: true, DisableChatHistory: true, DisableBrowser: true, DisableDelegation: true, Mode: action.Inspect, Contract: artifact.InspectionContract(), Limits: agent.Limits{ModelRequests: 12, Tokens: 30000, WallSeconds: 90}, OnEvent: func(event agent.Event) {
		if event.Kind == agent.EventToolCalled && event.ToolCall != nil {
			c.Calls = append(c.Calls, *event.ToolCall)
		}
	}})
	if err != nil {
		c.Error = err.Error()
		return c
	}
	c.Result = result.ConversationID + "/" + result.RevisionID
	s, err := memory.Open(root)
	if err != nil {
		c.Error = err.Error()
		return c
	}
	records, err := s.List("", true)
	if err != nil {
		c.Error = err.Error()
		return c
	}
	if len(records) != 1 || records[0].Text != "I prefer concise answers." || records[0].Origin == nil || records[0].Origin.ConversationID != result.ConversationID {
		c.Error = "production capture or its source attribution did not match"
		return c
	}
	c.Passed = true
	return c
}

func run() (finalErr error) {
	provider := flag.String("provider", "openai", "direct provider; credentials via environment only")
	modelName := flag.String("model", "", "explicit model, otherwise repository default")
	endpoint := flag.String("endpoint", "", "full adapter endpoint")
	split := flag.String("split", "development", "development, held-out or all")
	path := flag.String("report", "", "new private report file")
	flag.Parse()
	if *path == "" || (*split != "development" && *split != "held-out" && *split != "all") {
		return errors.New("supply --report NEW_FILE and --split development|held-out|all")
	}
	f, err := os.OpenFile(*path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := privatefs.Protect(*path); err != nil {
		return err
	}
	selected, err := model.ParseProvider(*provider)
	if err != nil {
		return err
	}
	name := model.EffectiveModel(selected, *modelName)
	dataset, _ := json.Marshal(scenarios)
	digest := sha256.Sum256(dataset)
	budget := &agent.Budget{Limits: agent.Limits{ModelRequests: 240, Tokens: 300000}}
	report := struct {
		Provider, Model, Split, DatasetSHA256, Scope string
		At                                           time.Time
		Scenarios                                    []scenario
		Trials                                       []trial
		Usage                                        agent.Usage
		Error                                        string `json:",omitempty"`
	}{Provider: string(selected), Model: name, Split: *split, DatasetSHA256: hex.EncodeToString(digest[:]), Scope: "Synthetic longitudinal production-tool evaluation with fresh contexts/reopened stores; abstract-topic approvals simulated and recorded. No real profile, UI, external actions, or broad generalization claim.", At: time.Now().UTC(), Scenarios: scenarios}
	defer func() {
		report.Usage = budget.Usage()
		if finalErr != nil {
			report.Error = finalErr.Error()
		}
		encoder := json.NewEncoder(f)
		encoder.SetIndent("", "  ")
		finalErr = errors.Join(finalErr, encoder.Encode(report))
	}()
	backend, err := model.New(model.Config{Provider: selected, Model: name, BaseURL: *endpoint})
	if err != nil {
		return err
	}
	root, err := os.MkdirTemp("", "gator-longitudinal-eval-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	for _, item := range scenarios {
		if *split != "all" && *split != item.Split {
			continue
		}
		current := trial{ID: item.ID, Split: item.Split}
		e := evaluation{state: filepath.Join(root, item.ID), backend: agent.WithBudget(backend.Model, budget), current: &current}
		err := e.run(ctx, item)
		if item.Split == "development" {
			production := executorCheck(ctx, e.backend, filepath.Join(root, "real-executor"))
			current.Checks = append(current.Checks, production)
			if !production.Passed {
				err = errors.Join(err, errors.New(production.Error))
			}
		}
		current.Passed = err == nil
		if err != nil {
			current.Error = err.Error()
			finalErr = errors.Join(finalErr, fmt.Errorf("%s: %w", item.ID, err))
		}
		report.Trials = append(report.Trials, current)
		fmt.Printf("%s: passed=%t checks=%d\n", item.ID, current.Passed, len(current.Checks))
		if err != nil && len(current.Checks) <= 1 {
			break
		}
	}
	if len(report.Trials) == 0 {
		return errors.New("no evaluation cases selected")
	}
	return finalErr
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
