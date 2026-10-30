package cliagent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This opt-in component test verifies native conversation continuity, including
// a real subprocess restart. It does not stand in for the later manager/UI tests.
func TestLiveNativeConversation(t *testing.T) {
	output := os.Getenv("GATOR_LIVE_NATIVE_REPORT")
	if output == "" {
		t.Skip("opt-in native session evaluation")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("report path must be new")
	}
	if os.Getenv("OPENAI_API_KEY") == "" || os.Getenv("GATOR_LIVE_MODEL") == "" {
		t.Fatal("live model/credential missing")
	}
	type check struct {
		Kind   string      `json:"kind"`
		Stage  string      `json:"stage"`
		Passed bool        `json:"passed"`
		State  NativeState `json:"state"`
		Error  string      `json:"error,omitempty"`
	}
	report := []check{}
	defer func() {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Error(err)
			return
		}
		if err := os.WriteFile(output, append(data, '\n'), 0600); err != nil {
			t.Error(err)
		}
	}()
	for _, kind := range []string{"codex", "pi"} {
		t.Run(kind, func(t *testing.T) {
			path, err := exec.LookPath(kind)
			if err != nil {
				t.Fatal(err)
			}
			path, err = filepath.EvalSymlinks(path)
			if err != nil {
				t.Fatal(err)
			}
			roots := []string{filepath.Dir(path)}
			if kind == "pi" {
				for dir := filepath.Dir(path); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
					if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
						roots = append(roots, dir)
						break
					}
				}
				node, err := exec.LookPath("node")
				if err != nil {
					t.Fatal(err)
				}
				node, err = filepath.EvalSymlinks(node)
				if err != nil {
					t.Fatal(err)
				}
				roots = append(roots, filepath.Dir(filepath.Dir(node)))
			}
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(root, "source")
			home := filepath.Join(root, "home")
			for _, dir := range []string{source, home} {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(source, "note.txt"), []byte("Project Lantern is owned by Mira.\n"), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := Config{Kind: kind, Executable: path, APIKeyEnv: "OPENAI_API_KEY", ReadOnlyRoots: roots, TimeoutSeconds: 120, Model: os.Getenv("GATOR_LIVE_MODEL"), BaseURL: os.Getenv("GATOR_LIVE_CLI_ENDPOINT")}
			if kind == "pi" {
				cfg.Provider = "openai"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			p, err := openNativeProcess(ctx, cfg, source, home, "", false, nil)
			if err != nil {
				report = append(report, check{Kind: kind, Stage: "start", Error: err.Error()})
				t.Fatal(err)
			}
			defer func() {
				if p != nil {
					p.Close()
				}
			}()
			threadID := p.client.State().ThreadID
			for i, text := range []string{"Read note.txt. Remember the codeword capybara-924 in this conversation. Report the project owner and codeword briefly.", "Without reading any file, what codeword did I ask you to remember? Reply with that exact codeword.", "What codeword did I ask you to remember, and who owns the project from the file you read earlier?"} {
				stage := []string{"first_turn", "follow_up", "after_process_restart"}[i]
				if i == 2 {
					p.Close()
					p, err = openNativeProcess(ctx, cfg, source, home, threadID, false, nil)
					if err != nil {
						report = append(report, check{Kind: kind, Stage: stage, Error: err.Error()})
						t.Fatal(err)
					}
				}
				turnCtx, finish := context.WithTimeout(ctx, 90*time.Second)
				err = p.client.Send(turnCtx, text)
				state := p.client.State()
				if err == nil {
					state, err = p.client.Await(turnCtx)
				}
				finish()
				passed := err == nil && state.Status == "completed" && state.ThreadID == threadID && strings.Contains(state.Summary, "capybara-924")
				if i != 1 {
					passed = passed && strings.Contains(state.Summary, "Mira")
				}
				result := check{Kind: kind, Stage: stage, Passed: passed, State: state}
				if err != nil {
					result.Error = err.Error()
				}
				report = append(report, result)
				if !passed {
					t.Fatalf("native %s failed: %s %+v", stage, result.Error, state)
				}
			}
		})
	}
}
