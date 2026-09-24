package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/learning"
)

const learningUsage = `usage:
  gator learnings list
  gator learnings show LEARNING_ID
  gator learnings add --type preference|environment_fact|procedure|failure_prevention --key KEY [--scope global|project[=PATH]] TEXT
  gator learnings enable|disable|remove|reject LEARNING_ID
  gator learnings edit LEARNING_ID [--key KEY] TEXT

"remove" is reversible: it disables the learning and retains its provenance.
Candidate learnings are created only from future transaction-outcome work; this
foundation lets users inspect, enable, disable, or reject them when present.`

func learningCommand(arguments []string, out io.Writer) error {
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	return runLearningCommand(stateDir, arguments, out)
}

// runLearningCommand is the shared local presentation adapter used by both
// the CLI and the Work TUI. It invokes the same file-backed learning service;
// the TUI never shells out to the CLI.
func runLearningCommand(stateDir string, arguments []string, out io.Writer) error {
	store, err := learning.Open(stateDir)
	if err != nil {
		return err
	}
	if len(arguments) == 0 || isHelpArgument(arguments[0]) {
		_, err := fmt.Fprintln(out, learningUsage)
		return err
	}
	switch arguments[0] {
	case "list":
		if len(arguments) != 1 {
			return errors.New("usage: gator learnings list")
		}
		records, err := store.List()
		if err != nil {
			return err
		}
		return writeLearnings(out, records)
	case "show":
		if len(arguments) != 2 {
			return errors.New("usage: gator learnings show LEARNING_ID")
		}
		record, err := store.Load(arguments[1])
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(record)
	case "add":
		return addLearning(store, arguments[1:], out)
	case "enable", "disable", "remove", "reject":
		if len(arguments) != 2 {
			return fmt.Errorf("usage: gator learnings %s LEARNING_ID", arguments[0])
		}
		var record learning.Record
		switch arguments[0] {
		case "enable":
			record, err = store.Enable(arguments[1])
		case "disable":
			record, err = store.Disable(arguments[1])
		case "remove":
			record, err = store.Remove(arguments[1])
		case "reject":
			record, err = store.Reject(arguments[1])
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "%s %s (%s).\n", record.ID, record.Status, record.Scope.Kind)
		return err
	case "edit":
		return editLearning(store, arguments[1:], out)
	default:
		return fmt.Errorf("unknown learning operation %q\n%s", arguments[0], learningUsage)
	}
}

func addLearning(store learning.Store, arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("learnings add", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	typeName := flags.String("type", "", "learning type")
	key := flags.String("key", "", "stable conflict key")
	scopeName := flags.String("scope", "global", "global or project[=PATH]")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	content := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if content == "" {
		return errors.New("learning text is required")
	}
	typeValue, err := parseLearningType(*typeName)
	if err != nil {
		return err
	}
	scope, err := parseLearningScope(*scopeName)
	if err != nil {
		return err
	}
	record, err := store.Create(learning.Create{
		Type: typeValue, Key: strings.ToLower(strings.TrimSpace(*key)), Content: content,
		Scope: scope, Origin: learning.UserAuthored,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Added active learning %s (%s, %s).\n", record.ID, record.Type, formatLearningScope(record.Scope))
	return err
}

func editLearning(store learning.Store, arguments []string, out io.Writer) error {
	if len(arguments) < 2 {
		return errors.New("usage: gator learnings edit LEARNING_ID [--key KEY] TEXT")
	}
	id := arguments[0]
	flags := flag.NewFlagSet("learnings edit", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	key := flags.String("key", "", "replacement stable conflict key")
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	remaining := flags.Args()
	if len(remaining) == 0 {
		return errors.New("usage: gator learnings edit LEARNING_ID [--key KEY] TEXT")
	}
	content := strings.TrimSpace(strings.Join(remaining, " "))
	record, err := store.Edit(id, strings.ToLower(strings.TrimSpace(*key)), content)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Updated learning %s.\n", record.ID)
	return err
}

func parseLearningType(value string) (learning.Type, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	result := learning.Type(value)
	switch result {
	case learning.Preference, learning.EnvironmentFact, learning.Procedure, learning.FailurePrevention:
		return result, nil
	default:
		return "", errors.New("learning type must be preference, environment_fact, procedure, or failure_prevention")
	}
}

func parseLearningScope(value string) (learning.Scope, error) {
	value = strings.TrimSpace(value)
	if value == "global" {
		return learning.Scope{Kind: learning.Global}, nil
	}
	if value == "project" {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return learning.Scope{}, err
		}
		return learning.Scope{Kind: learning.Project, Value: workingDirectory}, nil
	}
	if path, found := strings.CutPrefix(value, "project="); found && strings.TrimSpace(path) != "" {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return learning.Scope{}, err
		}
		return learning.Scope{Kind: learning.Project, Value: absolute}, nil
	}
	return learning.Scope{}, errors.New("learning scope must be global, project, or project=PATH")
}

func writeLearnings(out io.Writer, records []learning.Record) error {
	if len(records) == 0 {
		_, err := fmt.Fprintln(out, "No learnings yet. Add one with: gator learnings add --type preference --key output --scope project \"Use Markdown.\"")
		return err
	}
	for _, record := range records {
		confidence := ""
		if record.Origin == learning.Inferred {
			confidence = fmt.Sprintf(" confidence:%d", record.Confidence)
		}
		if _, err := fmt.Fprintf(out, "%s  %s  %s  %s  %s%s\n", record.ID, record.Status, record.Type, formatLearningScope(record.Scope), record.Key, confidence); err != nil {
			return err
		}
	}
	return nil
}

func formatLearningScope(scope learning.Scope) string {
	if scope.Kind == learning.Global {
		return string(learning.Global)
	}
	return string(scope.Kind) + ":" + scope.Value
}

func learningTUIAction(stateDir string, arguments []string) (string, error) {
	var output bytes.Buffer
	err := runLearningCommand(stateDir, arguments, &output)
	return strings.TrimSpace(output.String()), err
}
