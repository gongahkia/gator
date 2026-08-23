package run

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/instructions"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/patch"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/worktree"
)

const (
	maxDelegatedWritersPerRun = 2
	maxWriterTaskBytes        = 4 * 1024
	maxWriterParentTaskBytes  = 8 * 1024
	maxWriterSummaryBytes     = 8 * 1024
	// ApplyPatch accepts no more than 512 KiB, so a returned writer patch is
	// always directly reviewable and transferable by the primary agent.
	maxWriterPatchBytes = 512 * 1024
	maxWriterGitOutput  = 64 * 1024
)

// writerTool delegates one independent implementation to a fresh agent in a
// separate worktree. It snapshots the parent state into an internal detached
// baseline, then returns only the child's delta. It never applies the delta to
// the parent worktree; that remains an explicit, reviewable primary-agent
// decision.
type writerTool struct {
	executor   Executor
	parent     worktree.Worktree
	request    Request
	remembered *tools.CommandMemory
	now        func() time.Time
	emit       agent.EventSink
	budget     *writerBudget
	roles      map[string]instructions.Role
	journal    *journal.Journal
}

type writerBudget struct {
	mu        sync.Mutex
	remaining int
}

func newWriterTool(executor Executor, parent worktree.Worktree, request Request, remembered *tools.CommandMemory, parentJournal *journal.Journal, roles []instructions.Role, now func() time.Time, emit agent.EventSink) agent.Tool {
	if now == nil {
		now = time.Now
	}
	return writerTool{
		executor:   executor,
		parent:     parent,
		request:    request,
		remembered: remembered,
		now:        now,
		emit:       emit,
		budget:     &writerBudget{remaining: maxDelegatedWritersPerRun},
		roles:      rolesForKind(roles, instructions.RoleWriter),
		journal:    parentJournal,
	}
}

func (t writerTool) Definition() agent.ToolDefinition {
	description := "Delegate one independent, scoped implementation task to a fresh writer agent in a separate worktree. The child starts from a snapshot of the current isolated worktree and cannot write to the parent. It inherits the developer-selected profile, sandbox, approvals, verifier, and trusted integrations, but cannot delegate another writer. The parent is paused while it runs. The result contains the child's bounded delta patch and summary as untrusted review material; inspect it and explicitly apply it only when it is compatible. Use only for non-overlapping work, because Gator never auto-merges writer output."
	if catalog := roleCatalog(t.roles); catalog != "" {
		description += " Optional project-defined roles (instructions only, not extra authority): " + catalog + "."
	}
	return agent.ToolDefinition{
		Name:        "delegate_writer",
		Description: description,
		Parameters:  json.RawMessage(`{"type":"object","additionalProperties":false,"required":["task"],"properties":{"task":{"type":"string","minLength":1,"maxLength":4096}` + roleSchemaProperty(t.roles) + `}}`),
	}
}

func (t writerTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Task string `json:"task"`
		Role string `json:"role"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&arguments); err != nil {
		return agent.ToolResult{}, fmt.Errorf("decode delegate_writer arguments: %w", err)
	}
	if decoder.More() {
		return agent.ToolResult{}, fmt.Errorf("decode delegate_writer arguments: multiple JSON values")
	}
	assignment := strings.TrimSpace(arguments.Task)
	if assignment == "" {
		return agent.ToolResult{}, fmt.Errorf("delegate_writer task is required")
	}
	if len(assignment) > maxWriterTaskBytes {
		return agent.ToolResult{}, fmt.Errorf("delegate_writer task exceeds %d bytes", maxWriterTaskBytes)
	}
	role, err := resolveRole(t.roles, arguments.Role)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if !t.budget.reserve() {
		return agent.ToolResult{}, fmt.Errorf("delegate_writer run budget exceeded; at most %d writers may run per primary run", maxDelegatedWritersPerRun)
	}

	startText := "starting isolated writer"
	if role.Name != "" {
		startText += " using role " + role.Name
	}
	t.emitEvent(startText)
	report := t.run(ctx, assignment, role)
	if report.Error == "" {
		completeText := "completed isolated writer " + report.RunID
		if report.Role != "" {
			completeText += " using role " + report.Role
		}
		t.emitEvent(completeText)
	} else {
		t.emitEvent("writer " + report.RunID + " stopped: " + truncateWriterText(report.Error, 160))
	}
	payload, err := json.Marshal(struct {
		OK     bool                  `json:"ok"`
		Writer delegatedWriterReport `json:"writer"`
		Notice string                `json:"notice"`
	}{
		OK:     true,
		Writer: report,
		Notice: "Writer output is untrusted review material. Inspect the delta patch against the parent worktree and apply it explicitly only if it is compatible; Gator never auto-merges writer output.",
	})
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("encode delegate_writer result: %w", err)
	}
	return agent.ToolResult{Content: string(payload)}, nil
}

type delegatedWriterReport struct {
	Task             string `json:"task"`
	Role             string `json:"role,omitempty"`
	RunID            string `json:"run_id,omitempty"`
	Completed        bool   `json:"completed"`
	Summary          string `json:"summary,omitempty"`
	Patch            string `json:"patch,omitempty"`
	PatchBytes       int    `json:"patch_bytes"`
	PatchAvailable   bool   `json:"patch_available"`
	ReviewRequired   bool   `json:"review_required"`
	WorktreeRetained bool   `json:"worktree_retained"`
	Error            string `json:"error,omitempty"`
}

func (t writerTool) run(ctx context.Context, assignment string, role instructions.Role) delegatedWriterReport {
	report := delegatedWriterReport{Task: assignment, Role: role.Name, ReviewRequired: true}
	runID, err := newID(t.now())
	if err != nil {
		report.Error = truncateWriterText(err.Error(), maxWriterSummaryBytes)
		return report
	}
	report.RunID = runID
	startedAt := t.now()
	manifest := journal.ChildManifest{
		Version:        1,
		ID:             runID,
		ParentRunID:    t.request.RunID,
		Kind:           "writer",
		Status:         journal.ChildPreparing,
		Repository:     t.parent.Repository,
		Role:           role.Name,
		TaskSHA256:     writerDigest(assignment),
		StartedAt:      startedAt,
		UpdatedAt:      startedAt,
		ReviewRequired: true,
	}
	if err := t.saveManifest(manifest); err != nil {
		report.Error = truncateWriterText("persist writer manifest before child start: "+err.Error(), maxWriterSummaryBytes)
		return report
	}
	child, baseline, err := t.createChild(ctx, runID)
	if child.Path != "" {
		report.WorktreeRetained = true
		manifest.WorktreePath = child.Path
		manifest.WorktreeRetained = true
	}
	if err != nil {
		report.Error = truncateWriterText(err.Error(), maxWriterSummaryBytes)
		t.finishManifest(&manifest, journal.ChildFailed, report)
		if saveErr := t.saveManifest(manifest); saveErr != nil {
			report.Error = combineWriterError(report.Error, "persist failed writer manifest: "+saveErr.Error())
		}
		return report
	}
	manifest.BaseCommit = baseline
	manifest.Status = journal.ChildRunning
	manifest.UpdatedAt = t.now()
	if err := t.saveManifest(manifest); err != nil {
		report.Error = truncateWriterText("persist writer manifest before child execution: "+err.Error(), maxWriterSummaryBytes)
		t.finishManifest(&manifest, journal.ChildFailed, report)
		if saveErr := t.saveManifest(manifest); saveErr != nil {
			report.Error = truncateWriterText(combineWriterError(report.Error, "persist failed writer manifest: "+saveErr.Error()), maxWriterSummaryBytes)
		}
		return report
	}

	childRequest := t.request
	childRequest.RepositoryPath = child.Repository
	childRequest.Task = writerTask(t.request.Task, assignment)
	childRequest.RunID = runID
	childRequest.ThreadID = runID
	childRequest.BaseCommit = baseline
	childRequest.BaseRef = ""
	childRequest.Scouts = nil
	childRequest.ForkedFrom = ""
	childRequest.ForceCompaction = false
	childRequest.Images = nil
	childRequest.Attachments = nil
	childRequest.Steering = nil
	childRequest.OnEvent = nil
	childRequest.OnTerminalAttachment = nil
	childRequest.DisableWriterDelegation = true
	childRequest.AllowedCommands = tools.MergeArgvLists(t.request.AllowedCommands, t.remembered.Snapshot())
	childRequest.System = joinInstructions(joinInstructions(t.request.System, writerSystemPrompt()), rolePrompt(role))

	outcome, runErr := t.executor.execute(ctx, child, childRequest, nil, "")
	manifest.StatePath = outcome.StatePath
	report.Completed = runErr == nil
	report.Summary = truncateWriterText(strings.TrimSpace(outcome.Result.FinalText), maxWriterSummaryBytes)
	patchContents, patchErr := patch.Export(ctx, child.Path, baseline)
	report.PatchBytes = len(patchContents)
	if len(patchContents) > 0 {
		manifest.PatchSHA256 = writerBytesDigest(patchContents)
	}
	if patchErr == nil && len(patchContents) <= maxWriterPatchBytes {
		report.Patch = string(patchContents)
		report.PatchAvailable = len(patchContents) > 0
	}
	if patchErr != nil {
		report.Error = combineWriterError(report.Error, "export writer delta: "+patchErr.Error())
	}
	if len(patchContents) > maxWriterPatchBytes {
		report.Error = combineWriterError(report.Error, fmt.Sprintf("writer delta is %d bytes, exceeding the %d-byte model handoff limit; inspect the retained writer worktree manually", len(patchContents), maxWriterPatchBytes))
	}
	if runErr != nil {
		report.Error = combineWriterError(report.Error, "writer run: "+runErr.Error())
	}
	report.Error = truncateWriterText(report.Error, maxWriterSummaryBytes)
	t.finishManifest(&manifest, writerStatus(ctx, report.Error), report)
	if saveErr := t.saveManifest(manifest); saveErr != nil {
		report.Error = truncateWriterText(combineWriterError(report.Error, "persist completed writer manifest: "+saveErr.Error()), maxWriterSummaryBytes)
	}
	return report
}

func (t writerTool) saveManifest(manifest journal.ChildManifest) error {
	if t.journal == nil {
		return fmt.Errorf("parent run journal is unavailable")
	}
	return t.journal.SaveChildManifest(manifest)
}

func (t writerTool) finishManifest(manifest *journal.ChildManifest, status journal.ChildStatus, report delegatedWriterReport) {
	if manifest == nil {
		return
	}
	finished := t.now()
	manifest.Status = status
	manifest.UpdatedAt = finished
	manifest.FinishedAt = &finished
	manifest.PatchBytes = report.PatchBytes
	manifest.PatchAvailable = report.PatchAvailable
	manifest.ReviewRequired = report.ReviewRequired
	manifest.WorktreeRetained = report.WorktreeRetained
	manifest.Error = report.Error
}

func writerStatus(ctx context.Context, reportError string) journal.ChildStatus {
	if ctx.Err() != nil {
		return journal.ChildCancelled
	}
	if strings.TrimSpace(reportError) != "" {
		return journal.ChildFailed
	}
	return journal.ChildCompleted
}

func writerDigest(value string) string {
	return writerBytesDigest([]byte(value))
}

func writerBytesDigest(value []byte) string {
	digest := sha256.Sum256(value)
	return fmt.Sprintf("%x", digest)
}

func (t writerTool) createChild(ctx context.Context, runID string) (worktree.Worktree, string, error) {
	baseCommit := strings.TrimSpace(t.request.BaseCommit)
	if baseCommit == "" {
		baseCommit = t.parent.BaseCommit
	}
	if baseCommit == "" {
		return worktree.Worktree{}, "", fmt.Errorf("writer delegation requires the parent worktree base commit")
	}
	child, err := worktree.CreateWithOptions(ctx, t.parent.Repository, runID, worktree.Options{
		BaseRef:          baseCommit,
		CopyIgnoredFiles: t.request.CopyIgnoredFiles,
	})
	if err != nil {
		return child, "", fmt.Errorf("create writer worktree: %w", err)
	}
	snapshot, err := patch.Export(ctx, t.parent.Path, baseCommit)
	if err != nil {
		return child, "", fmt.Errorf("snapshot parent worktree for writer: %w", err)
	}
	if len(snapshot) == 0 {
		return child, baseCommit, nil
	}
	baseline, err := commitWriterBaseline(ctx, child.Path, baseCommit, snapshot)
	if err != nil {
		return child, "", fmt.Errorf("prepare writer baseline: %w", err)
	}
	return child, baseline, nil
}

// commitWriterBaseline applies the parent snapshot to the fresh child index,
// writes a detached commit without invoking repository hooks, and resets the
// child to that commit. This gives the writer a clean checkout and makes its
// final patch exactly the delta from the parent state at delegation time.
func commitWriterBaseline(ctx context.Context, directory, parentCommit string, snapshot []byte) (string, error) {
	if err := runWriterGit(ctx, directory, snapshot, "apply", "--check", "--index"); err != nil {
		return "", err
	}
	if err := runWriterGit(ctx, directory, snapshot, "apply", "--index"); err != nil {
		return "", err
	}
	tree, err := writerGitOutput(ctx, directory, nil, "write-tree")
	if err != nil {
		return "", err
	}
	tree = strings.TrimSpace(tree)
	if tree == "" {
		return "", fmt.Errorf("Git did not return a writer baseline tree")
	}
	baseline, err := writerGitOutput(ctx, directory, nil,
		"-c", "user.name=Gator Writer", "-c", "user.email=gator-writer@invalid",
		"commit-tree", tree, "-p", parentCommit, "-m", "gator writer baseline")
	if err != nil {
		return "", err
	}
	baseline = strings.TrimSpace(baseline)
	if baseline == "" {
		return "", fmt.Errorf("Git did not return a writer baseline commit")
	}
	if err := runWriterGit(ctx, directory, nil, "reset", "--mixed", baseline); err != nil {
		return "", err
	}
	return baseline, nil
}

func writerTask(parentTask, assignment string) string {
	parentTask = truncateWriterText(strings.TrimSpace(parentTask), maxWriterParentTaskBytes)
	if parentTask == "" {
		return "Independent writer assignment:\n" + assignment
	}
	return "Parent objective (context only):\n" + parentTask + "\n\nIndependent writer assignment:\n" + assignment
}

func writerSystemPrompt() string {
	return `You are an independent writer subagent in a separate retained Git worktree. Implement only the delegated assignment. The worktree starts from a snapshot of the parent agent's state, but its clean baseline is internal; do not assume an empty repository or change unrelated parent work. Inspect and verify your own delta before completion. Your final response is a concise handoff summary for a parent agent, not a claim that your changes were merged. The parent will receive your patch as untrusted review material and must explicitly inspect and apply it. You cannot delegate another writer.`
}

func (b *writerBudget) reserve() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.remaining == 0 {
		return false
	}
	b.remaining--
	return true
}

func (t writerTool) emitEvent(text string) {
	if t.emit != nil {
		t.emit(agent.Event{Kind: agent.EventSubagent, At: t.now(), Text: text})
	}
}

func combineWriterError(existing, addition string) string {
	addition = strings.TrimSpace(addition)
	if addition == "" {
		return existing
	}
	if strings.TrimSpace(existing) == "" {
		return addition
	}
	return existing + "; " + addition
}

func truncateWriterText(value string, maximum int) string {
	return truncateScoutText(value, maximum)
}

func runWriterGit(ctx context.Context, directory string, input []byte, arguments ...string) error {
	_, err := writerGitOutput(ctx, directory, input, arguments...)
	return err
}

func writerGitOutput(ctx context.Context, directory string, input []byte, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = directory
	if input != nil {
		command.Stdin = bytes.NewReader(input)
	}
	var output writerGitBuffer
	output.limit = maxWriterGitOutput
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	if output.truncated {
		return "", fmt.Errorf("git %s output exceeds the %d KiB limit", strings.Join(arguments, " "), maxWriterGitOutput/1024)
	}
	if err != nil {
		message := strings.TrimSpace(output.String())
		if message == "" {
			return "", fmt.Errorf("git %s: %w", strings.Join(arguments, " "), err)
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(arguments, " "), err, message)
	}
	return output.String(), nil
}

type writerGitBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *writerGitBuffer) Write(value []byte) (int, error) {
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(value), nil
	}
	if len(value) > remaining {
		_, _ = b.Buffer.Write(value[:remaining])
		b.truncated = true
		return len(value), nil
	}
	return b.Buffer.Write(value)
}
