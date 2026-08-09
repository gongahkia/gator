package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/gongahkia/gator/internal/core"
	"github.com/gongahkia/gator/internal/tui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "gator:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, input io.Reader, output, stderr io.Writer) error {
	if len(args) == 0 {
		engine, err := openEngine(ctx)
		if err != nil {
			return err
		}
		return tui.Run(ctx, engine, input, output, stderr)
	}
	switch args[0] {
	case "help", "--help", "-h":
		usage(output)
		return nil
	case "version", "--version":
		fmt.Fprintln(output, "gator standalone v0.1.0")
		return nil
	case "providers":
		engine, err := openEngine(ctx)
		if err != nil {
			return err
		}
		for _, status := range engine.Providers(ctx) {
			fmt.Fprintf(output, "%s\tavailable=%t\tdiscovered=%t\ttransport=%s\t%s\n", status.ID, status.Available, status.Discovered, transport(status.Capabilities), status.Reason)
		}
		return nil
	case "health":
		return health(ctx, output)
	case "runs":
		engine, err := openEngine(ctx)
		if err != nil {
			return err
		}
		return listRuns(engine, output)
	case "events":
		if len(args) != 2 {
			return errors.New("usage: gator events <run-id>")
		}
		engine, err := openEngine(ctx)
		if err != nil {
			return err
		}
		return events(engine, args[1], output)
	case "run":
		return runCommand(ctx, args[1:], input, output, stderr)
	case "review":
		return reviewCommand(ctx, args[1:], output, stderr)
	case "handoff":
		return handoffCommand(ctx, args[1:], input, output, stderr)
	case "worktrees":
		engine, err := openEngine(ctx)
		if err != nil {
			return err
		}
		return listWorktrees(engine, output)
	case "policy":
		return policyCommand(ctx, args[1:], output)
	case "experiment":
		return experimentCommand(ctx, args[1:], input, output, stderr)
	default:
		return fmt.Errorf("unknown command %q; run gator help", args[0])
	}
}

func openEngine(ctx context.Context) (*core.Engine, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return core.Open(ctx, cwd)
}

type filesFlag []core.FileReference

func (f *filesFlag) String() string { return fmt.Sprint(len(*f)) }
func (f *filesFlag) Set(value string) error {
	reference, err := core.ParseFileReference(value)
	if err != nil {
		return err
	}
	*f = append(*f, reference)
	return nil
}

func runCommand(ctx context.Context, args []string, input io.Reader, output, stderr io.Writer) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	provider, role := flags.String("provider", "", "provider id"), flags.String("role", "writer", "writer, researcher, reviewer, or integrator")
	execute, worktree := flags.Bool("execute", false, "execute the provider"), flags.Bool("worktree", false, "use a new isolated Git worktree")
	noDiff := flags.Bool("no-diff", false, "do not include current Git diff")
	var files filesFlag
	flags.Var(&files, "file", "context PATH or PATH:START:END (repeatable)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: gator run [flags] <objective>")
	}
	engine, err := openEngine(ctx)
	if err != nil {
		return err
	}
	var includeDiff *bool
	if *noDiff {
		value := false
		includeDiff = &value
	}
	run, err := engine.Run(ctx, core.RunRequest{Objective: flags.Arg(0), Provider: *provider, Role: *role, Files: files, IncludeDiff: includeDiff, Worktree: *worktree, Execute: *execute}, input, output, stderr)
	fmt.Fprintf(output, "%s\t%s\tprovider=%s\tverify=%s\n", run.ID, run.State, run.Provider, run.Verification.State)
	return err
}

func reviewCommand(ctx context.Context, args []string, output, stderr io.Writer) error {
	flags := flag.NewFlagSet("review", flag.ContinueOnError)
	flags.SetOutput(stderr)
	verify := flags.Bool("verify", false, "run configured verification commands")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: gator review [--verify] <run-id>")
	}
	engine, err := openEngine(ctx)
	if err != nil {
		return err
	}
	run, err := engine.Store().GetRun(flags.Arg(0))
	if err != nil {
		return err
	}
	if !*verify {
		fmt.Fprintf(output, "%s\tstate=%s\tverification=%s\tbase=%s\tdiff=%s\n", run.ID, run.State, run.Verification.State, run.Verification.BaseSHA, run.Verification.DiffSHA256)
		return nil
	}
	evidence, err := engine.VerifyAll(ctx, run.ID, output, stderr)
	for _, item := range evidence {
		fmt.Fprintf(output, "%s\tpassed=%t\texit=%d\n", item.CommandID, item.Passed, item.ExitCode)
	}
	return err
}

func handoffCommand(ctx context.Context, args []string, input io.Reader, output, stderr io.Writer) error {
	flags := flag.NewFlagSet("handoff", flag.ContinueOnError)
	flags.SetOutput(stderr)
	execute := flags.Bool("execute", false, "execute target provider")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 2 {
		return errors.New("usage: gator handoff [--execute] <run-id> <provider>")
	}
	engine, err := openEngine(ctx)
	if err != nil {
		return err
	}
	run, err := engine.Handoff(ctx, flags.Arg(0), flags.Arg(1), *execute, input, output, stderr)
	fmt.Fprintf(output, "%s\t%s\tparent=%s\n", run.ID, run.State, run.ParentRunID)
	return err
}

func policyCommand(ctx context.Context, args []string, output io.Writer) error {
	if len(args) != 1 || (args[0] != "init" && args[0] != "show" && args[0] != "validate") {
		return errors.New("usage: gator policy <init|show|validate>")
	}
	engine, err := openEngine(ctx)
	if err != nil {
		return err
	}
	switch args[0] {
	case "init":
		_, err := core.SavePolicy(engine.Workspace().Root, core.DefaultPolicy())
		if err != nil {
			return err
		}
		fmt.Fprintln(output, core.PolicyPath(engine.Workspace().Root))
		return nil
	case "show":
		policy, ref, err := engine.Policy()
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "version=%s\nsource=%s\ndefault_provider=%s\ntopology=%s\nmax_files=%d\nmax_bytes=%d\ninclude_diff=%t\nverification_commands=%d\n", ref.Version, ref.Source, policy.Routing.DefaultProvider, policy.Workflow.Topology, policy.Context.MaxFiles, policy.Context.MaxBytes, policy.Context.IncludeDiff, len(policy.Verification.Commands))
		return nil
	default:
		_, ref, err := engine.Policy()
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "valid\t%s\n", ref.Version)
		return nil
	}
}

func experimentCommand(ctx context.Context, args []string, input io.Reader, output, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "compare" {
		return errors.New("usage: gator experiment compare [flags] <objective>")
	}
	flags := flag.NewFlagSet("experiment compare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	provider := flags.String("provider", "", "provider id")
	baseline := flags.String("baseline", "", "baseline policy JSON")
	candidate := flags.String("candidate", "", "candidate policy JSON")
	execute := flags.Bool("execute", false, "execute providers in isolated worktrees")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: gator experiment compare --provider P --baseline PATH --candidate PATH [--execute] <objective>")
	}
	engine, err := openEngine(ctx)
	if err != nil {
		return err
	}
	result, err := engine.ComparePolicies(ctx, core.ExperimentRequest{Objective: flags.Arg(0), Provider: *provider, BaselinePolicy: *baseline, CandidatePolicy: *candidate, Execute: *execute}, input, output, stderr)
	if result.ID != "" {
		fmt.Fprintf(output, "%s\t%s\t%s\n", result.ID, result.Verdict, result.Reason)
	}
	return err
}

func health(ctx context.Context, output io.Writer) error {
	engine, err := openEngine(ctx)
	if err != nil {
		return err
	}
	workspace := engine.Workspace()
	fmt.Fprintf(output, "workspace\tready\t%s\t%s\n", workspace.Root, workspace.Branch)
	policy, ref, err := engine.Policy()
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "policy\tready\t%s\t%s\n", ref.Version, policy.Workflow.Topology)
	ready := 0
	for _, status := range engine.Providers(ctx) {
		if status.Available {
			ready++
		}
		fmt.Fprintf(output, "provider.%s\t%s\t%s\n", status.ID, availability(status.Available), status.Reason)
	}
	fmt.Fprintf(output, "providers.ready\t%d\n", ready)
	return nil
}
func listRuns(engine *core.Engine, output io.Writer) error {
	runs, err := engine.Store().ListRuns()
	if err != nil {
		return err
	}
	for _, run := range runs {
		fmt.Fprintf(output, "%s\t%s\t%s\t%s\t%s\n", run.ID, run.Provider, run.State, run.Workspace.Kind, run.Verification.State)
	}
	return nil
}
func events(engine *core.Engine, id string, output io.Writer) error {
	values, err := engine.Store().Events(id)
	if err != nil {
		return err
	}
	for _, event := range values {
		fmt.Fprintf(output, "%s\t%s\t%s\t%s\n", event.At.Format(time.RFC3339), event.Type, event.State, event.Message)
	}
	return nil
}
func listWorktrees(engine *core.Engine, output io.Writer) error {
	runs, err := engine.Store().ListRuns()
	if err != nil {
		return err
	}
	values := []core.Run{}
	for _, run := range runs {
		if run.Workspace.Kind == "worktree" {
			values = append(values, run)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Workspace.Root < values[j].Workspace.Root })
	for _, run := range values {
		fmt.Fprintf(output, "%s\t%s\t%s\t%s\n", run.ID, run.State, run.Workspace.Branch, run.Workspace.Root)
	}
	return nil
}
func availability(value bool) string {
	if value {
		return "ready"
	}
	return "unavailable"
}
func transport(value core.ProviderCapabilities) string {
	if value.Structured {
		return "structured"
	}
	return "terminal"
}

func usage(output io.Writer) {
	fmt.Fprint(output, `Gator standalone coding-agent harness

Usage:
  gator                                  open the terminal-native TUI
  gator run [--provider P] [--file F] [--worktree] [--execute] "objective"
  gator providers | health | runs | events <run-id> | worktrees
  gator review [--verify] <run-id>
  gator handoff [--execute] <run-id> <provider>
  gator policy <init|show|validate>
  gator experiment compare --provider P --baseline policy.json --candidate policy.json [--execute] "objective"

Provider output is never persisted as a structured transcript. Only --execute
starts a provider process; dry runs record a metadata-only, reviewable plan.
`)
}

// Keep filepath referenced in this command package as a compile-time guard that
// standalone command handling remains filesystem-native rather than Neovim-bound.
var _ = filepath.Separator
