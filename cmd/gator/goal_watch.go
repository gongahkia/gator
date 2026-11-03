package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/workspace"
)

// A resumed Work bundle includes inherited files and transcript replay. Only
// changed output and this execution's tool results can force a fresh alert.
func currentWatchEffects(outcome workrun.Outcome) ([]artifact.File, []agent.Message) {
	before := make(map[string]artifact.File, len(outcome.ParentArtifacts))
	for _, file := range outcome.ParentArtifacts {
		before[file.Path] = file
	}
	var changed []artifact.File
	for _, file := range outcome.Manifest.Artifacts {
		prior, exists := before[file.Path]
		if !exists || prior.SHA256 != file.SHA256 || prior.Bytes != file.Bytes {
			changed = append(changed, file)
		}
		delete(before, file.Path)
	}
	for _, removed := range before {
		changed = append(changed, removed)
	}
	if len(outcome.Events) == 0 {
		return changed, outcome.Result.Messages // Missing trace stays conservative.
	}
	var messages []agent.Message
	for _, event := range outcome.Events {
		if event.Kind == agent.EventToolFinished && event.ToolCall != nil {
			messages = append(messages, agent.Message{Role: agent.RoleTool, ToolName: event.ToolCall.Name, Content: event.ToolResult})
		}
	}
	return changed, messages
}

type goalWatchDecision struct {
	Continuation string    `json:"continuation,omitempty"`
	NextCheckAt  time.Time `json:"next_check_at,omitempty"`
	NextStepID   string    `json:"next_step_id,omitempty"`
	Attention    *bool     `json:"attention"`
	Urgent       bool      `json:"urgent"`
	Summary      string    `json:"summary"`
	NextStep     string    `json:"next_step"`
	Rationale    string    `json:"rationale"`
	Evidence     []string  `json:"evidence"`
	ChangeIDs    []string  `json:"change_ids"`
}

func parseGoalWatchDecision(finalText string) (goalWatchDecision, bool) {
	value := strings.TrimSpace(finalText)
	if strings.HasPrefix(value, "```json\n") && strings.HasSuffix(value, "\n```") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "```json\n"), "\n```")
	}
	var decision goalWatchDecision
	err := json.Unmarshal([]byte(value), &decision)
	return decision, err == nil && decision.Attention != nil && strings.TrimSpace(decision.Summary) != ""
}

func goalWatchUrgent(finalText string) bool {
	decision, valid := parseGoalWatchDecision(finalText)
	return valid && *decision.Attention && decision.Urgent
}

// A quiet watch is an explicit model decision with no durable side effect.
// If the final answer is malformed, surface it for review rather than hide it.
func classifyGoalWatchResult(finalText string, messages []agent.Message, artifacts []artifact.File, references watchReferenceSet) (bool, string) {
	decision := evaluateGoalWatchResult(finalText, messages, artifacts, references, nil)
	return decision.Status != "quiet", decision.Summary
}

func evaluateGoalWatchResult(finalText string, messages []agent.Message, artifacts []artifact.File, references watchReferenceSet, changes []jobs.WatchChange) jobs.WatchDecision {
	forcedAttention := len(artifacts) > 0
	failedSuggestion := false
	for _, message := range messages {
		if message.Role != agent.RoleTool || message.ToolName != "suggest_change" {
			continue
		}
		forcedAttention = true
		var result struct {
			OK bool `json:"ok"`
		}
		if json.Unmarshal([]byte(message.Content), &result) != nil || !result.OK {
			failedSuggestion = true
		}
	}
	decision, valid := parseGoalWatchDecision(finalText)
	if valid {
		if failedSuggestion {
			return jobs.WatchDecision{Status: "review", Summary: "Goal watch could not queue its proposed change; review the Work run.", NextStep: "Review the Work run and proposed change.", Rationale: "A suggested plan change failed host validation."}
		}
		summary := boundedWatchText(decision.Summary, 360)
		attention := *decision.Attention || forcedAttention
		status := "quiet"
		if attention {
			status = "attention"
		}
		result := jobs.WatchDecision{Status: status, Summary: summary, NextStep: boundedWatchText(decision.NextStep, 240), Rationale: boundedWatchText(decision.Rationale, 360), Urgent: attention && decision.Urgent}
		if result.Rationale == "" {
			result.Rationale = summary
		}
		if attention && result.NextStep == "" {
			result.Status = "review"
			result.NextStep = "Review the Work run."
		}
		allowedChanges := map[string]bool{}
		for _, change := range changes {
			allowedChanges[change.Source+" "+change.ID] = true
		}
		for _, id := range decision.ChangeIDs {
			id = boundedWatchText(id, 180)
			if !allowedChanges[id] {
				result.Status = "review"
				result.Summary += " [change ID unverified; review Work run]"
				continue
			}
			if len(result.ChangeIDs) < 8 {
				result.ChangeIDs = append(result.ChangeIDs, id)
			}
		}
		if attention {
			evidence, unverified := boundedWatchEvidence(decision.Evidence, references)
			if evidence != "" {
				result.Summary += " [evidence: " + evidence + "]"
				result.Evidence = strings.Split(evidence, "; ")
			} else {
				result.Status = "review"
				result.Summary += " [evidence missing or unverified; review Work run]"
			}
			if evidence != "" && unverified {
				result.Status = "review"
				result.Summary += " [other references unverified; review Work run]"
			}
		}
		if forcedAttention && !*decision.Attention {
			result.Status = "review"
			result.Rationale = "The Work run created an artifact or suggested a change despite a quiet model decision."
		}
		if !attention && len(decision.Evidence) > 0 {
			_, unverified := boundedWatchEvidence(decision.Evidence, references)
			if unverified {
				result.Status = "review"
				result.Summary += " [quiet rationale references unverified; review Work run]"
			}
		}
		return result
	}
	return jobs.WatchDecision{Status: "review", Summary: summarizeJobResult(finalText, artifacts), NextStep: "Review the Work run.", Rationale: "The watch returned an invalid decision."}
}

func boundedWatchText(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) > limit {
		value = string([]rune(value)[:limit])
	}
	return value
}

// watchReferenceSet checks reference integrity against material retained by
// this run. It cannot establish whether a referenced source supports a claim.
type watchReferenceSet struct {
	evidence  map[string]artifact.Evidence
	connected map[string]bool
	items     map[string]bool
	plan      map[string]bool
	source    workspace.Root
	files     map[string]artifact.File
	roots     map[string]workspace.Root
}

func newWatchReferenceSet(manifest artifact.Manifest, items []jobs.WatchItem, plan goal.Plan, work workspace.Work) watchReferenceSet {
	refs := watchReferenceSet{
		evidence: map[string]artifact.Evidence{}, connected: map[string]bool{},
		items: map[string]bool{}, plan: map[string]bool{}, source: work.Source,
		files: map[string]artifact.File{}, roots: map[string]workspace.Root{},
	}
	refs.addOutputReferences("output", manifest.Artifacts, work.Output)
	for _, entry := range manifest.Evidence {
		refs.evidence[entry.ID] = entry
		if strings.HasPrefix(entry.Locator, "source/") {
			data, err := work.Source.ReadRegularFile(strings.TrimPrefix(entry.Locator, "source/"), 512*1024)
			if err != nil || entry.SnapshotPath == "" {
				continue
			}
			digest := sha256.Sum256(data)
			if entry.SHA256 != hex.EncodeToString(digest[:]) {
				continue
			}
		}
		refs.evidence[entry.Locator] = entry
	}
	workRoot, rootErr := workspace.Open(work.Path)
	observed := map[string]map[string]bool{}
	for _, entry := range manifest.ConnectedSources {
		if rootErr != nil || entry.SnapshotPath == "" {
			continue
		}
		data, err := workRoot.ReadRegularFile(entry.SnapshotPath, 512*1024)
		if err != nil {
			continue
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != entry.SHA256 {
			continue
		}
		key := entry.ConnectorID + "/" + entry.Operation
		refs.connected[key] = true
		switch entry.Operation {
		case "gmail_get":
			var message struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(data, &message) == nil && message.ID != "" {
				key = entry.ConnectorID + "/gmail_list"
				if observed[key] == nil {
					observed[key] = map[string]bool{}
				}
				observed[key][message.ID] = true
			}
		case "calendar_events":
			var events struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			if json.Unmarshal(data, &events) == nil {
				if observed[key] == nil {
					observed[key] = map[string]bool{}
				}
				for _, event := range events.Items {
					if event.ID != "" {
						observed[key][event.ID] = true
					}
				}
			}
		case "fetch":
			if observed[key] == nil {
				observed[key] = map[string]bool{}
			}
			observed[key]["resource"] = true
		}
	}
	for _, item := range items {
		if observed[item.Source][item.ID] {
			refs.items[item.Source+" "+item.ID] = true
		}
	}
	for _, step := range plan.Steps {
		refs.plan["goal plan step "+step.ID] = true
	}
	if !plan.NextReviewAt.IsZero() {
		refs.plan["goal plan review"] = true
	}
	return refs
}

func (refs *watchReferenceSet) addOutputReferences(prefix string, files []artifact.File, root workspace.Root) {
	if root.Path() == "" {
		return
	}
	refs.roots[prefix] = root
	for _, file := range files {
		refs.files[prefix+"/"+file.Path] = file
	}
}

func (refs watchReferenceSet) allows(reference string) bool {
	if refs.items[reference] || refs.connected[reference] || refs.plan[reference] {
		return true
	}
	entry, found := refs.evidence[reference]
	if found {
		return entry.Locator != "" && entry.SHA256 != ""
	}
	colon := strings.LastIndexByte(reference, ':')
	if colon < 0 {
		return false
	}
	path, lineText := reference[:colon], reference[colon+1:]
	file, output := refs.files[path]
	if !strings.HasPrefix(path, "source/") && !output {
		return false
	}
	entry, found = refs.evidence[path]
	if !output && (!found || entry.Locator != path || entry.SnapshotPath == "") {
		return false
	}
	first, last, ranged := strings.Cut(lineText, "-")
	line, err := strconv.Atoi(first)
	if err != nil || line < 1 || line > 100_000 {
		return false
	}
	end := line
	if ranged {
		end, err = strconv.Atoi(last)
		if err != nil || end < line || end > 100_000 {
			return false
		}
	}
	root, relative, expected := refs.source, strings.TrimPrefix(path, "source/"), entry.SHA256
	if output {
		prefix, _, _ := strings.Cut(path, "/")
		root, relative, expected = refs.roots[prefix], file.Path, file.SHA256
	}
	data, err := root.ReadRegularFile(relative, 512*1024)
	if err != nil {
		return false
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != expected {
		return false
	}
	lineCount := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lineCount++
	}
	return end <= lineCount
}

func boundedWatchEvidence(references []string, allowed watchReferenceSet) (string, bool) {
	var selected []string
	seen := map[string]bool{}
	unverified := false
	for index, reference := range references {
		if index == 32 {
			unverified = true
			break
		}
		clean := strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, reference)
		clean = strings.Join(strings.Fields(clean), " ")
		if clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		if !allowed.allows(clean) {
			unverified = true
			continue
		}
		if len([]rune(clean)) > 100 {
			clean = string([]rune(clean)[:100]) + "…"
		}
		if len(selected) < 2 {
			selected = append(selected, clean)
		}
	}
	return strings.Join(selected, "; "), unverified
}
