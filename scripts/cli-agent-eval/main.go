// Opt-in live adapter evaluation. Sends only a synthetic fixture to the chosen
// installed CLI; does not alter user configuration, memory or source files.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gongahkia/gator/internal/cliagent"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	kind := flag.String("kind", "codex", "codex or pi")
	model := flag.String("model", "gpt-5.6", "model")
	baseURL := flag.String("base-url", "", "regional API base URL")
	edit := flag.Bool("edit", false, "test isolated patch creation")
	sessions := flag.Bool("session-manager", false, "test live manager/native-worker follow-up continuity")
	crossChat := flag.Bool("cross-chat", false, "split the native follow-up across separate Work conversation turns")
	controls := flag.Bool("controls", false, "test native clarification (Codex), steering, interruption and resumption")
	completion := flag.Bool("completion-loop", false, "test live implementation, host checks, independent review and repair")
	flag.Parse()
	path, err := exec.LookPath(*kind)
	check(err)
	path, err = filepath.EvalSymlinks(path)
	check(err)
	roots := []string{filepath.Dir(path)}
	if *kind == "pi" {
		for dir := filepath.Dir(path); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
				roots = append(roots, dir)
				break
			}
		}
		node, err := exec.LookPath("node")
		check(err)
		node, err = filepath.EvalSymlinks(node)
		check(err)
		roots = append(roots, filepath.Dir(filepath.Dir(node)))
	}
	root, err := os.MkdirTemp("", "gator-cli-eval-")
	check(err)
	defer os.RemoveAll(root)
	root, err = filepath.EvalSymlinks(root)
	check(err)
	source := filepath.Join(root, "source")
	scratch := filepath.Join(root, "scratch")
	check(os.Mkdir(source, 0700))
	check(os.Mkdir(scratch, 0700))
	original := "# Project Lantern\nOwner: Mira\nDeadline: 2030-05-16\nStatus: draft\n"
	check(os.WriteFile(filepath.Join(source, "project.md"), []byte(original), 0600))
	cfg := cliagent.Config{Kind: *kind, Executable: path, Model: *model, BaseURL: *baseURL, APIKeyEnv: "OPENAI_API_KEY", ReadOnlyRoots: roots, TimeoutSeconds: 120, AllowEdits: *edit}
	if *kind == "pi" {
		cfg.Provider = "openai"
	}
	if *sessions {
		check(runSessionManager(context.Background(), cfg, root, source, original, *crossChat))
		return
	}
	if *completion {
		cfg.AllowEdits = true
		check(runCompletionLoop(context.Background(), cfg, root, source))
		return
	}
	if *controls {
		check(runSessionControls(context.Background(), cfg, root, source, original))
		return
	}
	task := "Read project.md. Report the project name, owner and deadline exactly. Do not edit files."
	if *edit {
		task = "Edit only project.md: change Status: draft to Status: reviewed, preserving all other content. Read the result and report the project name, owner and deadline in your summary."
	}
	start := time.Now()
	value, runErr := cliagent.Run(context.Background(), cliagent.Request{Config: cfg, Source: source, Scratch: scratch, ID: "live-fixture", Task: task, Edit: *edit, Scopes: []string{"project.md"}})
	current, err := os.ReadFile(filepath.Join(source, "project.md"))
	check(err)
	passed := runErr == nil && string(current) == original && strings.Contains(value.Summary, "Mira") && strings.Contains(value.Summary, "2030-05-16")
	if *edit {
		passed = passed && strings.Contains(string(value.Patch), "+Status: reviewed") && strings.Contains(string(value.Patch), "-Status: draft")
	} else {
		passed = passed && len(value.Patch) == 0
	}
	diagnostic := ""
	if runErr != nil {
		diagnostic = runErr.Error()
	}
	report := map[string]any{"kind": *kind, "model": *model, "edit": *edit, "passed": passed, "seconds": time.Since(start).Seconds(), "summary": value.Summary, "usage": value.Usage, "patch_bytes": len(value.Patch), "original_unchanged": string(current) == original, "error": diagnostic, "at": time.Now().UTC().Format(time.RFC3339)}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	check(encoder.Encode(report))
	if !passed {
		os.Exit(1)
	}
}
func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
