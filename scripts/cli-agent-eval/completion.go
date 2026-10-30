package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/cliagent"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workrun"
)

func runCompletionLoop(ctx context.Context, cfg cliagent.Config, root, source string) error {
	backend, err := model.New(model.Config{Provider: model.OpenAI, Model: cfg.Model, BaseURL: strings.TrimRight(cfg.BaseURL, "/") + "/responses"})
	if err != nil {
		return err
	}
	original := "def normalize_title(text):\n    return text\n"
	verifier := `from normalize import normalize_title
cases = [("  Hello   WORLD  ", "hello world"), ("A\tB\nC", "a b c"), ("   ", ""), ("Already Good", "already good")]
for value, expected in cases:
    actual = normalize_title(value)
    assert actual == expected, f"normalize_title({value!r}): expected {expected!r}, got {actual!r}"
print("4 acceptance cases passed")
`
	if err := os.WriteFile(filepath.Join(source, "normalize.py"), []byte(original), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(source, "verify.py"), []byte(verifier), 0600); err != nil {
		return err
	}
	objective := "Implement normalize_title(text) in normalize.py: trim surrounding whitespace, collapse all whitespace runs to one space, lowercase letters, and return an empty string for whitespace-only input. Controlled repair evaluation: on the first assignment only, implement just lowercasing and return so the host verifier can expose the remaining cases. On later repair assignments, satisfy every acceptance criterion. Change only normalize.py."
	criteria := []string{"Leading/trailing whitespace is removed", "All internal whitespace runs collapse to a single space", "Letters are lowercased", "Whitespace-only text normalizes to empty"}
	args, _ := json.Marshal(map[string]any{"agent": cfg.Role(), "objective": objective, "acceptance": criteria, "max_attempts": 3})
	prompt := fmt.Sprintf("Call complete_code exactly once with these arguments: %s. Let its bounded loop implement, verify, independently review and repair. Do not implement the code yourself or start other workers. After it returns, write output/report.md stating its status and retained loop ID. Never claim completion if the loop is not completed.", args)
	state := filepath.Join(root, "state")
	service := workrun.Service{Executor: workrun.Executor{Model: backend.Model, StateDir: state}}
	request := workrun.Request{RunID: "completion-live", SourcePath: source, Objective: prompt, Mode: action.Draft, Contract: artifact.DefaultContract("report.md"), ExternalAgents: []cliagent.Config{cfg}, MaxSteps: 16, Limits: agent.Limits{ModelRequests: 40, Tokens: 200000}, Code: workrun.CodePolicy{Scopes: []string{"normalize.py"}, Verification: [][]string{{"python3", "-B", "verify.py"}}, Sandbox: sandbox.Policy{Mode: sandbox.Strict}}}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	start := time.Now()
	_, runErr := service.Execute(ctx, request)
	files, readErr := filepath.Glob(filepath.Join(state, "gator", "completions", request.RunID, "*.json"))
	checks := map[string]bool{"work_succeeded": runErr == nil, "one_retained_loop": readErr == nil && len(files) == 1}
	var evidence json.RawMessage
	var record struct {
		Status   string `json:"status"`
		Attempts []struct {
			SessionID string `json:"session_id"`
			Feedback  string `json:"feedback"`
			Artifact  string `json:"artifact"`
			Candidate *struct {
				Status string `json:"status"`
			} `json:"candidate"`
			Review *struct {
				Passed bool `json:"passed"`
			} `json:"review"`
		} `json:"attempts"`
	}
	if len(files) == 1 {
		evidence, readErr = os.ReadFile(files[0])
		if readErr == nil {
			readErr = json.Unmarshal(evidence, &record)
		}
	}
	checks["loop_completed"] = readErr == nil && record.Status == "completed"
	checks["repair_observed"] = len(record.Attempts) >= 2 && strings.Contains(record.Attempts[0].Feedback, "verification failed")
	same, retained := len(record.Attempts) >= 2 && record.Attempts[0].SessionID != "", len(record.Attempts) >= 2
	for _, attempt := range record.Attempts {
		same = same && attempt.SessionID == record.Attempts[0].SessionID
		retained = retained && attempt.Artifact != ""
	}
	checks["same_native_worker"], checks["attempt_patches_retained"] = same, retained
	checks["verified_and_independently_reviewed"] = false
	if len(record.Attempts) > 0 {
		last := record.Attempts[len(record.Attempts)-1]
		checks["verified_and_independently_reviewed"] = last.Candidate != nil && last.Candidate.Status == "verified" && last.Review != nil && last.Review.Passed
	}
	current, sourceErr := os.ReadFile(filepath.Join(source, "normalize.py"))
	currentVerifier, verifierErr := os.ReadFile(filepath.Join(source, "verify.py"))
	checks["original_and_verifier_unchanged"] = sourceErr == nil && verifierErr == nil && string(current) == original && string(currentVerifier) == verifier
	passed := true
	for _, value := range checks {
		passed = passed && value
	}
	diagnostic := ""
	if err := errors.Join(runErr, readErr); err != nil {
		diagnostic = strings.ReplaceAll(err.Error(), os.Getenv(cfg.APIKeyEnv), "[redacted]")
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if len(evidence) == 0 {
		evidence = json.RawMessage(`null`)
	}
	if err := encoder.Encode(map[string]any{"evaluation": "bounded-completion-loop-v1", "controlled_initial_defect": true, "kind": cfg.Kind, "model": cfg.Model, "checks": checks, "passed": passed, "completion": evidence, "error": diagnostic, "seconds": time.Since(start).Seconds(), "at": time.Now().UTC().Format(time.RFC3339)}); err != nil {
		return err
	}
	if !passed {
		return errors.New("live completion-loop evaluation failed")
	}
	return nil
}
