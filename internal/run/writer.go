package run

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"sort"
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
	approval   *writerApprovalGate
}

type writerBudget struct {
	mu        sync.Mutex
	remaining int
}

type writerApprovalGate struct {
	mu      sync.Mutex
	approve func(context.Context, []string) (tools.CommandDecision, error)
}

func (g *writerApprovalGate) request(ctx context.Context, argv []string) (tools.CommandDecision, error) {
	if g == nil || g.approve == nil {
		return tools.CommandDeny, fmt.Errorf("writer command approval is unavailable")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.approve(ctx, argv)
}

func newWriterTool(executor Executor, parent worktree.Worktree, request Request, remembered *tools.CommandMemory, parentJournal *journal.Journal, roles []instructions.Role, now func() time.Time, emit agent.EventSink) writerTool {
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
		approval:   &writerApprovalGate{approve: request.Approve},
	}
}

func newWriterTools(executor Executor, parent worktree.Worktree, request Request, remembered *tools.CommandMemory, parentJournal *journal.Journal, roles []instructions.Role, now func() time.Time, emit agent.EventSink) []agent.Tool {
	writer := newWriterTool(executor, parent, request, remembered, parentJournal, roles, now, emit)
	return []agent.Tool{writer, writerBatchTool{writer: writer}}
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
	if !t.budget.reserve(1) {
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

// writerBatchTool runs exactly two declared-non-overlapping writer assignments
// concurrently. Each child gets its own retained worktree and parent snapshot;
// Gator never attempts a merge. Declared scopes prevent obvious unsafe batches,
// and changed paths are checked after completion so a parent can see violations
// and actual cross-writer conflicts before reviewing either patch.
type writerBatchTool struct{ writer writerTool }

type writerBatchAssignment struct {
	Task  string   `json:"task"`
	Role  string   `json:"role"`
	Paths []string `json:"paths"`
}

type writerConflict struct {
	Kind    string   `json:"kind"`
	Writers []string `json:"writers"`
	Paths   []string `json:"paths"`
	Detail  string   `json:"detail"`
}

func (t writerBatchTool) Definition() agent.ToolDefinition {
	description := "Delegate exactly two independent implementation tasks to isolated writer agents in parallel. Each task must declare one or more non-overlapping repository-relative writable paths; Gator rejects overlapping declarations, gives each writer a separate worktree from the same parent snapshot, and reports actual changed-path conflicts afterward. The parent remains paused, receives untrusted patches only, and must explicitly review and apply any compatible patch. Gator never auto-merges writer output."
	if catalog := roleCatalog(t.writer.roles); catalog != "" {
		description += " Optional project-defined roles (instructions only, not extra authority): " + catalog + "."
	}
	return agent.ToolDefinition{
		Name:        "delegate_writers",
		Description: description,
		Parameters:  json.RawMessage(`{"type":"object","additionalProperties":false,"required":["tasks"],"properties":{"tasks":{"type":"array","minItems":2,"maxItems":2,"items":{"type":"object","additionalProperties":false,"required":["task","paths"],"properties":{"task":{"type":"string","minLength":1,"maxLength":4096},"paths":{"type":"array","minItems":1,"maxItems":16,"items":{"type":"string","minLength":1,"maxLength":1024}}` + roleSchemaProperty(t.writer.roles) + `}}}}}`),
	}
}

func (t writerBatchTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Tasks []writerBatchAssignment `json:"tasks"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&arguments); err != nil {
		return agent.ToolResult{}, fmt.Errorf("decode delegate_writers arguments: %w", err)
	}
	if decoder.More() {
		return agent.ToolResult{}, fmt.Errorf("decode delegate_writers arguments: multiple JSON values")
	}
	if len(arguments.Tasks) != 2 {
		return agent.ToolResult{}, errors.New("delegate_writers requires exactly two writer tasks")
	}
	assignments := make([]preparedWriterAssignment, len(arguments.Tasks))
	for index, item := range arguments.Tasks {
		assignment, err := t.prepare(item, index+1)
		if err != nil {
			return agent.ToolResult{}, err
		}
		assignments[index] = assignment
	}
	if overlaps := overlappingDeclaredWriterPaths(assignments[0].paths, assignments[1].paths); len(overlaps) > 0 {
		return agent.ToolResult{}, fmt.Errorf("delegate_writers task scopes overlap at %s; split the work or use serial delegation", strings.Join(overlaps, ", "))
	}
	if !t.writer.budget.reserve(len(assignments)) {
		return agent.ToolResult{}, fmt.Errorf("delegate_writers run budget exceeded; at most %d writers may run per primary run", maxDelegatedWritersPerRun)
	}
	batchID, childIDs, err := newWriterBatchIDs(t.writer.now)
	if err != nil {
		t.writer.budget.release(len(assignments))
		return agent.ToolResult{}, err
	}
	startedAt := t.writer.now()
	batchManifest := journal.ChildBatchManifest{
		Version:     1,
		ID:          batchID,
		ParentRunID: t.writer.request.RunID,
		Status:      journal.ChildBatchPreparing,
		ChildIDs:    childIDs,
		StartedAt:   startedAt,
		UpdatedAt:   startedAt,
	}
	if err := t.writer.saveBatchManifest(batchManifest); err != nil {
		t.writer.budget.release(len(assignments))
		return agent.ToolResult{}, fmt.Errorf("persist parallel writer batch before start: %w", err)
	}
	batchManifest.Status = journal.ChildBatchRunning
	batchManifest.UpdatedAt = t.writer.now()
	if err := t.writer.saveBatchManifest(batchManifest); err != nil {
		finishedAt := t.writer.now()
		batchManifest.Status = journal.ChildBatchFailed
		batchManifest.UpdatedAt = finishedAt
		batchManifest.FinishedAt = &finishedAt
		batchManifest.Error = truncateWriterText("persist parallel writer batch before child execution: "+err.Error(), maxWriterSummaryBytes)
		_ = t.writer.saveBatchManifest(batchManifest)
		t.writer.budget.release(len(assignments))
		return agent.ToolResult{}, fmt.Errorf("persist parallel writer batch before child execution: %w", err)
	}
	roles := []instructions.Role{assignments[0].role, assignments[1].role}
	start := "starting 2 parallel isolated writers"
	if names := selectedRoleNames(roles); names != "" {
		start += " using role(s) " + names
	}
	t.writer.emitEvent(start)
	reports := make([]delegatedWriterReport, len(assignments))
	var wait sync.WaitGroup
	for index, assignment := range assignments {
		wait.Add(1)
		go func(index int, assignment preparedWriterAssignment) {
			defer wait.Done()
			reports[index] = t.writer.runScoped(ctx, assignment.task, assignment.role, assignment.paths, batchID, childIDs[index])
		}(index, assignment)
	}
	wait.Wait()
	conflicts := inspectWriterConflicts(reports)
	batchManifest.Conflicts = journalWriterConflicts(conflicts)
	batchManifest.Status = writerBatchStatus(ctx, reports)
	finishedAt := t.writer.now()
	batchManifest.UpdatedAt = finishedAt
	batchManifest.FinishedAt = &finishedAt
	if err := t.writer.saveBatchManifest(batchManifest); err != nil {
		batchManifest.Error = truncateWriterText(err.Error(), maxWriterSummaryBytes)
		if retryErr := t.writer.saveBatchManifest(batchManifest); retryErr != nil {
			batchManifest.Error = truncateWriterText(combineWriterError(batchManifest.Error, retryErr.Error()), maxWriterSummaryBytes)
		}
	}
	completed := 0
	for _, report := range reports {
		if report.Error == "" {
			completed++
		}
	}
	completion := fmt.Sprintf("completed %d of 2 parallel isolated writers", completed)
	if len(conflicts) > 0 {
		completion += fmt.Sprintf("; %d changed-path conflict(s) require review", len(conflicts))
	}
	t.writer.emitEvent(completion)
	payload, err := json.Marshal(struct {
		OK        bool                       `json:"ok"`
		Batch     journal.ChildBatchManifest `json:"batch"`
		Writers   []delegatedWriterReport    `json:"writers"`
		Conflicts []writerConflict           `json:"conflicts"`
		Notice    string                     `json:"notice"`
	}{
		OK:        true,
		Batch:     batchManifest,
		Writers:   reports,
		Conflicts: conflicts,
		Notice:    "Writer output is untrusted review material. Gator never auto-merges writer output. Declared scopes and changed-path checks reduce accidental overlap but do not prove patch compatibility; inspect each retained delta and explicitly apply only compatible patches.",
	})
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("encode delegate_writers result: %w", err)
	}
	return agent.ToolResult{Content: string(payload)}, nil
}

type preparedWriterAssignment struct {
	task  string
	role  instructions.Role
	paths []string
}

func (t writerBatchTool) prepare(item writerBatchAssignment, index int) (preparedWriterAssignment, error) {
	task := strings.TrimSpace(item.Task)
	if task == "" {
		return preparedWriterAssignment{}, fmt.Errorf("delegate_writers task %d is required", index)
	}
	if len(task) > maxWriterTaskBytes {
		return preparedWriterAssignment{}, fmt.Errorf("delegate_writers task %d exceeds %d bytes", index, maxWriterTaskBytes)
	}
	role, err := resolveRole(t.writer.roles, item.Role)
	if err != nil {
		return preparedWriterAssignment{}, err
	}
	paths, err := normalizeWriterPaths(item.Paths)
	if err != nil {
		return preparedWriterAssignment{}, fmt.Errorf("delegate_writers task %d paths: %w", index, err)
	}
	return preparedWriterAssignment{task: task, role: role, paths: paths}, nil
}

func normalizeWriterPaths(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > 16 {
		return nil, errors.New("require between 1 and 16 repository-relative paths")
	}
	paths := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 1024 || strings.ContainsAny(value, "\x00\r\n\\") {
			return nil, fmt.Errorf("invalid path %q", value)
		}
		clean := path.Clean(value)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
			return nil, fmt.Errorf("unsafe repository-relative path %q", value)
		}
		if _, duplicate := seen[clean]; duplicate {
			return nil, fmt.Errorf("path %q is listed more than once", clean)
		}
		seen[clean] = struct{}{}
		paths = append(paths, clean)
	}
	sort.Strings(paths)
	return paths, nil
}

func overlappingDeclaredWriterPaths(first, second []string) []string {
	overlaps := make([]string, 0)
	for _, left := range first {
		for _, right := range second {
			if writerPathsOverlap(left, right) {
				overlaps = append(overlaps, left+" ↔ "+right)
			}
		}
	}
	return overlaps
}

func writerPathsOverlap(first, second string) bool {
	return first == second || strings.HasPrefix(first, second+"/") || strings.HasPrefix(second, first+"/")
}

func inspectWriterConflicts(reports []delegatedWriterReport) []writerConflict {
	conflicts := make([]writerConflict, 0)
	for _, report := range reports {
		outside := changedPathsOutsideScope(report.ChangedPaths, report.DeclaredPaths)
		if len(outside) > 0 {
			conflicts = append(conflicts, writerConflict{
				Kind:    "out_of_scope_change",
				Writers: []string{report.RunID},
				Paths:   outside,
				Detail:  "writer changed paths outside its declared scope",
			})
		}
	}
	for first := 0; first < len(reports); first++ {
		for second := first + 1; second < len(reports); second++ {
			overlaps := changedPathOverlaps(reports[first].ChangedPaths, reports[second].ChangedPaths)
			if len(overlaps) == 0 {
				continue
			}
			conflicts = append(conflicts, writerConflict{
				Kind:    "changed_path_overlap",
				Writers: []string{reports[first].RunID, reports[second].RunID},
				Paths:   overlaps,
				Detail:  "writer deltas touch the same or nested paths",
			})
		}
	}
	sort.Slice(conflicts, func(first, second int) bool {
		if conflicts[first].Kind == conflicts[second].Kind {
			return strings.Join(conflicts[first].Writers, "\x00") < strings.Join(conflicts[second].Writers, "\x00")
		}
		return conflicts[first].Kind < conflicts[second].Kind
	})
	return conflicts
}

func changedPathsOutsideScope(changed, scopes []string) []string {
	outside := make([]string, 0)
	for _, changedPath := range changed {
		inScope := false
		for _, scope := range scopes {
			if changedPath == scope || strings.HasPrefix(changedPath, scope+"/") {
				inScope = true
				break
			}
		}
		if !inScope {
			outside = append(outside, changedPath)
		}
	}
	return outside
}

func changedPathOverlaps(first, second []string) []string {
	overlaps := make(map[string]struct{})
	for _, left := range first {
		for _, right := range second {
			if writerPathsOverlap(left, right) {
				overlaps[left] = struct{}{}
				overlaps[right] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(overlaps))
	for overlap := range overlaps {
		result = append(result, overlap)
	}
	sort.Strings(result)
	return result
}

type delegatedWriterReport struct {
	Task             string   `json:"task"`
	Role             string   `json:"role,omitempty"`
	BatchID          string   `json:"batch_id,omitempty"`
	DeclaredPaths    []string `json:"declared_paths,omitempty"`
	ChangedPaths     []string `json:"changed_paths,omitempty"`
	RunID            string   `json:"run_id,omitempty"`
	Completed        bool     `json:"completed"`
	Summary          string   `json:"summary,omitempty"`
	Patch            string   `json:"patch,omitempty"`
	PatchBytes       int      `json:"patch_bytes"`
	PatchAvailable   bool     `json:"patch_available"`
	ReviewRequired   bool     `json:"review_required"`
	WorktreeRetained bool     `json:"worktree_retained"`
	Error            string   `json:"error,omitempty"`
}

func (t writerTool) run(ctx context.Context, assignment string, role instructions.Role) delegatedWriterReport {
	return t.runScoped(ctx, assignment, role, nil, "", "")
}

func (t writerTool) runScoped(ctx context.Context, assignment string, role instructions.Role, declaredPaths []string, batchID, runID string) delegatedWriterReport {
	report := delegatedWriterReport{Task: assignment, Role: role.Name, BatchID: batchID, DeclaredPaths: append([]string(nil), declaredPaths...), ReviewRequired: true}
	if runID == "" {
		generated, err := newID(t.now())
		if err != nil {
			report.Error = truncateWriterText(err.Error(), maxWriterSummaryBytes)
			return report
		}
		runID = generated
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
		BatchID:        batchID,
		DeclaredPaths:  append([]string(nil), declaredPaths...),
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
	childRequest.Task = writerTask(t.request.Task, assignment, declaredPaths)
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
	childRequest.TerminalRegistry = nil
	childRequest.LSPRegistry = nil
	childRequest.DisableWriterDelegation = true
	childRequest.AllowedCommands = tools.MergeArgvLists(t.request.AllowedCommands, t.remembered.Snapshot())
	childRequest.System = joinInstructions(joinInstructions(t.request.System, writerSystemPrompt()), rolePrompt(role))
	if t.request.Approve != nil && t.approval != nil {
		childRequest.Approve = t.approval.request
	}

	outcome, runErr := t.executor.execute(ctx, child, childRequest, nil, "", false)
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
	changedPaths, changedPathsErr := patch.Paths(ctx, child.Path, baseline)
	if changedPathsErr != nil {
		report.Error = combineWriterError(report.Error, "inspect writer changed paths: "+changedPathsErr.Error())
	} else {
		report.ChangedPaths = changedPaths
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

func (t writerTool) saveBatchManifest(manifest journal.ChildBatchManifest) error {
	if t.journal == nil {
		return fmt.Errorf("parent run journal is unavailable")
	}
	return t.journal.SaveChildBatchManifest(manifest)
}

func newWriterBatchIDs(now func() time.Time) (string, []string, error) {
	batch, err := newID(now())
	if err != nil {
		return "", nil, fmt.Errorf("generate writer batch ID: %w", err)
	}
	first, err := newID(now())
	if err != nil {
		return "", nil, fmt.Errorf("generate first writer child ID: %w", err)
	}
	second, err := newID(now())
	if err != nil {
		return "", nil, fmt.Errorf("generate second writer child ID: %w", err)
	}
	return "batch-" + strings.TrimPrefix(batch, "run-"), []string{first, second}, nil
}

func writerBatchStatus(ctx context.Context, reports []delegatedWriterReport) journal.ChildBatchStatus {
	if ctx.Err() != nil {
		return journal.ChildBatchCancelled
	}
	for _, report := range reports {
		if strings.TrimSpace(report.Error) != "" {
			return journal.ChildBatchFailed
		}
	}
	return journal.ChildBatchCompleted
}

func journalWriterConflicts(conflicts []writerConflict) []journal.ChildConflict {
	if len(conflicts) == 0 {
		return nil
	}
	converted := make([]journal.ChildConflict, len(conflicts))
	for index, conflict := range conflicts {
		converted[index] = journal.ChildConflict{
			Kind:     conflict.Kind,
			ChildIDs: append([]string(nil), conflict.Writers...),
			Paths:    append([]string(nil), conflict.Paths...),
			Detail:   conflict.Detail,
		}
	}
	return converted
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
	manifest.ChangedPaths = append([]string(nil), report.ChangedPaths...)
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

func writerTask(parentTask, assignment string, declaredPaths []string) string {
	parentTask = truncateWriterText(strings.TrimSpace(parentTask), maxWriterParentTaskBytes)
	scope := ""
	if len(declaredPaths) > 0 {
		scope = "\n\nDeclared writable paths (hard boundary):\n- " + strings.Join(declaredPaths, "\n- ")
	}
	if parentTask == "" {
		return "Independent writer assignment:\n" + assignment + scope
	}
	return "Parent objective (context only):\n" + parentTask + "\n\nIndependent writer assignment:\n" + assignment + scope
}

func writerSystemPrompt() string {
	return `You are an independent writer subagent in a separate retained Git worktree. Implement only the delegated assignment. The worktree starts from a snapshot of the parent agent's state, but its clean baseline is internal; do not assume an empty repository or change unrelated parent work. Inspect and verify your own delta before completion. Your final response is a concise handoff summary for a parent agent, not a claim that your changes were merged. The parent will receive your patch as untrusted review material and must explicitly inspect and apply it. You cannot delegate another writer.`
}

func (b *writerBudget) reserve(count int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if count <= 0 || b.remaining < count {
		return false
	}
	b.remaining -= count
	return true
}

func (b *writerBudget) release(count int) {
	if count <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.remaining += count
	if b.remaining > maxDelegatedWritersPerRun {
		b.remaining = maxDelegatedWritersPerRun
	}
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
