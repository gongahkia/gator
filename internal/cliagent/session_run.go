package cliagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/hostproc"
	"github.com/gongahkia/gator/internal/patch"
)

type SessionControl struct {
	ID         string
	Kind       string
	Text       string
	QuestionID string
	Answers    map[string][]string
	Reply      chan<- error
}

type SessionRequest struct {
	Request
	Store, Owner, Capture, SessionID string
	Steering                         <-chan string
	Controls                         <-chan SessionControl
	OnState                          func(string, NativeState) error
}

// RunSession performs one assignment in a retained native conversation. The
// native process stays live for steering/questions, then stops before trusted
// patch inspection. Subsequent assignments resume the native conversation and
// private checkout, not a summary pasted into a fresh model context.
func RunSession(ctx context.Context, r SessionRequest) (result Result, err error) {
	if err := nativeText(r.Task); err != nil {
		return result, err
	}
	if r.Config.Kind != "codex" && r.Config.Kind != "pi" && r.Config.Kind != "claude" {
		return result, errors.New("persistent sessions unavailable for this adapter")
	}
	key := os.Getenv(r.Config.APIKeyEnv)
	if key == "" {
		return result, errors.New("CLI credential environment is missing")
	}
	defer func() {
		if err != nil {
			err = redactedSessionError{cause: err, text: strings.ReplaceAll(err.Error(), key, "[redacted]")}
		}
	}()
	s, err := openSession(r.Store, r.SessionID, SessionBinding{Owner: r.Owner, Capture: r.Capture, Config: r.Config, Edit: r.Edit, Scopes: r.Scopes})
	if err != nil {
		return result, err
	}
	defer s.Close()
	result.SessionID = s.record.ID
	ctx, cancel := context.WithTimeout(ctx, time.Duration(r.Config.TimeoutSeconds)*time.Second)
	defer cancel()
	directory := filepath.Join(s.path, "checkout")
	if !s.record.Initialized {
		if err := initializeSession(ctx, s, r.Request); err != nil {
			return result, err
		}
	} else if len(r.Baseline) > 0 {
		return result, errors.New("continuation uses its retained checkout; do not select a new baseline")
	}
	result.BaselineSHA256 = s.record.BaselineSHA256
	s.record.ProcessOpen = true
	if err := s.saveLocked(); err != nil {
		return result, err
	}
	p, err := openNativeProcess(ctx, r.Config, directory, filepath.Join(s.path, "home"), s.record.Native.ThreadID, r.Edit, r.Scopes)
	if err != nil {
		// openNativeProcess waits for its process to stop on every failure.
		s.record.ProcessOpen = false
		return result, errors.Join(err, s.saveLocked())
	}
	// On failure keep ProcessOpen set until this child has actually stopped.
	// Unacknowledged/unfinished commands remain in the journal for reconciliation.
	defer func() {
		p.Close()
		s.record.ProcessOpen = false
		s.record.Native = p.client.State()
		err = errors.Join(err, s.saveLocked())
	}()
	s.record.Native = p.client.State()
	if err := s.saveLocked(); err != nil {
		return result, err
	}
	id := "turn-" + sessionToken()
	if _, err := s.admit(id, "send", []byte(r.Task)); err != nil {
		return result, err
	}
	if err := p.client.Send(ctx, r.Task); err != nil {
		return result, err
	}
	delivery := "acknowledged"
	if r.Config.Kind == "claude" {
		delivery = "delivered"
	}
	if err := s.commandStatus(id, delivery); err != nil {
		return result, err
	}
	if r.OnEvent != nil {
		r.OnEvent(agent.Event{Kind: agent.EventSubagent, At: time.Now(), Text: r.Config.Kind + " native conversation started"})
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	var previous []byte
	steering, controls := r.Steering, r.Controls
	for {
		state := p.client.State()
		encoded, _ := json.Marshal(state)
		if !bytes.Equal(previous, encoded) {
			s.record.Native = state
			if err := s.saveLocked(); err != nil {
				return result, err
			}
			previous = encoded
			if r.OnState != nil {
				if err := r.OnState(s.record.ID, state); err != nil {
					return result, err
				}
			}
		}
		switch state.Status {
		case "completed", "failed", "cancelled":
			result.Summary, result.Turns = state.Summary, 1
			if err := s.commandStatus(id, "completed"); err != nil {
				return result, err
			}
			p.Close()
			if state.Status == "cancelled" {
				return result, fmt.Errorf("CLI turn cancelled: %s: %w", state.Error, context.Canceled)
			}
			if state.Status != "completed" {
				return result, fmt.Errorf("CLI turn %s: %s", state.Status, state.Error)
			}
			result.Patch, err = exportSession(ctx, s, r.Request)
			return result, err
		case "interrupted":
			return result, ErrSessionReconciliation
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case message, ok := <-steering:
			if !ok {
				steering = nil
				continue
			}
			if err := controlSession(ctx, s, p.client, SessionControl{ID: "steer-" + sessionToken(), Kind: "steer", Text: message}); err != nil {
				return result, err
			}
		case control, ok := <-controls:
			if !ok {
				controls = nil
				continue
			}
			controlErr := controlSession(ctx, s, p.client, control)
			if control.Reply != nil {
				select {
				case control.Reply <- controlErr:
				default:
				}
			}
			if s.failed != nil {
				return result, s.failed
			}
		case <-ticker.C:
		}
	}
}

type redactedSessionError struct {
	cause error
	text  string
}

func (e redactedSessionError) Error() string { return e.text }
func (e redactedSessionError) Unwrap() error { return e.cause }

func controlSession(ctx context.Context, s *sessionLease, c *nativeClient, control SessionControl) error {
	switch control.Kind {
	case "steer":
		if err := nativeText(control.Text); err != nil {
			return err
		}
		if c.State().Status != "running" {
			return errors.New("worker is not running")
		}
	case "answer":
		state := c.State()
		if state.Status != "awaiting_input" || control.QuestionID == "" || state.QuestionID != control.QuestionID {
			return errors.New("question changed or is no longer pending")
		}
		if len(control.Answers) != len(state.Questions) {
			return errors.New("answer every pending question exactly once")
		}
		for _, q := range state.Questions {
			answers := control.Answers[q.ID]
			if len(answers) != 1 {
				return errors.New("each question requires one answer")
			}
			if err := nativeText(answers[0]); err != nil {
				return err
			}
		}
	case "interrupt":
	default:
		return errors.New("invalid native control")
	}
	payload, _ := json.Marshal(struct {
		Text, Question string
		Answers        map[string][]string
	}{control.Text, control.QuestionID, control.Answers})
	send, err := s.admit(control.ID, control.Kind, payload)
	if err != nil || !send {
		return err
	}
	switch control.Kind {
	case "steer":
		err = c.Steer(ctx, control.Text)
	case "answer":
		err = c.Answer(ctx, control.Answers)
	case "interrupt":
		err = c.Interrupt(ctx)
	}
	if err != nil {
		// Do not mark an uncertain RPC rejected: it may already have executed.
		c.Close()
		return err
	}
	return s.commandStatus(control.ID, "completed")
}

func initializeSession(ctx context.Context, s *sessionLease, r Request) error {
	directory := filepath.Join(s.path, "checkout")
	if err := patch.InitSnapshot(ctx, r.Source, directory); err != nil {
		return err
	}
	git := func(args ...string) (string, error) {
		cmd := hostproc.CommandContext(ctx, "git", args...)
		cmd.Dir = directory
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	var err error
	s.record.Base, err = git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if len(r.Baseline) > 0 {
		if err := patch.ApplySnapshot(ctx, directory, r.Baseline); err != nil {
			return err
		}
		for _, args := range [][]string{{"add", "--all"}, {"-c", "user.name=Gator", "-c", "user.email=gator@invalid", "commit", "--quiet", "--allow-empty", "-m", "selected baseline"}} {
			if _, err := git(args...); err != nil {
				return err
			}
		}
	}
	s.record.SelectedBase, err = git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	tree, err := git("rev-parse", "HEAD^{tree}")
	if err != nil {
		return err
	}
	s.record.BaselineSHA256 = sessionDigest([]byte(tree))
	if err := os.Rename(filepath.Join(directory, ".git"), filepath.Join(s.path, "trusted-git")); err != nil {
		return err
	}
	s.record.Initialized = true
	s.record.Native.Status = "idle"
	return s.saveLocked()
}

func exportSession(ctx context.Context, s *sessionLease, r Request) (payload []byte, err error) {
	directory := filepath.Join(s.path, "checkout")
	git, trusted := filepath.Join(directory, ".git"), filepath.Join(s.path, "trusted-git")
	if err := os.RemoveAll(git); err != nil {
		return nil, err
	}
	if err := os.Rename(trusted, git); err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, os.Rename(git, trusted)) }()
	paths, err := patch.Paths(ctx, directory, s.record.SelectedBase)
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		allowed := r.Edit && len(r.Scopes) == 0
		if r.Edit {
			for _, scope := range r.Scopes {
				scope = filepath.ToSlash(scope)
				allowed = allowed || path == scope || strings.HasPrefix(path, scope+"/")
			}
		}
		if !allowed {
			return nil, fmt.Errorf("CLI changed %q outside its write envelope", path)
		}
	}
	if !r.Edit {
		return nil, nil
	}
	payload, err = patch.Export(ctx, directory, s.record.Base)
	if err == nil && bytes.Contains(payload, []byte(os.Getenv(r.Config.APIKeyEnv))) {
		return nil, errors.New("CLI patch contains its credential; result rejected")
	}
	return payload, err
}
