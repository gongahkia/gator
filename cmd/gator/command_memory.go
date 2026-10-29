package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/localmodel"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/retrieval"
	"github.com/gongahkia/gator/internal/state"
)

const memoryUsage = `Gator memory — explicit, inspectable personal knowledge

  gator memory add [--kind fact|preference|decision|note] [--goal ID] [--source REF|--source-file PATH] [--expires YYYY-MM-DD] TEXT
  gator memory list [--goal ID] [--all]
  gator memory search [--goal ID] QUERY
  gator memory diagnose
  gator memory show ID
  gator memory correct ID [--source-file PATH] TEXT
  gator memory forget ID
  gator memory embeddings status
  gator memory embeddings set ollama|openai|gemini [--allow-upload]
  gator memory embeddings install --yes

Goal memories are visible only in that goal; global memories are shared.
The agent uses bounded, relevant active memories as context, never as authority.`

func memoryCommand(args []string, out io.Writer) error {
	if len(args) == 0 || isHelpArgument(args[0]) {
		_, err := fmt.Fprintln(out, memoryUsage)
		return err
	}
	stateDir, err := state.ResolveDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	store, err := memory.Open(stateDir)
	if err != nil {
		return err
	}
	switch args[0] {
	case "embeddings":
		return memoryEmbeddingsCommand(args[1:], out)
	case "diagnose":
		if len(args) != 1 {
			return errors.New("usage: gator memory diagnose")
		}
		_, readErr := store.List("", true)
		var issues *memory.ReadIssues
		if errors.As(readErr, &issues) {
			if _, err := fmt.Fprintln(out, issues.Error()); err != nil {
				return err
			}
			for _, issue := range issues.Items {
				if _, err := fmt.Fprintf(out, "Unreadable record: %q\n", issue.File); err != nil {
					return err
				}
			}
			if issues.Count > len(issues.Items) {
				_, err = fmt.Fprintf(out, "%d additional unreadable records.\n", issues.Count-len(issues.Items))
				return err
			}
			_, err = fmt.Fprintln(out, "No files were modified. Restore damaged records from a verified backup; do not replace them with invented facts.")
			return err
		}
		if readErr != nil {
			return readErr
		}
		_, err = fmt.Fprintln(out, "No unreadable memory records found.")
		return err
	case "add":
		flags := flag.NewFlagSet("memory add", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		kind := flags.String("kind", "fact", "memory kind")
		goalID := flags.String("goal", "", "goal ID")
		source := flags.String("source", "user", "source reference")
		sourceFile := flags.String("source-file", "", "link to a bounded local file and detect changes")
		expires := flags.String("expires", "", "expiry date")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *goalID != "" {
			goals, err := goal.Open(stateDir)
			if err != nil {
				return err
			}
			if _, err := goals.Load(*goalID); err != nil {
				return err
			}
		}
		var expiry time.Time
		if *expires != "" {
			expiry, err = time.ParseInLocation("2006-01-02", *expires, time.Local)
			if err != nil {
				return errors.New("--expires must be YYYY-MM-DD")
			}
			expiry = expiry.AddDate(0, 0, 1)
			if !expiry.After(time.Now()) {
				return errors.New("expiry must be in the future")
			}
		}
		if *sourceFile != "" && *source != "user" {
			return errors.New("choose --source or --source-file, not both")
		}
		var record memory.Record
		if *sourceFile != "" {
			record, err = store.AddFromFile(*kind, strings.Join(flags.Args(), " "), *goalID, *sourceFile, expiry)
		} else {
			record, err = store.Add(*kind, strings.Join(flags.Args(), " "), *goalID, *source, expiry)
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "%s  %s\n", record.ID, record.Text)
		return err
	case "list":
		flags := flag.NewFlagSet("memory list", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		goalID := flags.String("goal", "", "goal ID")
		all := flags.Bool("all", false, "include expired")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if len(flags.Args()) != 0 {
			return errors.New("usage: gator memory list [--goal ID] [--all]")
		}
		records, err := store.List(*goalID, *all)
		return printMemoryRead(out, records, err)
	case "search":
		flags := flag.NewFlagSet("memory search", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		goalID := flags.String("goal", "", "goal ID")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		records, err := store.Search(strings.Join(flags.Args(), " "), *goalID, 30)
		return printMemoryRead(out, records, err)
	case "show":
		if len(args) != 2 {
			return errors.New("usage: gator memory show ID")
		}
		record, err := store.Load(args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "ID: %s\nKind: %s\nGoal: %s\nSource: %s\nSource status: %s\nExpires: %s\nCreated: %s\nUpdated: %s\nText: %s\n", record.ID, record.Kind, valueOrDash(record.GoalID), valueOrDash(record.Source), valueOrDash(record.SourceStatus), formatMemoryExpiry(record.ExpiresAt), record.CreatedAt.Format(time.RFC3339), record.UpdatedAt.Format(time.RFC3339), record.Text)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Last observed: %s\nLast confirmed: %s\n", formatMemoryEvidenceTime(record.LastObservedAt), formatMemoryEvidenceTime(record.LastConfirmedAt))
		if err != nil {
			return err
		}
		for _, evidence := range record.Evidence {
			if _, err = fmt.Fprintf(out, "Evidence: %s · %s · conversation %s · run %s · text SHA256 %s\n", evidence.ObservedAt.Format(time.RFC3339Nano), evidence.Basis, valueOrDash(evidence.ConversationID), valueOrDash(evidence.RunID), evidence.TextSHA256); err != nil {
				return err
			}
		}
		return err
	case "correct":
		if len(args) < 3 {
			return errors.New("usage: gator memory correct ID [--source-file PATH] TEXT")
		}
		flags := flag.NewFlagSet("memory correct", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		sourceFile := flags.String("source-file", "", "relink to a reviewed local source file")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		if len(flags.Args()) == 0 {
			return errors.New("memory correction text is required")
		}
		previous, err := store.Load(args[1])
		if err != nil {
			return err
		}
		var record memory.Record
		if *sourceFile != "" {
			record, err = store.CorrectFromFile(args[1], strings.Join(flags.Args(), " "), *sourceFile)
		} else {
			record, err = store.Correct(args[1], strings.Join(flags.Args(), " "), previous.Source)
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Corrected %s.\n", record.ID)
		return err
	case "forget":
		if len(args) != 2 {
			return errors.New("usage: gator memory forget ID")
		}
		if err := store.Forget(args[1]); err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Forgot %s.\n", args[1])
		return err
	default:
		return fmt.Errorf("unknown memory command %q", args[0])
	}
}

func memoryEmbeddingsCommand(args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "install" {
		if len(args) != 2 || args[1] != "--yes" {
			return errors.New("embeddinggemma:latest is a 622 MB Ollama download from https://ollama.com/library/embeddinggemma; repeat with install --yes")
		}
		client, err := localmodel.NewClient("")
		if err != nil {
			return err
		}
		model := localmodel.Model{OllamaModel: retrieval.LocalModel, SourceURL: "https://ollama.com/library/embeddinggemma", Download: "622 MB", DownloadBytes: 622_000_000}
		if err := client.Pull(context.Background(), model, func(progress localmodel.Progress) {
			if progress.Status != "" {
				_, _ = fmt.Fprintln(out, progress.Status)
			}
		}); err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "Local semantic recall is ready.")
		return err
	}
	settingsStore, err := config.DefaultStore()
	if err != nil {
		return err
	}
	settings, err := settingsStore.Load()
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "status" && len(args) == 1 {
		provider := settings.Embeddings.Provider
		if provider == "" {
			provider = "ollama"
		}
		status := "configured"
		if provider == "ollama" {
			status = "unavailable; using lexical search"
			if client, clientErr := localmodel.NewClient(""); clientErr == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				models, modelErr := client.Models(ctx)
				cancel()
				if modelErr == nil {
					status = "model missing; using lexical search (gator memory embeddings install --yes)"
					for _, model := range models {
						if model.Name == retrieval.LocalModel {
							status = "ready"
							break
						}
					}
				}
			}
		} else if !settings.Embeddings.CloudUploadApproved {
			status = "upload disabled; using lexical search"
		}
		_, err := fmt.Fprintf(out, "Memory search: %s embeddings (%s); lexical fallback on failure.\n", provider, status)
		return err
	}
	if args[0] != "set" || len(args) < 2 || len(args) > 3 {
		return errors.New("usage: gator memory embeddings set ollama|openai|gemini [--allow-upload]")
	}
	provider := args[1]
	if provider != "ollama" && provider != "openai" && provider != "gemini" {
		return errors.New("embeddings provider must be ollama, openai, or gemini")
	}
	approved := len(args) == 3 && args[2] == "--allow-upload"
	if len(args) == 3 && !approved {
		return errors.New("unknown embeddings option")
	}
	if provider != "ollama" && !approved {
		return errors.New("cloud embeddings upload saved memory and note text; repeat with --allow-upload to opt in")
	}
	settings.Embeddings = config.EmbeddingSettings{Provider: provider, CloudUploadApproved: approved && provider != "ollama"}
	if err := settingsStore.Save(settings); err != nil {
		return err
	}
	if provider == "ollama" {
		_, err = fmt.Fprintln(out, "Memory search uses local Ollama embeddinggemma:latest when installed, otherwise lexical fallback. Run gator memory embeddings install --yes to install it.")
	} else {
		_, err = fmt.Fprintf(out, "Memory search uses %s embeddings and may upload saved memory and note text. Configure its API key via /model Cloud or gator provider login %s.\n", provider, provider)
	}
	return err
}

func printMemories(out io.Writer, records []memory.Record) error {
	for _, record := range records {
		line := record.Text
		if record.SourceStatus == "changed" || record.SourceStatus == "unavailable" {
			line += " [source " + record.SourceStatus + "; excluded from automatic recall]"
		}
		if len(record.MatchedTerms) > 0 {
			line += " [lexical match: " + strings.Join(record.MatchedTerms, ", ") + "; scope: " + record.MatchScope + "]"
		}
		if _, err := fmt.Fprintf(out, "%s  %-10s %-29s %s\n", record.ID, record.Kind, valueOrDash(record.GoalID), line); err != nil {
			return err
		}
	}
	return nil
}
func formatMemoryExpiry(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Format("2006-01-02")
}

func formatMemoryEvidenceTime(value time.Time) string {
	if value.IsZero() {
		return "unknown (legacy record)"
	}
	return value.Format(time.RFC3339Nano)
}

// Partial reads remain visible in script and terminal output; ordinary failures
// still return an error and are never presented as an empty memory store.
func printMemoryRead(out io.Writer, records []memory.Record, readErr error) error {
	if warning := memory.ReadWarning(readErr); warning != "" {
		if _, err := fmt.Fprintln(out, "Warning: "+warning); err != nil {
			return err
		}
	} else if readErr != nil {
		return readErr
	}
	return printMemories(out, records)
}
