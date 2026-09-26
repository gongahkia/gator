package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/gongahkia/gator/internal/orchestrator"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/worksession"
)

func isWorkSessionCommand(value string) bool {
	switch value {
	case "list", "show", "history", "resume", "back", "forward", "tasks":
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

func firstLine(value string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(value), "\n")
	if len(line) > 100 {
		return line[:100] + "…"
	}
	return line
}
