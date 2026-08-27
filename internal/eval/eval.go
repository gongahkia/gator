// Package eval runs a bounded, reproducible Gator evaluation against a fixture
// repository. It records a report; it does not claim SWE-bench or model
// competitiveness.
package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/sandbox"
)

const specVersion = 1

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
}

// Report is the durable outcome of one evaluation attempt.
type Report struct {
	ID             string        `json:"id"`
	RunID          string        `json:"run_id"`
	Status         string        `json:"status"`
	Task           string        `json:"task"`
	Provider       string        `json:"provider"`
	Model          string        `json:"model"`
	MaxSteps       int           `json:"max_steps"`
	Steps          int           `json:"steps"`
	TimeoutSeconds int           `json:"timeout_seconds"`
	Verify         [][]string    `json:"verify"`
	Sandbox        string        `json:"sandbox"`
	Network        string        `json:"network"`
	Duration       time.Duration `json:"duration_ns"`
	Error          string        `json:"error,omitempty"`
	FinalText      string        `json:"final_text,omitempty"`
	StatePath      string        `json:"state_path,omitempty"`
	WorktreePath   string        `json:"worktree_path,omitempty"`
	StartedAt      time.Time     `json:"started_at"`
}

// Options configures one evaluation. Model is required; live providers are
// injected by the command layer so this package never reads credentials.
type Options struct {
	Spec       Spec
	RunID      string
	Provider   string
	ModelName  string
	StateDir   string
	Repository string
	Executor   gatorrun.Executor
	Now        func() time.Time
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
	if strings.TrimSpace(spec.ID) == "" || strings.TrimSpace(spec.Task) == "" {
		return Spec{}, errors.New("eval spec id and task are required")
	}
	if spec.MaxSteps < 1 {
		return Spec{}, errors.New("eval spec max_steps must be positive")
	}
	if len(spec.Verify) == 0 {
		return Spec{}, errors.New("eval spec verify is required")
	}
	if spec.TimeoutSeconds < 1 {
		spec.TimeoutSeconds = 120
	}
	return spec, nil
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

// CopyRepository copies the fixture Git repository into dest.
func CopyRepository(source, dest string) error {
	return copyDir(source, dest)
}

// Run executes one evaluation attempt. Status is resolved, unresolved, or
// error. Re-using RunID against a previous report file is the caller's
// mistake; this function always executes.
func Run(ctx context.Context, options Options) (Report, error) {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	started := now()
	report := Report{
		ID:             options.Spec.ID,
		RunID:          options.RunID,
		Task:           options.Spec.Task,
		Provider:       options.Provider,
		Model:          options.ModelName,
		MaxSteps:       options.Spec.MaxSteps,
		TimeoutSeconds: options.Spec.TimeoutSeconds,
		Verify:         options.Spec.Verify,
		Sandbox:        string(options.Executor.Sandbox.Normalize().Mode),
		Network:        string(options.Executor.Sandbox.Normalize().Network),
		StartedAt:      started,
		Status:         "error",
	}
	if strings.TrimSpace(options.RunID) == "" {
		return report, errors.New("eval run_id is required")
	}
	if strings.TrimSpace(options.Repository) == "" {
		return report, errors.New("eval repository is required")
	}
	timeout := time.Duration(options.Spec.TimeoutSeconds) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	outcome, err := options.Executor.Execute(runCtx, gatorrun.Request{
		RepositoryPath: options.Repository,
		Task:           options.Spec.Task,
		Provider:       options.Provider,
		Model:          options.ModelName,
		RunID:          options.RunID,
		MaxSteps:       options.Spec.MaxSteps,
		Verification:   options.Spec.Verify,
		StateDir:       options.StateDir,
		Mode:           gatorrun.ExecuteMode,
	})
	report.Duration = now().Sub(started)
	report.StatePath = outcome.StatePath
	report.WorktreePath = outcome.Worktree.Path
	report.Steps = outcome.Result.Steps
	report.FinalText = outcome.Result.FinalText
	if err != nil {
		report.Error = err.Error()
		if runCtx.Err() != nil {
			report.Status = "error"
			report.Error = "eval exceeded timeout_seconds: " + err.Error()
		} else {
			report.Status = "unresolved"
		}
		return report, nil
	}
	report.Status = "resolved"
	return report, nil
}

func PolicyFromSpec(spec Spec) sandbox.Policy {
	policy := sandbox.Policy{Mode: sandbox.Mode(spec.Sandbox), Network: sandbox.Network(spec.Network)}
	return policy.Normalize()
}

func WriteReport(path string, report Report) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("eval report path is required")
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode eval report: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create eval report directory: %w", err)
	}
	return os.WriteFile(path, append(payload, '\n'), 0o600)
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
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("eval repository %q is not a directory", source)
	}
	if err := os.MkdirAll(dest, info.Mode()); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		from := filepath.Join(source, entry.Name())
		to := filepath.Join(dest, entry.Name())
		if entry.IsDir() {
			if err := copyDir(from, to); err != nil {
				return err
			}
			continue
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
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
