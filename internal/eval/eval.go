// Package eval runs a bounded, reproducible Gator evaluation against a fixture
// repository. It records a report; it does not claim SWE-bench or model
// competitiveness.
package eval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workspace"
)

const (
	specVersion                = 1
	suiteVersion               = 1
	reportVersion              = 2
	maxSuiteCases              = 64
	maxScoreCommands           = 8
	defaultScoreTimeoutSeconds = 120
)

var (
	identifierPattern    = regexp.MustCompile(`\A[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}\z`)
	environmentIDPattern = regexp.MustCompile(`\A(?:[^\s@]+@)?sha256:[a-f0-9]{64}\z`)
)

// Spec is the fixed evaluation contract: task, model budget, tool policy, and
// verifier. Callers must use a unique RunID for every attempt so reports never
// collide with a previous prediction.
type Spec struct {
	Version        int        `json:"version"`
	ID             string     `json:"id"`
	Task           string     `json:"task"`
	MaxSteps       int        `json:"max_steps"`
	TimeoutSeconds int        `json:"timeout_seconds"`
	Verify         [][]string `json:"verify"`
	Sandbox        string     `json:"sandbox,omitempty"`
	Network        string     `json:"network,omitempty"`
	Repository     string     `json:"repository,omitempty"`
	// Scopes select repository-relative guidance in the fixture. They do not
	// grant access outside the isolated evaluation worktree.
	Scopes []string `json:"scopes,omitempty"`
	// AllowedCommands and AllowedCommandPrefixes are the only exploratory
	// commands a noninteractive evaluation can approve. Verification commands
	// remain separately required and are always exposed to the agent.
	AllowedCommands        [][]string `json:"allowed_commands,omitempty"`
	AllowedCommandPrefixes [][]string `json:"allowed_command_prefixes,omitempty"`
	// Setup contains trusted fixture-author commands that run once before the
	// agent starts. It is never model supplied.
	Setup [][]string `json:"setup,omitempty"`
	// Score contains trusted, post-run oracle commands. They are never exposed
	// to the model. {{fixture}} and {{worktree}} expand only for the scorer.
	Score               [][]string `json:"score,omitempty"`
	ScoreTimeoutSeconds int        `json:"score_timeout_seconds,omitempty"`
	// Labels let suites publish stratified results without inferring categories
	// from an issue title after the fact.
	Labels []string `json:"labels,omitempty"`
}

// Report is the durable outcome of one evaluation attempt.
type Report struct {
	Version                int           `json:"version"`
	ID                     string        `json:"id"`
	RunID                  string        `json:"run_id"`
	SuiteID                string        `json:"suite_id,omitempty"`
	Attempt                int           `json:"attempt,omitempty"`
	Status                 string        `json:"status"`
	AgentStatus            string        `json:"agent_status"`
	Task                   string        `json:"task"`
	Provider               string        `json:"provider"`
	Model                  string        `json:"model"`
	MaxSteps               int           `json:"max_steps"`
	Steps                  int           `json:"steps"`
	TimeoutSeconds         int           `json:"timeout_seconds"`
	Verify                 [][]string    `json:"verify"`
	Sandbox                string        `json:"sandbox"`
	Network                string        `json:"network"`
	BaseCommit             string        `json:"base_commit,omitempty"`
	FixtureSHA256          string        `json:"fixture_sha256,omitempty"`
	HarnessVersion         string        `json:"harness_version,omitempty"`
	HarnessCommit          string        `json:"harness_commit,omitempty"`
	EnvironmentID          string        `json:"environment_id,omitempty"`
	ProviderEndpointSHA256 string        `json:"provider_endpoint_sha256,omitempty"`
	Scopes                 []string      `json:"scopes,omitempty"`
	AllowedCommands        [][]string    `json:"allowed_commands,omitempty"`
	AllowedCommandPrefixes [][]string    `json:"allowed_command_prefixes,omitempty"`
	Setup                  [][]string    `json:"setup,omitempty"`
	Score                  [][]string    `json:"score,omitempty"`
	ScoreStatus            string        `json:"score_status"`
	ScoreResults           []CheckResult `json:"score_results,omitempty"`
	Labels                 []string      `json:"labels,omitempty"`
	Duration               time.Duration `json:"duration_ns"`
	Error                  string        `json:"error,omitempty"`
	FinalText              string        `json:"final_text,omitempty"`
	StatePath              string        `json:"state_path,omitempty"`
	WorktreePath           string        `json:"worktree_path,omitempty"`
	StartedAt              time.Time     `json:"started_at"`
}

// CheckResult is bounded post-run scoring evidence. Its output is retained
// because a failing oracle without its diagnostic is not independently useful.
type CheckResult struct {
	Argv      []string `json:"argv"`
	ExitCode  int      `json:"exit_code"`
	Output    string   `json:"output,omitempty"`
	Truncated bool     `json:"truncated,omitempty"`
	TimedOut  bool     `json:"timed_out,omitempty"`
}

// Suite is a versioned, serial collection of fixture-relative case
// directories. A suite is deliberately only a manifest: scheduling and
// container lifecycle remain the caller's responsibility.
type Suite struct {
	Version int      `json:"version"`
	ID      string   `json:"id"`
	Cases   []string `json:"cases"`
}

// SuiteReport aggregates durable reports from one suite invocation.
type SuiteReport struct {
	Version       int           `json:"version"`
	ID            string        `json:"id"`
	RunID         string        `json:"run_id"`
	Status        string        `json:"status"`
	Provider      string        `json:"provider"`
	Model         string        `json:"model"`
	StartedAt     time.Time     `json:"started_at"`
	Duration      time.Duration `json:"duration_ns"`
	Resolved      int           `json:"resolved"`
	Unresolved    int           `json:"unresolved"`
	Errors        int           `json:"errors"`
	ScorePassed   int           `json:"score_passed"`
	ScoreFailed   int           `json:"score_failed"`
	ScoreErrors   int           `json:"score_errors"`
	Unscored      int           `json:"unscored"`
	Attempts      int           `json:"attempts"`
	Cases         []Report      `json:"cases"`
	CaseSummaries []CaseSummary `json:"case_summaries"`
}

// CaseSummary aggregates repeated trials of one fixture without erasing the
// attempt-level records needed to investigate model variance.
type CaseSummary struct {
	ID          string `json:"id"`
	Attempts    int    `json:"attempts"`
	Resolved    int    `json:"resolved"`
	Unresolved  int    `json:"unresolved"`
	Errors      int    `json:"errors"`
	ScorePassed int    `json:"score_passed"`
	ScoreFailed int    `json:"score_failed"`
	ScoreErrors int    `json:"score_errors"`
	Unscored    int    `json:"unscored"`
}

// Options configures one evaluation. Model is required; live providers are
// injected by the command layer so this package never reads credentials.
type Options struct {
	Spec             Spec
	RunID            string
	Provider         string
	ModelName        string
	StateDir         string
	Repository       string
	Executor         gatorrun.Executor
	FixturePath      string
	FixtureSHA256    string
	HarnessVersion   string
	HarnessCommit    string
	EnvironmentID    string
	ProviderEndpoint string
	SuiteID          string
	Attempt          int
	Now              func() time.Time
}

// LoadSpec reads eval.json from directory.
func LoadSpec(directory string) (Spec, error) {
	contents, err := os.ReadFile(filepath.Join(directory, "eval.json"))
	if err != nil {
		return Spec{}, fmt.Errorf("read eval spec: %w", err)
	}
	var spec Spec
	if err := json.Unmarshal(contents, &spec); err != nil {
		return Spec{}, fmt.Errorf("decode eval spec: %w", err)
	}
	if spec.Version != specVersion {
		return Spec{}, fmt.Errorf("unsupported eval spec version %d", spec.Version)
	}
	if spec.TimeoutSeconds < 1 {
		spec.TimeoutSeconds = 120
	}
	if len(spec.Score) > 0 && spec.ScoreTimeoutSeconds < 1 {
		spec.ScoreTimeoutSeconds = defaultScoreTimeoutSeconds
	}
	if err := validateSpec(spec); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

// LoadSuite reads suite.json from directory.
func LoadSuite(directory string) (Suite, error) {
	contents, err := os.ReadFile(filepath.Join(directory, "suite.json"))
	if err != nil {
		return Suite{}, fmt.Errorf("read eval suite: %w", err)
	}
	var suite Suite
	if err := json.Unmarshal(contents, &suite); err != nil {
		return Suite{}, fmt.Errorf("decode eval suite: %w", err)
	}
	if suite.Version != suiteVersion {
		return Suite{}, fmt.Errorf("unsupported eval suite version %d", suite.Version)
	}
	if !validIdentifier(suite.ID) {
		return Suite{}, errors.New("eval suite id must be a portable run identifier")
	}
	if len(suite.Cases) == 0 || len(suite.Cases) > maxSuiteCases {
		return Suite{}, fmt.Errorf("eval suite must contain between 1 and %d cases", maxSuiteCases)
	}
	seen := make(map[string]struct{}, len(suite.Cases))
	for _, fixture := range suite.Cases {
		if !validFixturePath(fixture) {
			return Suite{}, fmt.Errorf("eval suite case %q must be a relative fixture directory", fixture)
		}
		clean := filepath.Clean(fixture)
		if _, exists := seen[clean]; exists {
			return Suite{}, fmt.Errorf("eval suite repeats case %q", fixture)
		}
		seen[clean] = struct{}{}
	}
	return suite, nil
}

// LoadScript reads optional script.json turns for a deterministic offline eval.
func LoadScript(directory string) ([]agent.Turn, error) {
	return LoadTurns(filepath.Join(directory, "script.json"))
}

// LoadTurns reads scripted agent turns from path. Missing files are empty.
func LoadTurns(path string) ([]agent.Turn, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read eval script: %w", err)
	}
	var turns []agent.Turn
	if err := json.Unmarshal(contents, &turns); err != nil {
		return nil, fmt.Errorf("decode eval script: %w", err)
	}
	if len(turns) == 0 {
		return nil, errors.New("eval script is empty")
	}
	return turns, nil
}

// CopyRepository copies a regular-file fixture tree into dest. It does not
// carry Git metadata, symlinks, device files, or named pipes into an eval.
func CopyRepository(source, dest string) error {
	return copyDir(source, dest)
}

// FixtureSHA256 returns a stable digest of every regular fixture file except
// Git metadata. It gives results a content identity independent of the fresh
// baseline commit that PrepareRepository creates for each attempt.
func FixtureSHA256(directory string) (string, error) {
	hash := sha256.New()
	if err := digestFixtureDirectory(hash, directory, "."); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// PrepareRepository makes a private, freshly initialized Git baseline from a
// read-only fixture tree. The returned path and its retained worktrees are
// intentionally left for report-driven inspection; callers own later cleanup.
func PrepareRepository(ctx context.Context, source string) (string, error) {
	workspace, err := os.MkdirTemp("", "gator-eval-repository-")
	if err != nil {
		return "", fmt.Errorf("create eval repository directory: %w", err)
	}
	fail := func(err error) (string, error) {
		_ = os.RemoveAll(workspace)
		return "", err
	}
	repository := filepath.Join(workspace, "repository")
	if err := CopyRepository(source, repository); err != nil {
		return fail(fmt.Errorf("copy eval repository: %w", err))
	}
	for _, argv := range [][]string{
		{"git", "init", "--quiet"},
		{"git", "add", "--all"},
		{"git", "-c", "core.hooksPath=/dev/null", "-c", "user.name=Gator Eval", "-c", "user.email=gator-eval@example.invalid", "commit", "--quiet", "--no-gpg-sign", "-m", "gator evaluation baseline"},
	} {
		command := exec.CommandContext(ctx, argv[0], argv[1:]...)
		command.Dir = repository
		output, err := command.CombinedOutput()
		if err != nil {
			return fail(fmt.Errorf("create eval Git baseline: %s: %w: %s", strings.Join(argv, " "), err, strings.TrimSpace(string(output))))
		}
	}
	return repository, nil
}

// ValidateRunID confirms that a run identifier can safely become a worktree
// and journal path before fixture preparation creates any local artifacts.
func ValidateRunID(runID string) error {
	if strings.TrimSpace(runID) == "" {
		return errors.New("eval run_id is required")
	}
	if !validIdentifier(runID) {
		return fmt.Errorf("invalid eval run_id %q", runID)
	}
	return nil
}

// ValidateEnvironmentID requires a content-addressed container identity for a
// live suite. A mutable image tag does not establish a reproducible runtime.
func ValidateEnvironmentID(environmentID string) error {
	if !environmentIDPattern.MatchString(strings.TrimSpace(environmentID)) {
		return errors.New("eval environment_id must be an OCI-style sha256 digest")
	}
	return nil
}

// Run executes one evaluation attempt. Status is resolved, unresolved, or
// error. Re-using RunID against a previous report file is the caller's
// mistake; this function always executes.
func Run(ctx context.Context, options Options) (Report, error) {
	if len(options.Spec.Score) > 0 && options.Spec.ScoreTimeoutSeconds < 1 {
		options.Spec.ScoreTimeoutSeconds = defaultScoreTimeoutSeconds
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	started := now()
	report := Report{
		Version:                reportVersion,
		ID:                     options.Spec.ID,
		RunID:                  options.RunID,
		SuiteID:                options.SuiteID,
		Attempt:                options.Attempt,
		Task:                   options.Spec.Task,
		Provider:               options.Provider,
		Model:                  options.ModelName,
		MaxSteps:               options.Spec.MaxSteps,
		TimeoutSeconds:         options.Spec.TimeoutSeconds,
		Verify:                 options.Spec.Verify,
		Sandbox:                string(options.Executor.Sandbox.Normalize().Mode),
		Network:                string(options.Executor.Sandbox.Normalize().Network),
		FixtureSHA256:          options.FixtureSHA256,
		HarnessVersion:         options.HarnessVersion,
		HarnessCommit:          options.HarnessCommit,
		EnvironmentID:          options.EnvironmentID,
		ProviderEndpointSHA256: stringSHA256(options.ProviderEndpoint),
		Scopes:                 cloneStrings(options.Spec.Scopes),
		AllowedCommands:        cloneArgvLists(options.Spec.AllowedCommands),
		AllowedCommandPrefixes: cloneArgvLists(options.Spec.AllowedCommandPrefixes),
		Setup:                  cloneArgvLists(options.Spec.Setup),
		Score:                  cloneArgvLists(options.Spec.Score),
		ScoreStatus:            "not_requested",
		Labels:                 cloneStrings(options.Spec.Labels),
		StartedAt:              started,
		Status:                 "error",
	}
	if err := ValidateRunID(options.RunID); err != nil {
		return report, err
	}
	if err := validateSpec(options.Spec); err != nil {
		return report, fmt.Errorf("validate eval spec: %w", err)
	}
	if strings.TrimSpace(options.Repository) == "" {
		return report, errors.New("eval repository is required")
	}
	timeout := time.Duration(options.Spec.TimeoutSeconds) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	outcome, err := options.Executor.Execute(runCtx, gatorrun.Request{
		RepositoryPath:          options.Repository,
		Task:                    options.Spec.Task,
		Provider:                options.Provider,
		Model:                   options.ModelName,
		RunID:                   options.RunID,
		MaxSteps:                options.Spec.MaxSteps,
		Verification:            options.Spec.Verify,
		Scopes:                  options.Spec.Scopes,
		AllowedCommands:         options.Spec.AllowedCommands,
		AllowedCommandPrefixes:  options.Spec.AllowedCommandPrefixes,
		StateDir:                options.StateDir,
		Setup:                   options.Spec.Setup,
		DisableWriterDelegation: true,
		Mode:                    gatorrun.ExecuteMode,
	})
	report.StatePath = outcome.StatePath
	report.WorktreePath = outcome.Worktree.Path
	report.BaseCommit = outcome.Worktree.BaseCommit
	report.Steps = outcome.Result.Steps
	report.FinalText = outcome.Result.FinalText
	agentStatus := "resolved"
	if err != nil {
		report.Error = err.Error()
		if runCtx.Err() != nil {
			agentStatus = "error"
			report.Error = "eval exceeded timeout_seconds: " + err.Error()
		} else {
			agentStatus = "unresolved"
		}
	}
	report.AgentStatus = agentStatus
	report.Status = agentStatus
	if len(options.Spec.Score) > 0 {
		if outcome.Worktree.Path == "" {
			report.ScoreStatus = "error"
			report.Status = "error"
			report.Error = joinErrors(report.Error, "post-run scorer has no retained worktree")
		} else {
			results, scoreStatus, scoreErr := runScore(ctx, outcome.Worktree.Path, options.FixturePath, options.Spec)
			report.ScoreResults = results
			report.ScoreStatus = scoreStatus
			if scoreErr != nil {
				report.Status = "error"
				report.Error = joinErrors(report.Error, "post-run scorer: "+scoreErr.Error())
			} else if scoreStatus != "passed" && report.Status == "resolved" {
				report.Status = "unresolved"
				report.Error = joinErrors(report.Error, "post-run scorer did not pass")
			}
		}
	}
	report.Duration = now().Sub(started)
	return report, nil
}

func PolicyFromSpec(spec Spec) sandbox.Policy {
	policy := sandbox.Policy{Mode: sandbox.Mode(spec.Sandbox), Network: sandbox.Network(spec.Network)}
	return policy.Normalize()
}

func runScore(ctx context.Context, worktreePath, fixturePath string, spec Spec) ([]CheckResult, string, error) {
	if strings.TrimSpace(fixturePath) == "" {
		return nil, "error", errors.New("eval scorer requires fixture_path")
	}
	fixture, err := filepath.Abs(fixturePath)
	if err != nil {
		return nil, "error", fmt.Errorf("resolve scorer fixture path: %w", err)
	}
	info, err := os.Stat(fixture)
	if err != nil {
		return nil, "error", fmt.Errorf("stat scorer fixture path: %w", err)
	}
	if !info.IsDir() {
		return nil, "error", errors.New("eval scorer fixture path is not a directory")
	}
	root, err := workspace.Open(worktreePath)
	if err != nil {
		return nil, "error", fmt.Errorf("open scoring worktree: %w", err)
	}
	timeout := time.Duration(spec.ScoreTimeoutSeconds) * time.Second
	scoreCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	policy := PolicyFromSpec(spec)
	policy.ReadOnlyRoots = append(policy.ReadOnlyRoots, fixture)
	results := make([]CheckResult, 0, len(spec.Score))
	for _, command := range spec.Score {
		argv, err := expandScoreCommand(command, fixture, root.Path())
		if err != nil {
			return results, "error", err
		}
		result, err := tools.RunDeveloperSetup(scoreCtx, root, tools.CommandPolicy{
			Sandbox:        policy,
			Timeout:        timeout,
			MaxOutputBytes: 128 * 1024,
		}, argv)
		if err != nil {
			return results, "error", err
		}
		results = append(results, CheckResult{
			Argv:      append([]string(nil), result.Argv...),
			ExitCode:  result.ExitCode,
			Output:    result.Output,
			Truncated: result.Truncated,
			TimedOut:  result.TimedOut,
		})
		if result.TimedOut {
			return results, "error", fmt.Errorf("scorer command timed out: %s", strings.Join(argv, " "))
		}
		if result.ExitCode != 0 {
			return results, "failed", nil
		}
	}
	return results, "passed", nil
}

func expandScoreCommand(command []string, fixture, worktree string) ([]string, error) {
	result := make([]string, len(command))
	for index, token := range command {
		if err := validateScoreTemplate(token); err != nil {
			return nil, err
		}
		token = strings.ReplaceAll(token, "{{fixture}}", fixture)
		result[index] = strings.ReplaceAll(token, "{{worktree}}", worktree)
	}
	return result, nil
}

func validateScoreTemplate(token string) error {
	remaining := strings.ReplaceAll(strings.ReplaceAll(token, "{{fixture}}", ""), "{{worktree}}", "")
	if strings.Contains(remaining, "{{") || strings.Contains(remaining, "}}") {
		return fmt.Errorf("eval scorer token %q has an unknown template", token)
	}
	return nil
}

func joinErrors(left, right string) string {
	if strings.TrimSpace(left) == "" {
		return right
	}
	if strings.TrimSpace(right) == "" {
		return left
	}
	return left + "; " + right
}

func stringSHA256(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func WriteReport(path string, report Report) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("eval report path is required")
	}
	return writeJSON(path, report, "eval report")
}

// WriteSuiteReport atomically publishes an aggregate after its individual
// case reports have been written.
func WriteSuiteReport(path string, report SuiteReport) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("eval suite report path is required")
	}
	return writeJSON(path, report, "eval suite report")
}

// SummarizeSuite derives one deterministic aggregate. A suite is resolved only
// when every case resolved; an execution error takes precedence over ordinary
// unresolved model outcomes.
func SummarizeSuite(suite Suite, runID, provider, modelName string, startedAt, finishedAt time.Time, reports []Report) SuiteReport {
	report := SuiteReport{
		Version:   reportVersion,
		ID:        suite.ID,
		RunID:     runID,
		Provider:  provider,
		Model:     modelName,
		StartedAt: startedAt,
		Duration:  finishedAt.Sub(startedAt),
		Cases:     append([]Report(nil), reports...),
		Status:    "resolved",
	}
	for _, caseReport := range reports {
		switch caseReport.Status {
		case "resolved":
			report.Resolved++
		case "error":
			report.Errors++
		default:
			report.Unresolved++
		}
		switch caseReport.ScoreStatus {
		case "passed":
			report.ScorePassed++
		case "failed":
			report.ScoreFailed++
		case "error":
			report.ScoreErrors++
		default:
			report.Unscored++
		}
		if caseReport.Attempt > report.Attempts {
			report.Attempts = caseReport.Attempt
		}
	}
	report.CaseSummaries = summarizeCases(reports)
	if report.Errors > 0 {
		report.Status = "error"
	} else if report.Unresolved > 0 || report.Resolved != len(reports) || len(reports) == 0 {
		report.Status = "unresolved"
	}
	return report
}

func summarizeCases(reports []Report) []CaseSummary {
	index := make(map[string]int)
	var summaries []CaseSummary
	for _, report := range reports {
		position, found := index[report.ID]
		if !found {
			position = len(summaries)
			index[report.ID] = position
			summaries = append(summaries, CaseSummary{ID: report.ID})
		}
		summary := &summaries[position]
		summary.Attempts++
		switch report.Status {
		case "resolved":
			summary.Resolved++
		case "error":
			summary.Errors++
		default:
			summary.Unresolved++
		}
		switch report.ScoreStatus {
		case "passed":
			summary.ScorePassed++
		case "failed":
			summary.ScoreFailed++
		case "error":
			summary.ScoreErrors++
		default:
			summary.Unscored++
		}
	}
	return summaries
}

// ScriptedModel is a deterministic offline provider for eval fixtures.
type ScriptedModel struct {
	Turns []agent.Turn
}

func (m *ScriptedModel) Complete(_ context.Context, _ agent.TurnRequest) (agent.Turn, error) {
	if m == nil || len(m.Turns) == 0 {
		return agent.Turn{}, errors.New("eval script is exhausted")
	}
	turn := m.Turns[0]
	m.Turns = m.Turns[1:]
	return turn, nil
}

func copyDir(source, dest string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("eval repository %q must not be a symlink", source)
	}
	if !info.IsDir() {
		return fmt.Errorf("eval repository %q is not a directory", source)
	}
	if err := os.MkdirAll(dest, directoryMode(info.Mode())); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		from := filepath.Join(source, entry.Name())
		to := filepath.Join(dest, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("eval fixture contains symlink %q", from)
		}
		if info.IsDir() {
			if err := copyDir(from, to); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("eval fixture contains unsupported file %q", from)
		}
		if err := copyFile(from, to); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(source, dest string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode(info.Mode()))
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func digestFixtureDirectory(hash io.Writer, directory, relative string) error {
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("eval fixture %q must not be a symlink", directory)
	}
	if !info.IsDir() {
		return fmt.Errorf("eval fixture %q is not a directory", directory)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		name := filepath.ToSlash(filepath.Join(relative, entry.Name()))
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("eval fixture contains symlink %q", path)
		}
		if info.IsDir() {
			if _, err := io.WriteString(hash, "d\x00"+name+"\x00"); err != nil {
				return err
			}
			if err := digestFixtureDirectory(hash, path, name); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("eval fixture contains unsupported file %q", path)
		}
		if _, err := io.WriteString(hash, "f\x00"+name+"\x00"); err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if _, err := io.WriteString(hash, "\x00"); err != nil {
			return err
		}
	}
	return nil
}

func validateSpec(spec Spec) error {
	if spec.Version != specVersion {
		return fmt.Errorf("unsupported eval spec version %d", spec.Version)
	}
	if !validIdentifier(spec.ID) {
		return errors.New("eval spec id must be a portable run identifier")
	}
	if strings.TrimSpace(spec.Task) == "" {
		return errors.New("eval spec task is required")
	}
	if spec.MaxSteps < 1 {
		return errors.New("eval spec max_steps must be positive")
	}
	if len(spec.Verify) == 0 {
		return errors.New("eval spec verify is required")
	}
	if err := validateArgvLists("verification", spec.Verify, 0); err != nil {
		return err
	}
	if err := validateArgvLists("allowed_commands", spec.AllowedCommands, 0); err != nil {
		return err
	}
	if err := validateArgvLists("setup", spec.Setup, 8); err != nil {
		return err
	}
	if err := validateArgvLists("score", spec.Score, maxScoreCommands); err != nil {
		return err
	}
	if len(spec.Score) == 0 && spec.ScoreTimeoutSeconds != 0 {
		return errors.New("eval spec score_timeout_seconds requires score commands")
	}
	if len(spec.Score) > 0 && spec.ScoreTimeoutSeconds < 1 {
		return errors.New("eval spec score_timeout_seconds must be positive")
	}
	for _, label := range spec.Labels {
		if !validIdentifier(label) {
			return fmt.Errorf("eval spec label %q must be a portable identifier", label)
		}
	}
	for _, command := range spec.Score {
		for _, token := range command {
			if err := validateScoreTemplate(token); err != nil {
				return err
			}
		}
	}
	for _, prefix := range spec.AllowedCommandPrefixes {
		if err := tools.ValidateCommandPrefix(prefix); err != nil {
			return fmt.Errorf("eval spec allowed_command_prefixes: %w", err)
		}
	}
	for _, scope := range spec.Scopes {
		if !validFixturePath(scope) {
			return fmt.Errorf("eval spec scope %q must be repository-relative", scope)
		}
	}
	if repository := strings.TrimSpace(spec.Repository); repository != "" && !validFixturePath(repository) && filepath.Clean(repository) != "." {
		return errors.New("eval spec repository must be inside the fixture directory")
	}
	return nil
}

func validateArgvLists(name string, commands [][]string, maximum int) error {
	if maximum > 0 && len(commands) > maximum {
		return fmt.Errorf("eval spec allows at most %d %s commands", maximum, name)
	}
	for _, command := range commands {
		if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
			return fmt.Errorf("eval spec %s commands must contain an argv program", name)
		}
	}
	return nil
}

func validIdentifier(value string) bool {
	return identifierPattern.MatchString(value)
}

func validFixturePath(value string) bool {
	clean := filepath.Clean(value)
	return strings.TrimSpace(value) != "" && !filepath.IsAbs(value) && clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func directoryMode(mode os.FileMode) os.FileMode {
	return mode.Perm() | 0o700
}

func fileMode(mode os.FileMode) os.FileMode {
	return mode.Perm() | 0o200
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func cloneArgvLists(values [][]string) [][]string {
	result := make([][]string, len(values))
	for index, value := range values {
		result[index] = append([]string(nil), value...)
	}
	return result
}

func writeJSON(path string, value any, label string) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", label, err)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create %s directory: %w", label, err)
	}
	temporary, err := os.CreateTemp(directory, ".gator-eval-report-")
	if err != nil {
		return fmt.Errorf("create temporary %s: %w", label, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("restrict temporary %s: %w", label, err)
	}
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write %s: %w", label, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync %s: %w", label, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close %s: %w", label, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish %s: %w", label, err)
	}
	return nil
}
