// Package tui is the terminal-native interactive client for the standalone core.
package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gongahkia/gator/internal/core"
)

func Run(ctx context.Context, engine *core.Engine, input io.Reader, output, errors io.Writer) error {
	reader := bufio.NewReader(input)
	for {
		if err := render(ctx, engine, output); err != nil {
			return err
		}
		fmt.Fprint(output, "\n[gator] command (run, runs, events, review, providers, policy, quit): ")
		command, err := readLine(reader)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch command {
		case "", "refresh":
			continue
		case "quit", "q", "exit":
			return nil
		case "runs", "l":
			if err := showRuns(engine, output); err != nil {
				return err
			}
			wait(reader, output)
		case "providers", "p":
			showProviders(ctx, engine, output)
			wait(reader, output)
		case "events", "e":
			fmt.Fprint(output, "run id: ")
			id, _ := readLine(reader)
			showEvents(engine, id, output)
			wait(reader, output)
		case "review":
			fmt.Fprint(output, "run id: ")
			id, _ := readLine(reader)
			if _, err := engine.VerifyAll(ctx, id, output, errors); err != nil {
				fmt.Fprintln(output, "review:", err)
			}
			wait(reader, output)
		case "policy":
			policy, ref, err := engine.Policy()
			if err != nil {
				return err
			}
			fmt.Fprintf(output, "\nPolicy %s (%s)\nTopology: %s\nContext: %d files / %d bytes / diff=%t\nVerification commands: %d\n", ref.Version, ref.Source, policy.Workflow.Topology, policy.Context.MaxFiles, policy.Context.MaxBytes, policy.Context.IncludeDiff, len(policy.Verification.Commands))
			wait(reader, output)
		case "run", "r":
			if err := startRun(ctx, engine, reader, input, output, errors); err != nil {
				fmt.Fprintln(output, "run:", err)
				wait(reader, output)
			}
		default:
			fmt.Fprintln(output, "unknown command")
			wait(reader, output)
		}
	}
}

func render(ctx context.Context, engine *core.Engine, output io.Writer) error {
	workspace := engine.Workspace()
	runs, err := engine.Store().ListRuns()
	if err != nil {
		return err
	}
	fmt.Fprint(output, "\033[H\033[2J")
	fmt.Fprintln(output, "Gator · standalone terminal-native coding-agent harness")
	fmt.Fprintf(output, "%s · %s · %s · %s\n", workspace.Root, workspace.Branch, short(workspace.Head), dirty(workspace.Dirty))
	policy, ref, err := engine.Policy()
	if err == nil {
		fmt.Fprintf(output, "policy %s · %s · %d verification command(s)\n", ref.Version, policy.Workflow.Topology, len(policy.Verification.Commands))
	}
	active, completed, failed := 0, 0, 0
	for _, run := range runs {
		switch run.State {
		case "running":
			active++
		case "completed":
			completed++
		case "failed":
			failed++
		}
	}
	fmt.Fprintf(output, "runs: %d active · %d completed · %d failed\n", active, completed, failed)
	fmt.Fprintln(output, "\nRecent runs")
	if len(runs) == 0 {
		fmt.Fprintln(output, "  no standalone runs yet")
	}
	for index, run := range runs {
		if index == 8 {
			break
		}
		fmt.Fprintf(output, "  %-28s %-10s %-10s %-13s verify=%s\n", run.ID, run.Provider, run.State, run.Workspace.Kind, run.Verification.State)
	}
	return nil
}

func startRun(ctx context.Context, engine *core.Engine, reader *bufio.Reader, providerInput io.Reader, output, errors io.Writer) error {
	fmt.Fprint(output, "objective: ")
	objective, err := readLine(reader)
	if err != nil {
		return err
	}
	policy, _, err := engine.Policy()
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "provider [%s]: ", policy.Routing.DefaultProvider)
	provider, _ := readLine(reader)
	if provider == "" {
		provider = policy.Routing.DefaultProvider
	}
	fmt.Fprint(output, "context files (comma-separated PATH or PATH:START:END; empty none): ")
	filesInput, _ := readLine(reader)
	files, err := parseFiles(filesInput)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "include Git diff [%t]: ", policy.Context.IncludeDiff)
	diffInput, _ := readLine(reader)
	includeDiff := policy.Context.IncludeDiff
	if diffInput == "y" || diffInput == "yes" {
		includeDiff = true
	}
	if diffInput == "n" || diffInput == "no" {
		includeDiff = false
	}
	fmt.Fprint(output, "isolated worktree? [y/N]: ")
	worktreeInput, _ := readLine(reader)
	fmt.Fprint(output, "execute provider now? [y/N]: ")
	executeInput, _ := readLine(reader)
	if executeInput == "y" || executeInput == "yes" {
		fmt.Fprintln(output, "\nGator is suspending its dashboard so the provider keeps its native terminal UI. Gator will not persist the provider output.")
	}
	run, err := engine.Run(ctx, core.RunRequest{Objective: objective, Provider: provider, Files: files, IncludeDiff: &includeDiff, Worktree: worktreeInput == "y" || worktreeInput == "yes", Execute: executeInput == "y" || executeInput == "yes"}, providerInput, output, errors)
	fmt.Fprintf(output, "\nrun %s · %s · verification=%s\n", run.ID, run.State, run.Verification.State)
	return err
}

func showRuns(engine *core.Engine, output io.Writer) error {
	runs, err := engine.Store().ListRuns()
	if err != nil {
		return err
	}
	for _, run := range runs {
		fmt.Fprintf(output, "%s\t%s\t%s\t%s\t%s\n", run.ID, run.Provider, run.State, run.Workspace.Root, run.Verification.State)
	}
	return nil
}

func showProviders(ctx context.Context, engine *core.Engine, output io.Writer) {
	for _, provider := range engine.Providers(ctx) {
		fmt.Fprintf(output, "%-10s available=%-5t discovered=%-5t transport=%-10s executable=%s %s\n", provider.ID, provider.Available, provider.Discovered, providerTransport(provider.Capabilities), provider.Executable, provider.Reason)
	}
}

func showEvents(engine *core.Engine, id string, output io.Writer) {
	events, err := engine.Store().Events(id)
	if err != nil {
		fmt.Fprintln(output, err)
		return
	}
	for _, event := range events {
		fmt.Fprintf(output, "%s\t%s\t%s\t%s\n", event.At.Format("15:04:05"), event.Type, event.State, event.Message)
	}
}

func parseFiles(value string) ([]core.FileReference, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	items := strings.Split(value, ",")
	refs := make([]core.FileReference, 0, len(items))
	for _, item := range items {
		ref, err := core.ParseFileReference(strings.TrimSpace(item))
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func wait(reader *bufio.Reader, output io.Writer) {
	fmt.Fprint(output, "\npress Enter to continue")
	_, _ = readLine(reader)
}
func readLine(reader *bufio.Reader) (string, error) {
	value, err := reader.ReadString('\n')
	return strings.TrimSpace(value), err
}
func short(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}
func dirty(value bool) string {
	if value {
		return "dirty"
	}
	return "clean"
}
func providerTransport(capability core.ProviderCapabilities) string {
	if capability.Structured {
		return "structured"
	}
	return "terminal"
}
