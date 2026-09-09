package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/worksession"
	"github.com/gongahkia/gator/internal/worktui"
)

func workInteractive() error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	input, err := os.Stdin.Stat()
	if err != nil || input.Mode()&os.ModeCharDevice == 0 {
		return errors.New("interactive Work requires a terminal; use 'gator work' for scripts")
	}
	output, err := os.Stdout.Stat()
	if err != nil || output.Mode()&os.ModeCharDevice == 0 {
		return errors.New("interactive Work requires a terminal; use 'gator work' for scripts")
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	sessions, err := worksession.Open(stateDir)
	if err != nil {
		return err
	}
	conversations, err := sessions.List(8)
	if err != nil {
		return err
	}
	jobStore, err := jobs.Open(stateDir)
	if err != nil {
		return err
	}
	definitions, err := jobStore.List()
	if err != nil {
		return err
	}
	inboxStore, err := inbox.Open(stateDir)
	if err != nil {
		return err
	}
	inboxEntries, err := inboxStore.List(false, 20)
	if err != nil {
		return err
	}
	settings, err := loadSettings()
	if err != nil {
		return err
	}
	application := worktui.New(worktui.Config{
		CurrentFolder: workingDirectory, Conversations: conversations, Jobs: definitions, Inbox: inboxEntries,
		FirstRun: settings.Defaults.Provider == "" && len(conversations) == 0,
		SetupCommand: func(provider string) *exec.Cmd {
			command := exec.Command(os.Args[0], "connect", provider)
			command.Stdin = os.Stdin
			command.Stdout = os.Stdout
			command.Stderr = os.Stderr
			return command
		},
		Run: func(source, conversationID, prompt string) worktui.RunResult {
			return runInteractiveWork(source, conversationID, prompt, stateDir)
		},
		MoveBack: func(conversationID string) (string, error) {
			conversation, err := sessions.Load(conversationID)
			if err != nil {
				return "", err
			}
			revision, err := sessions.LoadRevision(conversationID, conversation.HeadRevision)
			if err != nil {
				return "", err
			}
			if revision.ParentRevisionID == "" {
				return "", errors.New("already at the first revision")
			}
			moved, err := sessions.MoveHead(conversationID, revision.ParentRevisionID)
			if err != nil {
				return "", err
			}
			return "Moved back to " + moved.HeadRevision + ". Your newer branch is still retained.", nil
		},
		MoveForward: func(conversationID string) (string, error) {
			conversation, err := sessions.Load(conversationID)
			if err != nil {
				return "", err
			}
			children, err := sessions.Children(conversationID, conversation.HeadRevision)
			if err != nil {
				return "", err
			}
			if len(children) == 0 {
				return "", errors.New("there is no forward revision")
			}
			if len(children) > 1 {
				var choices []string
				for _, child := range children {
					choices = append(choices, child.ID+" — "+firstLine(child.Objective))
				}
				return "", errors.New("choose a branch from History:\n" + strings.Join(choices, "\n"))
			}
			moved, err := sessions.MoveHead(conversationID, children[0].ID)
			if err != nil {
				return "", err
			}
			return "Moved forward to " + moved.HeadRevision + ".", nil
		},
		History: func(conversationID string) (string, error) {
			conversation, err := sessions.Load(conversationID)
			if err != nil {
				return "", err
			}
			revisions, err := sessions.Revisions(conversationID)
			if err != nil {
				return "", err
			}
			var lines []string
			for _, revision := range revisions {
				marker := "  "
				if revision.ID == conversation.HeadRevision {
					marker = "→ "
				}
				lines = append(lines, marker+revision.ID+" — "+firstLine(revision.Objective))
			}
			return strings.Join(lines, "\n"), nil
		},
		CodeCommand: func() *exec.Cmd {
			command := exec.Command(os.Args[0], "code", "--tui")
			command.Stdin = os.Stdin
			command.Stdout = os.Stdout
			command.Stderr = os.Stderr
			return command
		},
	})
	program := tea.NewProgram(application, tea.WithAltScreen())
	_, err = program.Run()
	return err
}

func runInteractiveWork(source, conversationID, prompt, stateDir string) worktui.RunResult {
	arguments := []string{"work", "--json"}
	if conversationID != "" {
		arguments = append(arguments, "--conversation", conversationID)
	} else {
		arguments = append(arguments, "--source", source)
	}
	arguments = append(arguments, "--", prompt)
	command := exec.Command(os.Args[0], arguments...)
	command.Env = append(os.Environ(), "GATOR_STATE_DIR="+stateDir)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	var response struct {
		ConversationID string `json:"conversation_id"`
		RevisionID     string `json:"revision_id"`
		SnapshotID     string `json:"snapshot_id"`
		FinalText      string `json:"final_text"`
		OutputPath     string `json:"output_path"`
		Error          string `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return worktui.RunResult{Error: strings.TrimSpace(stderr.String() + " " + stdout.String())}
	}
	if runErr != nil && response.Error == "" {
		response.Error = fmt.Sprintf("%v: %s", runErr, stderr.String())
	}
	return worktui.RunResult{ConversationID: response.ConversationID, RevisionID: response.RevisionID, SnapshotID: response.SnapshotID, FinalText: response.FinalText, OutputPath: response.OutputPath, Error: response.Error}
}

func unifiedResume(arguments []string, out io.Writer) error {
	if len(arguments) == 0 {
		return workInteractive()
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	store, err := worksession.Open(stateDir)
	if err == nil {
		if _, loadErr := store.Load(arguments[0]); loadErr == nil {
			if len(arguments) == 1 {
				return workInteractive()
			}
			forwarded := append([]string{"resume", arguments[0]}, arguments[1:]...)
			return workSessionCommand(forwarded, os.Stdin, out, nativeWorkModel)
		}
	}
	return resumeTask(arguments, out)
}
