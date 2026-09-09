package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/worksession"
)

func isWorkSessionCommand(value string) bool {
	switch value {
	case "list", "show", "history", "resume", "back", "forward":
		return true
	default:
		return false
	}
}

func workSessionCommand(arguments []string, in io.Reader, out io.Writer, modelFactory workModelFactory) error {
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	store, err := worksession.Open(stateDir)
	if err != nil {
		return err
	}
	switch arguments[0] {
	case "list":
		conversations, err := store.List(50)
		if err != nil {
			return err
		}
		for _, conversation := range conversations {
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", conversation.ID, conversation.UpdatedAt.Format("2006-01-02 15:04"), conversation.Title, conversation.SourcePath)
		}
		return nil
	case "show":
		if len(arguments) != 2 {
			return errors.New("usage: gator work show CONVERSATION_ID")
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
		revisions, err := store.Revisions(conversation.ID)
		if err != nil {
			return err
		}
		for _, revision := range revisions {
			marker := " "
			if revision.ID == conversation.HeadRevision {
				marker = "*"
			}
			fmt.Fprintf(out, "%s %s\tparent=%s\t%s\t%s\n", marker, revision.ID, valueOrDash(revision.ParentRevisionID), revision.Status, firstLine(revision.Objective))
		}
		return nil
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
