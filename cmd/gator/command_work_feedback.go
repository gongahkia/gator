package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/worksession"
)

const workFeedbackUsage = `usage:
  gator work feedback WORK_ID list
  gator work feedback WORK_ID accept|reject|dont-learn [NOTE]
  gator work feedback WORK_ID correct --type TYPE --key KEY [--scope project|global|project=PATH] TEXT
  gator work feedback WORK_ID remember --type TYPE --key KEY [--scope project|global|project=PATH] TEXT

"correct" creates an inspectable candidate only. "remember" creates an
explicit active scoped learning immediately. Neither retries Work or delivery.`

func workFeedbackCommand(arguments []string, out io.Writer) error {
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	return runWorkFeedbackCommand(stateDir, arguments, out)
}

// runWorkFeedbackCommand is shared by the CLI and Work TUI. It records only
// explicit user feedback against an existing Work transaction; it never asks a
// model to infer intent from free-form conversation history.
func runWorkFeedbackCommand(stateDir string, arguments []string, out io.Writer) error {
	if len(arguments) < 2 || isHelpArgument(arguments[0]) {
		_, err := fmt.Fprintln(out, workFeedbackUsage)
		return err
	}
	workID, actionName := arguments[0], strings.ToLower(arguments[1])
	history, err := workhistory.Open(stateDir)
	if err != nil {
		return err
	}
	record, err := history.Load(workID)
	if err != nil {
		return fmt.Errorf("load Work for feedback: %w", err)
	}
	store, err := learning.Open(stateDir)
	if err != nil {
		return err
	}
	if actionName == "list" {
		if len(arguments) != 2 {
			return errors.New("usage: gator work feedback WORK_ID list")
		}
		observations, err := store.ListObservations(workID)
		if err != nil {
			return err
		}
		return writeWorkFeedback(out, observations)
	}
	if actionName == "correct" || actionName == "remember" {
		return writeCorrectiveFeedback(stateDir, store, record, actionName, arguments[2:], out)
	}
	var signal learning.Signal
	var summary string
	switch actionName {
	case "accept":
		signal, summary = learning.UserAccepted, "User accepted this Work outcome."
	case "reject":
		signal, summary = learning.UserRejected, "User rejected this Work outcome."
	case "dont-learn":
		signal, summary = learning.UserDeclinedLearning, "User declined learning from this Work outcome."
	default:
		return fmt.Errorf("unknown feedback action %q\n%s", actionName, workFeedbackUsage)
	}
	if note := strings.TrimSpace(strings.Join(arguments[2:], " ")); note != "" {
		summary = boundedFeedbackSummary(note)
	}
	observation, err := store.RecordObservation(learning.ObservationInput{
		Signal: signal, WorkID: record.ID, Summary: summary,
		EvidenceRefs: []string{workHistoryReference(record.ID)},
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Recorded %s feedback as %s.\n", actionName, observation.ID)
	return err
}

func writeCorrectiveFeedback(stateDir string, store learning.Store, history workhistory.Record, actionName string, arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("work feedback "+actionName, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	typeName := flags.String("type", "", "learning type")
	key := flags.String("key", "", "stable conflict key")
	scopeName := flags.String("scope", "project", "project, global, or project=PATH")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	content := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if content == "" {
		return errors.New("feedback text is required")
	}
	typeValue, err := parseLearningType(*typeName)
	if err != nil {
		return err
	}
	scope, err := feedbackScope(stateDir, history, *scopeName)
	if err != nil {
		return err
	}
	signal := learning.UserCorrected
	if actionName == "remember" {
		signal = learning.UserRemembered
	}
	observation, err := store.RecordObservation(learning.ObservationInput{
		Signal: signal, WorkID: history.ID, Scope: &scope, Summary: boundedFeedbackSummary(content),
		EvidenceRefs: []string{workHistoryReference(history.ID)},
	})
	if err != nil {
		return err
	}
	if actionName == "correct" {
		candidate, err := store.Propose(learning.Proposal{
			Observation: observation, Type: typeValue, Key: strings.ToLower(strings.TrimSpace(*key)), Content: content, Scope: scope,
		})
		if err != nil {
			return err
		}
		if _, err := store.LinkObservation(observation.ID, candidate.ID); err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Recorded correction %s and proposed candidate %s. Review it with gator learnings show %s; enable it only if you want it active.\n", observation.ID, candidate.ID, candidate.ID)
		return err
	}
	explicit, err := store.Create(learning.Create{
		Type: typeValue, Key: strings.ToLower(strings.TrimSpace(*key)), Content: content, Scope: scope, Origin: learning.UserAuthored,
		Provenance: learning.Provenance{WorkIDs: []string{history.ID}, EvidenceRefs: []string{observationReference(observation.ID)}},
	})
	if err != nil {
		return err
	}
	if _, err := store.LinkObservation(observation.ID, explicit.ID); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Remembered this as active learning %s.\n", explicit.ID)
	return err
}

func feedbackScope(stateDir string, history workhistory.Record, value string) (learning.Scope, error) {
	if strings.TrimSpace(value) != "project" {
		return parseLearningScope(value)
	}
	if history.ConversationID == "" {
		return learning.Scope{}, errors.New("this Work has no retained source; use --scope global or --scope project=PATH")
	}
	sessions, err := worksession.Open(stateDir)
	if err != nil {
		return learning.Scope{}, err
	}
	conversation, err := sessions.Load(history.ConversationID)
	if err != nil {
		return learning.Scope{}, err
	}
	if strings.TrimSpace(conversation.SourcePath) == "" {
		return learning.Scope{}, errors.New("this Work has no retained source; use --scope global or --scope project=PATH")
	}
	return learning.Scope{Kind: learning.Project, Value: conversation.SourcePath}, nil
}

func writeWorkFeedback(out io.Writer, observations []learning.Observation) error {
	if len(observations) == 0 {
		_, err := fmt.Fprintln(out, "No explicit feedback or learning-relevant observations are retained for this Work.")
		return err
	}
	for _, observation := range observations {
		linked := ""
		if len(observation.LearningIDs) > 0 {
			linked = " learnings:" + strings.Join(observation.LearningIDs, ",")
		}
		if _, err := fmt.Fprintf(out, "%s  %s  %s%s\n", observation.ID, observation.Signal, observation.Reliability, linked); err != nil {
			return err
		}
	}
	return nil
}

func workHistoryReference(workID string) string {
	return "gator/work-history/" + workID + ".json"
}

func observationReference(observationID string) string {
	return "gator/observations/" + observationID + ".json"
}

func boundedFeedbackSummary(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 1024 {
		return value
	}
	return strings.TrimSpace(value[:1021]) + "…"
}

func workFeedbackTUIAction(stateDir, workID string, arguments []string) (string, error) {
	var output bytes.Buffer
	err := runWorkFeedbackCommand(stateDir, append([]string{workID}, arguments...), &output)
	return strings.TrimSpace(output.String()), err
}
