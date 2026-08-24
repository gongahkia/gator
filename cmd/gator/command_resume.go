package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/journal"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

func resumeTask(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("resume", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	maxSteps := flags.Int("max-steps", 0, "maximum model turns for this continuation")
	compact := flags.Bool("compact", false, "summarize older retained context before continuing")
	last := flags.Bool("last", false, "resume the most recent retained thread for this repository")
	all := flags.Bool("all", false, "include retained threads from other repositories")
	var imagePaths attachmentFlags
	flags.Var(&imagePaths, "image", "repository-relative PNG, JPEG, or WebP image to include in the continuation (repeatable)")
	var documentPaths attachmentFlags
	flags.Var(&documentPaths, "attach", "repository-relative PDF or supported document/text file to include in the continuation (repeatable)")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	arguments = flags.Args()
	var target, continuation string
	if *last {
		continuation = strings.TrimSpace(strings.Join(arguments, " "))
	} else if len(arguments) > 0 {
		target = arguments[0]
		continuation = strings.TrimSpace(strings.Join(arguments[1:], " "))
	}

	if target == "" && !*last {
		if len(imagePaths)+len(documentPaths) > 0 {
			return errors.New("--image and --attach require a continuation task")
		}
		return interactiveWithOptions(interactiveOptions{StartInRecent: true, RecentAll: *all, AllowNoRepository: *all})
	}
	if target != "" && looksLikeRunRecordPath(target) {
		if *all {
			return errors.New("--all cannot be combined with a run record path")
		}
		return resumeStateWithAttachments(target, continuation, *maxSteps, *compact, imagePaths, documentPaths, out)
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	repository, repositoryErr := gitRepositoryRoot(workingDirectory)
	if repositoryErr != nil && !*all {
		return errors.New("resume by thread requires starting inside a Git checkout")
	}
	if repositoryErr != nil {
		repository = workingDirectory
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	selected, err := resolveResumeThread(stateDir, repository, target, *last, *all)
	if err != nil {
		return err
	}
	if continuation == "" {
		if len(imagePaths)+len(documentPaths) > 0 {
			return errors.New("--image and --attach require a continuation task")
		}
		return interactiveWithOptions(interactiveOptions{RepositoryPath: selected.Repository, ResumeStatePath: selected.HeadStatePath})
	}
	return resumeStateWithAttachments(selected.HeadStatePath, continuation, *maxSteps, *compact, imagePaths, documentPaths, out)
}

const resumeCandidateLimit = 1_000

func resolveResumeThread(stateDir, repository, target string, last, all bool) (journal.RecentThread, error) {
	var (
		threads []journal.RecentThread
		err     error
	)
	if all {
		threads, err = journal.ListAllRecentThreads(stateDir, resumeCandidateLimit)
	} else {
		threads, err = journal.ListRecentThreads(stateDir, repository, resumeCandidateLimit)
	}
	if err != nil {
		return journal.RecentThread{}, err
	}
	if len(threads) == 0 {
		return journal.RecentThread{}, errors.New("no retained threads are available; start a new Gator run first")
	}
	if last {
		return requireAvailableThread(threads[0])
	}

	var matches []journal.RecentThread
	for _, thread := range threads {
		if thread.ID == target {
			return requireAvailableThread(thread)
		}
		if strings.HasPrefix(thread.ID, target) {
			matches = append(matches, thread)
		}
	}
	if len(matches) == 0 {
		return journal.RecentThread{}, fmt.Errorf("no retained thread matches %q; run 'gator resume' to choose one", target)
	}
	if len(matches) > 1 {
		return journal.RecentThread{}, fmt.Errorf("retained thread prefix %q is ambiguous; use a longer ID or run 'gator resume'", target)
	}
	return requireAvailableThread(matches[0])
}

func requireAvailableThread(thread journal.RecentThread) (journal.RecentThread, error) {
	if !thread.Available {
		return journal.RecentThread{}, fmt.Errorf("retained thread %q cannot resume because its worktree no longer exists", thread.ID)
	}
	return thread, nil
}

func looksLikeRunRecordPath(target string) bool {
	if filepath.IsAbs(target) || strings.ContainsRune(target, filepath.Separator) {
		return true
	}
	info, err := os.Stat(target)
	return err == nil && info.IsDir()
}

func resumeState(statePath, continuation string, maxSteps int, compact bool, out io.Writer) error {
	return resumeStateWithAttachments(statePath, continuation, maxSteps, compact, nil, nil, out)
}

func resumeStateWithAttachments(statePath, continuation string, maxSteps int, compact bool, imagePaths, documentPaths []string, out io.Writer) error {
	if continuation == "" {
		if len(imagePaths)+len(documentPaths) > 0 {
			return errors.New("--image and --attach require a continuation task")
		}
		session, err := journal.LoadSession(statePath)
		if err != nil {
			return err
		}
		return interactiveWithOptions(interactiveOptions{RepositoryPath: session.Repository, ResumeStatePath: statePath})
	}
	session, err := journal.LoadSession(statePath)
	if err != nil {
		return err
	}
	images, attachments, err := loadPromptAttachments(session.Repository, imagePaths, documentPaths)
	if err != nil {
		return err
	}
	providerName, modelName, err := resolveConfiguredProvider(session.Provider, session.Model)
	if err != nil {
		return fmt.Errorf("load retained provider: %w", err)
	}
	executor, err := newExecutor(providerName, modelName, session.BaseURL)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Gator resume\n  provider: %s\n  model: %s\n  task: %s\n", providerName, displayModel(modelName), continuation); err != nil {
		return err
	}
	if err := writePromptAttachmentSummary(out, images, attachments); err != nil {
		return err
	}
	printer := eventPrinter{out: out}
	outcome, err := executor.Resume(context.Background(), session, statePath, continuation, gatorrun.Request{
		MaxSteps:        maxSteps,
		ForceCompaction: compact,
		OnEvent:         printer.Print,
		Images:          images,
		Attachments:     attachments,
	})
	if outcome.Worktree.Path != "" {
		if _, writeErr := fmt.Fprintf(out, "\nReview worktree: %s\n", outcome.Worktree.Path); writeErr != nil && err == nil {
			err = writeErr
		}
	}
	if outcome.StatePath != "" {
		if _, writeErr := fmt.Fprintf(out, "Run record: %s\n", outcome.StatePath); writeErr != nil && err == nil {
			err = writeErr
		}
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "\n%s\n", outcome.Result.FinalText)
	return err
}
