package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

func isWorkSessionCommand(value string) bool {
	switch value {
	case "list", "show", "history", "resume", "back", "forward", "tasks", "decisions", "reconcile":
		return true
	default:
		return false
	}
}

func workSessionCommand(arguments []string, in io.Reader, out io.Writer, modelFactory workModelFactory) error {
	stateDir, err := state.ResolveDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	if arguments[0] == "reconcile" {
		if len(arguments) != 3 {
			return errors.New("usage: gator work reconcile WORK_ID CORRELATION_ID")
		}
		return reconcileGmailSend(context.Background(), stateDir, arguments[1], arguments[2], out)
	}
	if arguments[0] == "decisions" {
		return workDecisionsCommand(stateDir, arguments[1:], out)
	}
	store, err := worksession.Open(stateDir)
	if err != nil {
		return err
	}
	history, err := workhistory.Open(stateDir)
	if err != nil {
		return err
	}
	switch arguments[0] {
	case "tasks":
		if len(arguments) < 2 || len(arguments) > 3 {
			return errors.New("usage: gator work tasks RUN_ID [TASK_ID]")
		}
		run := arguments[1]
		if run == "." || run == ".." || run == "" || strings.ContainsAny(run, "/\\") {
			return errors.New("invalid Work run ID")
		}
		tasks, err := orchestrator.ReadTasks(filepath.Join(stateDir, "gator", "tasks", run))
		if err != nil {
			return err
		}
		if len(arguments) == 3 {
			for _, task := range tasks {
				if task.ID == arguments[2] {
					return json.NewEncoder(out).Encode(task)
				}
			}
			return errors.New("retained task not found")
		}
		return json.NewEncoder(out).Encode(tasks)
	case "list":
		records, err := history.List(50)
		if err != nil {
			return err
		}
		return writeWorkHistory(out, stateDir, records)
	case "show":
		if len(arguments) != 2 {
			return errors.New("usage: gator work show WORK_ID|CONVERSATION_ID")
		}
		if record, loadErr := history.Load(arguments[1]); loadErr == nil {
			return json.NewEncoder(out).Encode(record)
		} else if !errors.Is(loadErr, os.ErrNotExist) {
			return loadErr
		}
		conversation, err := store.Load(arguments[1])
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(conversation)
	case "history":
		if len(arguments) != 2 {
			return errors.New("usage: gator work history CONVERSATION_ID")
		}
		conversation, err := store.Load(arguments[1])
		if err != nil {
			return err
		}
		records, err := history.ListConversation(conversation.ID, 0)
		if err != nil {
			return err
		}
		return writeWorkHistory(out, stateDir, records)
	case "back":
		if len(arguments) != 2 {
			return errors.New("usage: gator work back CONVERSATION_ID")
		}
		conversation, err := store.Load(arguments[1])
		if err != nil {
			return err
		}
		if conversation.HeadRevision == "" {
			return errors.New("conversation has no revision to move back from")
		}
		head, err := store.LoadRevision(conversation.ID, conversation.HeadRevision)
		if err != nil {
			return err
		}
		if head.ParentRevisionID == "" {
			return errors.New("conversation is already at its first revision")
		}
		moved, err := store.MoveHead(conversation.ID, head.ParentRevisionID)
		if err == nil {
			fmt.Fprintln(out, moved.HeadRevision)
		}
		return err
	case "forward":
		if len(arguments) < 2 || len(arguments) > 3 {
			return errors.New("usage: gator work forward CONVERSATION_ID [REVISION_ID]")
		}
		conversation, err := store.Load(arguments[1])
		if err != nil {
			return err
		}
		children, err := store.Children(conversation.ID, conversation.HeadRevision)
		if err != nil {
			return err
		}
		target := ""
		if len(arguments) == 3 {
			target = arguments[2]
		} else if len(children) == 1 {
			target = children[0].ID
		} else if len(children) == 0 {
			return errors.New("conversation has no forward revision")
		} else {
			for _, child := range children {
				fmt.Fprintf(out, "%s\t%s\n", child.ID, firstLine(child.Objective))
			}
			return errors.New("multiple forward revisions; repeat with one revision ID")
		}
		validChild := false
		for _, child := range children {
			validChild = validChild || child.ID == target
		}
		if !validChild {
			return errors.New("selected revision is not a child of the current head")
		}
		moved, err := store.MoveHead(conversation.ID, target)
		if err == nil {
			fmt.Fprintln(out, moved.HeadRevision)
		}
		return err
	case "resume":
		flags := flag.NewFlagSet("work resume", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		refresh := flags.Bool("refresh-source", false, "capture a fresh source snapshot")
		parent := flags.String("parent", "", "branch from a specific revision")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		remaining := flags.Args()
		if len(remaining) < 2 {
			return errors.New("usage: gator work resume [--refresh-source] [--parent REVISION] CONVERSATION_ID TASK")
		}
		conversationID := remaining[0]
		forwarded := []string{"--conversation", conversationID}
		if *refresh {
			forwarded = append(forwarded, "--refresh-source")
		}
		if strings.TrimSpace(*parent) != "" {
			forwarded = append(forwarded, "--parent", *parent)
		}
		forwarded = append(forwarded, remaining[1:]...)
		return runWorkTask(forwarded, in, out, modelFactory)
	}
	return errors.New("unknown Work conversation command")
}

func workDecisionsCommand(stateDir string, arguments []string, out io.Writer) error {
	var runID string
	var jsonOutput bool
	for _, argument := range arguments {
		if argument == "--json" && !jsonOutput {
			jsonOutput = true
		} else if !strings.HasPrefix(argument, "-") && runID == "" {
			runID = argument
		} else {
			return errors.New("usage: gator work decisions [RUN_ID] [--json]")
		}
	}
	var records []workrun.InteractionRecord
	var err error
	if runID == "" {
		records, err = workrun.ReadPendingInteractionRecords(stateDir)
	} else {
		records, err = workrun.ReadInteractionRecords(stateDir, runID)
	}
	if err != nil {
		return err
	}
	if jsonOutput {
		return json.NewEncoder(out).Encode(struct {
			LiveStatus string                      `json:"live_status"`
			Decisions  []workrun.InteractionRecord `json:"decisions"`
		}{LiveStatus: "per_run_lock", Decisions: records})
	}
	if _, err := fmt.Fprintln(out, "Recorded Work decisions (read-only; answer approvals in the original foreground run):"); err != nil {
		return err
	}
	if len(records) == 0 {
		_, err := fmt.Fprintln(out, "  none")
		return err
	}
	for _, record := range records {
		status := record.Status
		if record.Status == "pending" {
			switch record.LiveStatus {
			case "active":
				status = "pending (run active)"
			case "stopped":
				status = "pending (stale)"
			default:
				status = "pending (run unconfirmed)"
			}
		}
		if record.Status == "resolved" && record.Approved != nil {
			if *record.Approved {
				status = "approved"
			} else {
				status = "denied"
			}
		}
		if _, err := fmt.Fprintf(out, "  %s #%d %s %s\n", record.RunID, record.Interaction.ID, status, terminalSafe(record.Interaction.Kind)); err != nil {
			return err
		}
	}
	return nil
}

func firstLine(value string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(value), "\n")
	if len(line) > 100 {
		return line[:100] + "…"
	}
	return line
}
