package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/uisnapshot"
	sharedusage "github.com/gongahkia/gator/internal/usage"
	"github.com/gongahkia/gator/internal/worktui"
)

func workControls(stateDir string) func(context.Context, string, string, []string) (worktui.ControlScreen, error) {
	return func(ctx context.Context, family, action string, args []string) (worktui.ControlScreen, error) {
		if err := ctx.Err(); err != nil {
			return worktui.ControlScreen{}, err
		}
		switch family {
		case "overview":
			return overviewControl(ctx, stateDir, action, args)
		case "recovery":
			return recoveryControl(ctx, stateDir, action, args)
		case "data":
			return dataControl(ctx, stateDir, action, args)
		case "usage":
			return usageControl(stateDir, action, args)
		default:
			return worktui.ControlScreen{}, fmt.Errorf("%s controls are not available yet", family)
		}
	}
}
func usageControl(stateDir, action string, args []string) (worktui.ControlScreen, error) {
	s, err := sharedusage.Open(stateDir)
	if err != nil {
		return worktui.ControlScreen{}, err
	}
	if action == "limits" || action == "pause" {
		if len(args) < 1 {
			return worktui.ControlScreen{}, errors.New("refresh the shared limits before editing")
		}
		var prior sharedusage.Policy
		if err = json.Unmarshal([]byte(args[0]), &prior); err != nil {
			return worktui.ControlScreen{}, err
		}
		next := prior
		if action == "pause" {
			next.Paused = !next.Paused
		} else {
			if len(args) != 4 {
				return worktui.ControlScreen{}, errors.New("provide daily requests, daily tokens, and concurrent requests")
			}
			var parseErr error
			next.DailyRequests, parseErr = strconv.Atoi(args[1])
			if parseErr != nil {
				return worktui.ControlScreen{}, parseErr
			}
			next.DailyTokens, parseErr = strconv.ParseInt(args[2], 10, 64)
			if parseErr != nil {
				return worktui.ControlScreen{}, parseErr
			}
			next.ConcurrentRequests, parseErr = strconv.Atoi(args[3])
			if parseErr != nil {
				return worktui.ControlScreen{}, parseErr
			}
		}
		if err = s.SaveReviewedPolicy(next, prior); err != nil {
			return worktui.ControlScreen{}, err
		}
	}
	now := time.Now().UTC()
	view, err := s.Snapshot(now)
	if err != nil {
		return worktui.ControlScreen{}, err
	}
	p := view.Policy
	usage := &uisnapshot.Usage{
		Day: view.Day, ResetAt: time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Paused: p.Paused, Requests: view.Requests, Tokens: view.Tokens,
		ActiveRequests: view.ActiveRequests, FailedRequests: view.Failed, EstimatedRequests: view.EstimatedRequests,
		DailyRequestLimit: p.DailyRequests, DailyTokenLimit: p.DailyTokens, ConcurrentLimit: p.ConcurrentRequests,
		KnownCostUSD: view.KnownCostUSD,
	}
	previous, _ := json.Marshal(p)
	status := "Requests allowed within the shared limits"
	if p.Paused {
		status = "All new provider requests paused; current requests may finish"
	}
	lines := []string{status, "Day: " + view.Day + " (UTC)", fmt.Sprintf("%d provider requests · %d active · %d failed attempts", view.Requests, view.ActiveRequests, view.Failed), fmt.Sprintf("%d charged tokens · %d requests have estimated or unreported usage", view.Tokens, view.EstimatedRequests), fmt.Sprintf("Limits: %s requests/day · %s tokens/day · %s concurrent requests", limitLabel(int64(p.DailyRequests)), limitLabel(p.DailyTokens), limitLabel(int64(p.ConcurrentRequests))), "Counters include chat, background jobs, routing, retrieval, Code, and repeated attempts.", "Request and concurrency limits are reserved atomically across local Gator processes using this state directory.", "Tokens use input-byte and response reservations until provider usage arrives. Missing usage keeps its reservation. Provider totals can exceed estimates; this is not a billing ceiling.", "A paused or exhausted shared budget also stops automatic routing and embeddings. Source-only polls can continue.", "No complete monetary cost is available when usage or pricing is missing."}
	if view.KnownCostUSD != nil {
		lines = append(lines, fmt.Sprintf("Retained provider-reported cost: $%.4f", *view.KnownCostUSD))
	}
	for _, category := range []string{"chat", "work", "background", "routing", "retrieval", "code"} {
		lines = append(lines, fmt.Sprintf("%s: %d requests", category, view.Categories[category]))
		usage.Categories = append(usage.Categories, uisnapshot.UsageCategory{Name: category, Requests: view.Categories[category]})
	}
	screen := worktui.ControlScreen{Panel: uisnapshot.Panel{Kind: "usage", Title: "Shared usage and limits", Usage: usage, Blocks: []uisnapshot.Block{{Title: "All work on this computer", Lines: lines}}}}
	pauseLabel := "Pause provider requests"
	if p.Paused {
		pauseLabel = "Resume provider requests"
	}
	screen.Actions = []worktui.ControlAction{
		{Key: "E", Label: "Edit shared limits", Action: "limits", Args: []string{string(previous)}, Description: "These limits apply across foreground and background requests. Zero means uncapped. Daily counters reset at 00:00 UTC; existing per-run limits still apply.", Fields: []worktui.ControlField{{Label: "Provider requests per day (0 uncapped)", Kind: "number", Value: strconv.Itoa(p.DailyRequests)}, {Label: "Tokens per day (0 uncapped)", Kind: "number", Value: strconv.FormatInt(p.DailyTokens, 10)}, {Label: "Concurrent requests (0 uncapped)", Kind: "number", Value: strconv.Itoa(p.ConcurrentRequests)}}, Review: true},
		{Key: "P", Label: pauseLabel, Action: "pause", Args: []string{string(previous)}, Description: "Pause or resume new requests across chat, routing, retrieval, specialists, and jobs. An in-flight provider request may finish. This does not change saved schedules.", Review: true},
		{Key: "R", Label: "Refresh", Action: "status"},
	}
	return screen, nil
}
func limitLabel(value int64) string {
	if value == 0 {
		return "uncapped"
	}
	return strconv.FormatInt(value, 10)
}
func usageCommand(args []string, out io.Writer) error {
	root, err := state.ResolveDir("")
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "limits" {
		s, err := sharedusage.Open(root)
		if err != nil {
			return err
		}
		p, err := s.Policy()
		if err != nil {
			return err
		}
		flags := flag.NewFlagSet("usage limits", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		requests := flags.Int("requests", p.DailyRequests, "shared requests per UTC day; zero uncapped")
		tokens := flags.Int64("tokens", p.DailyTokens, "shared token admission allowance; zero uncapped")
		concurrent := flags.Int("concurrent", p.ConcurrentRequests, "shared simultaneous requests; zero uncapped")
		if err = flags.Parse(args[1:]); err != nil {
			return err
		}
		if len(flags.Args()) > 0 {
			return errors.New("unexpected usage limit arguments")
		}
		prior, _ := json.Marshal(p)
		_, err = usageControl(root, "limits", []string{string(prior), strconv.Itoa(*requests), strconv.FormatInt(*tokens, 10), strconv.Itoa(*concurrent)})
		if err != nil {
			return err
		}
	} else if len(args) > 0 && (args[0] == "pause" || args[0] == "resume") {
		s, err := sharedusage.Open(root)
		if err != nil {
			return err
		}
		p, err := s.Policy()
		if err != nil {
			return err
		}
		next := p
		next.Paused = args[0] == "pause"
		if err = s.SaveReviewedPolicy(next, p); err != nil {
			return err
		}
	} else if len(args) > 0 && args[0] != "status" {
		return errors.New("usage: gator usage [status|limits --requests N --tokens N --concurrent N|pause|resume]")
	}
	screen, err := usageControl(root, "status", nil)
	if err != nil {
		return err
	}
	for _, b := range screen.Panel.Blocks {
		for _, line := range b.Lines {
			if _, err = fmt.Fprintln(out, line); err != nil {
				return err
			}
		}
	}
	return nil
}
