package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/cliagent"
)

func runSessionControls(ctx context.Context, cfg cliagent.Config, root, source, original string) error {
	if cfg.Kind != "codex" && cfg.Kind != "pi" {
		return errors.New("live controls evaluation requires Codex or Pi with the supplied OpenAI credential")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	request := cliagent.SessionRequest{Request: cliagent.Request{Config: cfg, Source: source, ID: "controls", Task: "Read project.md."}, Store: filepath.Join(root, "sessions"), Owner: "controls-conversation", Capture: "synthetic-source"}
	type turn struct{ Stage, Status, Summary, Error string }
	turns := []turn{}
	checks := map[string]bool{}
	start := time.Now()
	run := func(stage, task string, makeControl func(cliagent.NativeState) *cliagent.SessionControl) (cliagent.Result, error, error) {
		commands := make(chan cliagent.SessionControl, 1)
		reply := make(chan error, 1)
		sent := false
		status := ""
		request.Task, request.Controls = task, commands
		request.OnState = func(_ string, state cliagent.NativeState) error {
			status = state.Status
			if !sent && makeControl != nil {
				if command := makeControl(state); command != nil {
					command.ID, command.Reply = stage, reply
					commands <- *command
					sent = true
				}
			}
			return nil
		}
		value, err := cliagent.RunSession(ctx, request)
		if request.SessionID == "" {
			request.SessionID = value.SessionID
		}
		diagnostic := ""
		if err != nil {
			diagnostic = err.Error()
		}
		turns = append(turns, turn{stage, status, value.Summary, diagnostic})
		var commandErr error
		if makeControl != nil {
			select {
			case commandErr = <-reply:
			default:
				commandErr = errors.New("expected control was not acknowledged")
			}
		}
		return value, err, commandErr
	}
	if cfg.Kind == "codex" {
		value, err, answerErr := run("question", "Before proceeding, call gator_ask_user with question 'Which format should the project summary use?' and options ['Markdown','Plain text']. Wait for the answer, then read project.md and report the chosen format and owner briefly.", func(state cliagent.NativeState) *cliagent.SessionControl {
			if state.Status != "awaiting_input" {
				return nil
			}
			return &cliagent.SessionControl{Kind: "answer", QuestionID: state.QuestionID, Answers: map[string][]string{"answer": {"Markdown"}}}
		})
		checks["structured_question_answered"] = err == nil && answerErr == nil && strings.Contains(value.Summary, "Markdown") && strings.Contains(value.Summary, "Mira")
	}
	value, err, steerErr := run("steering", "Read project.md and describe the project briefly.", func(state cliagent.NativeState) *cliagent.SessionControl {
		if state.Status != "running" {
			return nil
		}
		return &cliagent.SessionControl{Kind: "steer", Text: "Append the exact marker STEERING-LANTERN-924 to your final answer."}
	})
	checks["live_steering_observed"] = err == nil && steerErr == nil && strings.Contains(value.Summary, "STEERING-LANTERN-924")
	_, err, interruptErr := run("interruption", "Read project.md and write a detailed explanation of the project's status, ownership, and deadline.", func(state cliagent.NativeState) *cliagent.SessionControl {
		if state.Status != "running" {
			return nil
		}
		return &cliagent.SessionControl{Kind: "interrupt"}
	})
	checks["native_interruption"] = err != nil && interruptErr == nil && turns[len(turns)-1].Status == "cancelled"
	value, err, _ = run("resume_after_interruption", "Reply with exactly RESUMED-AFTER-INTERRUPT.", nil)
	checks["resume_after_interruption"] = err == nil && strings.Contains(value.Summary, "RESUMED-AFTER-INTERRUPT")
	current, readErr := os.ReadFile(filepath.Join(source, "project.md"))
	checks["original_unchanged"] = readErr == nil && string(current) == original
	passed := true
	for _, value := range checks {
		passed = passed && value
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(map[string]any{"evaluation": "native-session-controls-v1", "kind": cfg.Kind, "model": cfg.Model, "passed": passed, "checks": checks, "turns": turns, "seconds": time.Since(start).Seconds(), "at": time.Now().UTC().Format(time.RFC3339)}); err != nil {
		return err
	}
	if !passed {
		return errors.New("live native controls evaluation failed")
	}
	return nil
}
