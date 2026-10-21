package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workhome"
	"github.com/gongahkia/gator/internal/worktui"
)

func readEverydayOverview(ctx context.Context, root string) (workhome.Overview, error) {
	release, err := state.Guard(root, false)
	if err != nil {
		return workhome.Overview{}, err
	}
	defer release()
	running := jobSupervisorRequestAt(root, "health", io.Discard) == nil
	view, err := workhome.ReadOverview(ctx, root, running, time.Now())
	if err != nil {
		return view, err
	}
	settings, err := config.DefaultStore()
	if err == nil {
		status, statusErr := currentWorkModelStatus(settings, root)
		if statusErr != nil {
			err = statusErr
		} else if status.Provider == "" || strings.Contains(strings.ToLower(status.Access), "not authenticated") || strings.Contains(strings.ToLower(status.Access), "expired") || strings.Contains(strings.ToLower(status.Access), "not supported") || strings.Contains(strings.ToLower(status.Access), "not set") || strings.Contains(strings.ToLower(status.Access), "incomplete") || strings.Contains(strings.ToLower(status.Access), "not configured") {
			section := &view.Sections[3]
			section.Items = append(section.Items, workhome.OverviewItem{ID: "model", Kind: "model", Title: "Choose or reconnect a Work model", Detail: "Open model setup before starting new model work."})
			section.Total++
			section.Summary = fmt.Sprintf("%d · %s", section.Total, section.Items[0].Title)
		}
	}
	if err != nil {
		section := &view.Sections[3]
		section.Items = append(section.Items, workhome.OverviewItem{ID: "model", Kind: "model", Title: "Model setup could not be read", Detail: err.Error()})
		section.Total++
		section.Summary = fmt.Sprintf("%d · %s", section.Total, section.Items[0].Title)
	}
	return view, nil
}
func overviewControl(ctx context.Context, root, action string, args []string) (worktui.ControlScreen, error) {
	if action == "seen" {
		if len(args) != 1 {
			return worktui.ControlScreen{}, errors.New("refresh the overview before marking it reviewed")
		}
		at, err := time.Parse(time.RFC3339Nano, args[0])
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		release, err := state.Guard(root, false)
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		err = workhome.MarkOverviewSeen(root, at)
		release()
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		args = nil
	}
	view, err := readEverydayOverview(ctx, root)
	if err != nil {
		return worktui.ControlScreen{}, err
	}
	screen := worktui.ControlScreen{Location: strings.Join(args, "\x00"), Panel: uisnapshot.Panel{Kind: "overview", Title: "Everyday overview", Subtitle: "Retained facts · choose an item to act"}}
	if len(args) == 0 {
		screen.Panel.Title, screen.Panel.Subtitle = "Overview", "Changes from the last 24 hours"
		if view.Reviewed {
			screen.Panel.Subtitle = "Changes since " + view.Since.Local().Format("Jan 2 15:04")
		}
		for index, section := range view.Sections {
			screen.Actions = append(screen.Actions, worktui.ControlAction{Key: strconv.Itoa(index + 1), Label: section.Title, Detail: section.Summary, Action: "section", Args: []string{section.ID}})
		}
		screen.Actions = append(screen.Actions, worktui.ControlAction{Key: "M", Label: "Mark current changes reviewed", Action: "seen", Args: []string{view.At.Format(time.RFC3339Nano)}})
		screen.Panel.Footer = "Changes since " + view.Since.Local().Format(time.RFC1123) + ". Reviewing changes does not mark inbox results read or approve anything."
	} else {
		var selected *workhome.OverviewSection
		for i := range view.Sections {
			if view.Sections[i].ID == args[0] {
				selected = &view.Sections[i]
				break
			}
		}
		if selected == nil {
			return screen, errors.New("overview section is unavailable; refresh")
		}
		screen.Panel.Title = selected.Title
		if len(args) == 1 {
			for index, item := range selected.Items {
				detail := item.Detail
				if !item.At.IsZero() {
					detail += " · " + item.At.Local().Format("Jan 2 15:04")
				}
				key := ""
				if index < 9 {
					key = strconv.Itoa(index + 1)
				}
				screen.Actions = append(screen.Actions, worktui.ControlAction{Key: key, Label: item.Title, Detail: detail, Action: "item", Args: []string{selected.ID, item.ID}})
			}
			if len(selected.Items) == 0 {
				screen.Panel.Blocks = []uisnapshot.Block{{Lines: []string{selected.Summary}}}
			}
			if selected.Total > len(selected.Items) {
				screen.Panel.Footer = fmt.Sprintf("Showing the first %d of %d items. Individual stores retain the full history.", len(selected.Items), selected.Total)
			}
		} else {
			var item *workhome.OverviewItem
			for i := range selected.Items {
				if selected.Items[i].ID == args[1] {
					item = &selected.Items[i]
					break
				}
			}
			if item == nil {
				screen.Panel.Notice = "This item is no longer in the current view. Refresh to see what is waiting."
			} else {
				screen.Panel.Title = item.Title
				lines := []string{item.Detail}
				if !item.At.IsZero() {
					lines = append(lines, item.At.Local().Format(time.RFC1123))
				}
				screen.Panel.Blocks = []uisnapshot.Block{{Title: selected.Title, Lines: lines}}
				open := worktui.ControlAction{Key: "Enter", Label: "Open", Action: "navigate"}
				switch item.Kind {
				case "inbox":
					open.Label = "Read retained result"
					open.Command = "/inbox show " + item.Target
					screen.Actions = append(screen.Actions, worktui.ControlAction{Key: "D", Label: "Mark result read", Command: "/inbox read " + item.Target})
				case "suggestion":
					open.Label = "Inspect proposed change"
					open.Command = "/suggestions show " + item.Target
				case "source":
					open.Label = "Relink the missing source folder"
					open.Family = "data"
					open.Action = "relink-source"
					open.Args = []string{item.SourcePath}
				case "goal":
					open.Label = "Continue goal"
					open.GoalID = item.Target
				case "plan":
					open.Label = "Review goal plan"
					open.GoalID = item.Target
					open.Command = "/plan"
				case "work":
					open.Label = "Inspect Work and retained output"
					open.WorkID = item.Target
				case "decision":
					open.Label = "Inspect Work"
					open.WorkID = item.Target
					screen.Actions = append(screen.Actions, worktui.ControlAction{Key: "A", Label: "Open exact pending review", External: []string{"work", "answer", item.Target, strconv.Itoa(item.DecisionID)}, Review: true, Description: "Hand the terminal to the existing Work decision review for this live run. Inspect the full action and choose an explicit response there. Opening the review does not approve it."})
				case "check":
					open.Label = "Recover or retry this check"
					open.Command = "/recovery " + item.Target
				case "recovery":
					open.Label = "Open recovery and supervisor controls"
					open.Command = "/recovery"
				case "usage":
					open.Label = "Review shared usage and limits"
					open.Command = "/usage"
				case "model":
					open.Label = "Open Work model setup"
					open.Command = "/model"
				case "data":
					open.Label = "Review restored profile"
					open.Command = "/data"
				default:
					open.Label = "Refresh overview"
					open.Command = "/overview"
				}
				screen.Actions = append([]worktui.ControlAction{open}, screen.Actions...)
			}
		}
		screen.Actions = append(screen.Actions, worktui.ControlAction{Key: "B", Label: "All four sections", Action: "status"})
	}
	refreshed := "Refreshed " + view.At.Local().Format("15:04:05")
	if screen.Panel.Footer != "" {
		refreshed += " · " + screen.Panel.Footer
	}
	screen.Panel.Footer = refreshed
	if len(args) == 0 {
		screen.Panel.Blocks = append(screen.Panel.Blocks, uisnapshot.Block{Title: "About this overview", Collapsible: true, Lines: []string{screen.Panel.Footer, "This view reads retained activity without making a model request."}})
		screen.Panel.Footer = ""
	}
	screen.Actions = append(screen.Actions, worktui.ControlAction{Key: "R", Label: "Refresh", Action: "status", Args: append([]string(nil), args...)})
	return screen, nil
}
func overviewTerminalCommand(root string, args []string) *exec.Cmd {
	if len(args) != 4 || args[0] != "work" || args[1] != "answer" {
		return nil
	}
	binary, err := os.Executable()
	if err != nil {
		return nil
	}
	command := exec.Command(binary, args...)
	command.Env = append(os.Environ(), "GATOR_STATE_DIR="+root)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command
}
func overviewCommand(args []string, out io.Writer) error {
	if len(args) > 1 || len(args) == 1 && args[0] != "--json" && args[0] != "seen" {
		return errors.New("usage: gator overview [--json|seen]")
	}
	root, err := state.ResolveDir("")
	if err != nil {
		return err
	}
	view, err := readEverydayOverview(context.Background(), root)
	if err != nil {
		return err
	}
	if len(args) == 1 && args[0] == "seen" {
		if err = workhome.MarkOverviewSeen(root, view.At); err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "Current changes marked reviewed. Unread results and approvals remain pending.")
		return err
	}
	if len(args) == 1 {
		return json.NewEncoder(out).Encode(view)
	}
	for _, section := range view.Sections {
		if _, err = fmt.Fprintf(out, "%s — %s\n", section.Title, section.Summary); err != nil {
			return err
		}
		for _, item := range section.Items {
			if _, err = fmt.Fprintf(out, "  %s · %s\n", item.Title, item.Detail); err != nil {
				return err
			}
		}
	}
	_, err = fmt.Fprintln(out, "Open /overview in Gator for direct actions. No result, proposal, or external action was accepted by reading this view.")
	return err
}
