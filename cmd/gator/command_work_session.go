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
	"strconv"
	"strings"

	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

func isWorkSessionCommand(value string) bool {
	switch value {
	case "list", "show", "history", "resume", "back", "forward", "tasks", "completions", "activity", "decisions", "answer", "reconcile", "stop", "steer", "cancel-task", "steer-task", "answer-task":
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
	if arguments[0] == "activity" {
		return workActivityCommand(stateDir, arguments[1:], out)
	}
	if arguments[0] == "answer" {
		return workAnswerCommand(stateDir, arguments, in, out)
	}
	if arguments[0] == "stop" || arguments[0] == "steer" {
		return workControlCommand(stateDir, arguments, out)
	}
	if arguments[0] == "cancel-task" {
		if len(arguments) != 3 {
			return errors.New("usage: gator work cancel-task RUN_ID TASK_ID")
		}
		if err := workrun.SendWorkTaskCancel(context.Background(), stateDir, arguments[1], arguments[2]); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "Cancellation requested for specialist %s in Work run %s.\n", arguments[2], arguments[1])
		return err
	}
	if arguments[0] == "steer-task" {
		if len(arguments) < 4 {
			return errors.New("usage: gator work steer-task RUN_ID TASK_ID TEXT")
		}
		if err := workrun.SendWorkTaskSteer(context.Background(), stateDir, arguments[1], arguments[2], strings.Join(arguments[3:], " ")); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "Steering queued for specialist %s in Work run %s; it applies at the next model boundary if the specialist continues.\n", arguments[2], arguments[1])
		return err
	}
	if arguments[0] == "answer-task" {
		if len(arguments) != 5 {
			return errors.New("usage: gator work answer-task RUN_ID TASK_ID QUESTION_ID ANSWERS_JSON_OR_DASH")
		}
		input := io.Reader(strings.NewReader(arguments[4]))
		if arguments[4] == "-" {
			input = in
		}
		payload, err := io.ReadAll(io.LimitReader(input, 32*1024+1))
		if err != nil {
			return err
		}
		if len(payload) > 32*1024 {
			return errors.New("task answers exceed 32 KiB")
		}
		var answers map[string][]string
		if err := json.Unmarshal(payload, &answers); err != nil {
			return errors.New("answers must be a JSON object mapping question keys to one-element text arrays")
		}
		if err := workrun.SendWorkTaskAnswer(context.Background(), stateDir, arguments[1], arguments[2], arguments[3], answers); err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Answer queued for specialist %s in Work run %s.\n", arguments[2], arguments[1])
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
	case "completions":
		if len(arguments) < 2 || len(arguments) > 3 {
			return errors.New("usage: gator work completions RUN_ID [COMPLETION_ID]")
		}
		records, err := workrun.ReadCompletions(stateDir, arguments[1])
		if err != nil {
			return err
		}
		if len(arguments) == 3 {
			for _, record := range records {
				if record.ID == arguments[2] {
					return json.NewEncoder(out).Encode(record)
				}
			}
			return errors.New("retained completion not found")
		}
		return json.NewEncoder(out).Encode(records)
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
			return json.NewEncoder(out).Encode(struct {
				workhistory.Record
				LiveStatus string `json:"live_status,omitempty"`
			}{Record: record, LiveStatus: record.LiveStatus})
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

func workControlCommand(stateDir string, arguments []string, out io.Writer) error {
	command := arguments[0]
	if command == "stop" && len(arguments) != 2 {
		return errors.New("usage: gator work stop RUN_ID")
	}
	if command == "steer" && len(arguments) < 3 {
		return errors.New("usage: gator work steer RUN_ID TEXT")
	}
	text := ""
	if command == "steer" {
		text = strings.Join(arguments[2:], " ")
	}
	if err := workrun.SendWorkControl(context.Background(), stateDir, arguments[1], command, text); err != nil {
		return err
	}
	if command == "stop" {
		_, err := fmt.Fprintf(out, "Stop requested for Work run %s.\n", arguments[1])
		return err
	}
	_, err := fmt.Fprintf(out, "Steering queued for Work run %s; if the run continues, it will apply at the next model boundary.\n", arguments[1])
	return err
}

func workAnswerCommand(stateDir string, arguments []string, in io.Reader, out io.Writer) error {
	if len(arguments) != 3 {
		return errors.New("usage: gator work answer RUN_ID DECISION_ID")
	}
	id, err := strconv.Atoi(arguments[2])
	if err != nil || id < 1 {
		return errors.New("decision ID must be positive")
	}
	selected, err := workrun.ReadInteractionRecord(stateDir, arguments[1], id)
	if err != nil {
		return err
	}
	if selected.Status != "pending" || selected.LiveStatus != "active" {
		return errors.New("decision is not pending in an active foreground run")
	}
	digest, err := workrun.InteractionDigest(selected.Interaction)
	if err != nil {
		return err
	}
	preview, err := json.Marshal(selected.Interaction)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "\nPending Work decision\n  run: %s\n  decision: %d (%s)\n  preview sha256: %s\n  exact preview (escaped): %s\nApprove this exact request? [y/N] ", arguments[1], id, strconv.QuoteToASCII(selected.Interaction.Kind), digest, strconv.QuoteToASCII(string(preview))); err != nil {
		return err
	}
	line, err := readApprovalLine(in)
	if err != nil && !(errors.Is(err, io.EOF) && line != "") {
		return errors.New("decision was not answered")
	}
	var approved bool
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		approved = true
	case "n", "no":
		approved = false
	default:
		return errors.New("type yes or no to answer this decision")
	}
	if err := workrun.SendWorkDecision(context.Background(), stateDir, arguments[1], id, digest, approved); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Decision %d submitted for Work run %s; inspect its result before assuming the action completed.\n", id, arguments[1])
	return err
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
	if _, err := fmt.Fprintln(out, "Recorded Work decisions (answer an active pending request with gator work answer RUN_ID DECISION_ID):"); err != nil {
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
