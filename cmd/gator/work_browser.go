package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/browser"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workapp"
)

func browserControl(ctx context.Context, stateDir, operation string, args []string) (workapp.ControlScreen, error) {
	release, err := state.Guard(stateDir, false)
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	defer release()
	store, err := browser.Open(stateDir)
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	id, notice := "", ""
	if len(args) > 0 {
		id = args[0]
	}
	var grant *workapp.BrowserGrant
	switch operation {
	case "status", "select":
		if len(args) > 1 {
			return workapp.ControlScreen{}, errors.New("select one browser session")
		}
	case "install":
		if len(args) != 0 {
			return workapp.ControlScreen{}, errors.New("unexpected browser install arguments")
		}
		installCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
		defer cancel()
		if _, err = browser.InstallRuntime(installCtx, store); err != nil {
			return workapp.ControlScreen{}, err
		}
		notice = "Browser installed. Open a private window to continue."
	case "start":
		if len(args) != 0 {
			return workapp.ControlScreen{}, errors.New("unexpected browser start arguments")
		}
		session, err := openManagedBrowser(ctx, store, stateDir, browser.StartOptions{Headed: true, Paused: true})
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		id = session.ID
		notice = "Private window opened with agent access paused. Add the sites needed for sign-in, then sign in in that window."
	case "clear-grant":
		grant = &workapp.BrowserGrant{}
		notice = "Browser removed from the next task. Already submitted tasks keep their captured grant; pause or stop the browser to block them."
	default:
		if len(args) < 2 {
			return workapp.ControlScreen{}, errors.New("reopen Browser and review the current session")
		}
		client, err := browser.NewClient(store, id)
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		client = client.Reviewed(args[1])
		switch operation {
		case "pause", "resume", "visual", "grant", "stop", "tabs":
			if len(args) != 2 {
				return workapp.ControlScreen{}, errors.New("browser review changed; reopen Browser")
			}
		case "add-origin", "remove-origin", "select-tab":
			if len(args) != 3 {
				return workapp.ControlScreen{}, errors.New("provide the reviewed browser selection")
			}
		default:
			return workapp.ControlScreen{}, errors.New("unsupported browser action")
		}
		switch operation {
		case "pause", "resume":
			// Pausing only removes access, so an old display must not prevent
			// the user from stopping reads after another window edits the policy.
			if operation == "pause" {
				client = client.Reviewed("")
			}
			_, err = client.SetPaused(ctx, operation == "pause")
			notice = "Agent access resumed. Review and grant this browser to the next task when ready."
			if operation == "pause" {
				notice = "Agent access paused. You can now sign in or take over privately in the browser window."
			}
		case "visual":
			var current browser.Session
			current, err = client.Session(ctx, id)
			if err == nil {
				_, err = client.SetVisualCapture(ctx, !current.VisualCapture)
			}
		case "add-origin":
			_, err = client.AddOrigin(ctx, strings.TrimSpace(args[2]))
		case "remove-origin":
			_, err = client.RemoveOrigin(ctx, args[2])
		case "tabs":
			var tabs []browser.Tab
			tabs, err = client.CandidateTabs(ctx)
			if err == nil {
				screen := workapp.ControlScreen{Location: id, Panel: uisnapshot.Panel{Kind: "browser", Title: "Choose a browser tab", Subtitle: "Only the tab you choose will be shared. Tab titles and addresses below are shown to you, not the agent."}}
				for i, tab := range tabs {
					raw, _ := json.Marshal(tab)
					screen.Actions = append(screen.Actions, workapp.ControlAction{Key: fmt.Sprint(i + 1), Label: tab.Title, Detail: tab.URL, Action: "select-tab", Args: []string{id, args[1], string(raw)}, Review: true, Description: "Share only this tab: " + tab.Title + " — " + tab.URL + ". This replaces the session's selected tabs and invalidates previously reviewed task grants."})
				}
				screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "B", Label: "Back to browser", Action: "status", Args: []string{id}})
				return screen, nil
			}
		case "select-tab":
			var selected browser.Tab
			if err = json.Unmarshal([]byte(args[2]), &selected); err == nil {
				var candidates []browser.Tab
				candidates, err = client.CandidateTabs(ctx)
				if err == nil {
					found := false
					for _, tab := range candidates {
						if tab == selected {
							found = true
						}
					}
					if !found {
						err = errors.New("the tab changed or closed; choose it again")
					} else {
						_, err = client.SelectTabs(ctx, []browser.Tab{selected})
					}
				}
			}
		case "grant":
			var current browser.Session
			current, err = client.Session(ctx, id)
			if err == nil && (current.State != browser.StateRunning || current.Paused || len(current.SelectedTabs) == 0 || len(current.Origins) == 0) {
				err = errors.New("resume this browser and select a tab and allowed site before granting access")
			}
			if err == nil {
				var tabs []browser.Tab
				tabs, err = client.CandidateTabs(ctx)
				for _, selected := range current.SelectedTabs {
					found := false
					for _, tab := range tabs {
						found = found || tab.ID == selected.ID
					}
					if !found && err == nil {
						err = errors.New("a selected tab closed; choose a shared tab again")
					}
				}
			}
			if err == nil {
				grant = &workapp.BrowserGrant{SessionID: id, Policy: args[1]}
				notice = "Browser selected for the next submitted task only. Sending and other browser changes still require their own approval."
			}
		case "stop":
			// Revocation only removes access and must also work against an
			// older daemon that cannot honor newly reviewed task grants.
			client = client.Reviewed("")
			_, err = client.Stop(ctx)
			if errors.Is(err, browser.ErrUnavailable) {
				// Explicit revocation also works when the controller cannot be reached.
				current, loadErr := store.Get(id)
				if loadErr != nil {
					err = loadErr
				} else if browser.PolicyDigest(current) != args[1] {
					err = errors.New("browser access changed; reopen its controls")
				} else {
					_, err = store.Stop(id)
					if err == nil {
						err = browser.RemoveToken(store, id)
					}
				}
			}
			if err == nil {
				id = ""
				notice = "Browser access revoked. Open a new private window to start again."
			}
		}
		if err != nil {
			return workapp.ControlScreen{}, err
		}
	}
	screen, err := browserControlScreen(ctx, store, id)
	if err != nil {
		return screen, err
	}
	screen.Panel.Notice = notice
	screen.BrowserGrant = grant
	return screen, nil
}

func browserControlScreen(ctx context.Context, store *browser.Store, id string) (workapp.ControlScreen, error) {
	screen := workapp.ControlScreen{Location: id, Panel: uisnapshot.Panel{Kind: "browser", Title: "Browser", Subtitle: "Use a separate browser window. Pause access before signing in or taking over."}}
	if id == "" {
		status := browser.Runtime(store)
		if !status.Installed {
			screen.Panel.Blocks = []uisnapshot.Block{{Title: "Setup", Lines: []string{"Install the browser once to use local browsing.", status.Error}}}
			screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "I", Label: "Install browser", Action: "install", Review: true, Description: "Download the pinned Playwright and Chromium runtime into Gator's private storage. This can take several minutes and requires internet access."})
		} else {
			screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "N", Label: "Open private browser", Action: "start", Review: true, Description: "Open a separate Chromium window with a temporary profile. Agent access starts paused and screenshots start off. Sign-in data is kept in that temporary profile until the browser closes."})
		}
		sessions, err := store.List()
		if err != nil {
			return screen, err
		}
		for i, session := range sessions {
			if session.State != browser.StateRunning {
				continue
			}
			state := "Access enabled"
			if session.Paused {
				state = "Paused for private takeover"
			}
			screen.Actions = append(screen.Actions, workapp.ControlAction{Key: fmt.Sprint(i + 1), Label: session.ID, Detail: state + " · open to check connection", Action: "select", Args: []string{session.ID}})
		}
		screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "C", Label: "Clear next-task browser", Action: "clear-grant"}, workapp.ControlAction{Key: "R", Label: "Refresh", Action: "status"})
		return screen, nil
	}
	recorded, err := store.Get(id)
	if err != nil {
		return screen, err
	}
	client, err := browser.NewClient(store, id)
	if err != nil {
		return screen, err
	}
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	current, liveErr := client.Session(checkCtx, id)
	if liveErr != nil || current.State != browser.StateRunning {
		screen.Panel.Blocks = []uisnapshot.Block{{Title: "Browser unavailable", Lines: []string{"Gator could not confirm a live browser connection. Refresh to retry, or revoke access before opening a new window.", "Revoking access does not establish whether the last browser action completed."}}}
		if recorded.State == browser.StateRunning {
			screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "S", Label: "Revoke browser access", Action: "stop", Args: []string{id, browser.PolicyDigest(recorded)}, Review: true, Description: "Revoke this browser session's local access. A managed window closes when its controller can be reached; an unavailable controller may require closing the window yourself."})
		}
	} else {
		policy := browser.PolicyDigest(current)
		args := []string{id, policy}
		access := "Enabled"
		if current.Paused {
			access = "Paused — private takeover"
		}
		lines := []string{"Agent access: " + access, fmt.Sprintf("Model-visible screenshots: %t", current.VisualCapture)}
		tabs, tabsErr := client.CandidateTabs(checkCtx)
		for _, selected := range current.SelectedTabs {
			label := selected.ID + " (refresh to check this tab)"
			if tabsErr == nil {
				label = selected.ID + " (closed or unavailable)"
				for _, tab := range tabs {
					if tab.ID == selected.ID {
						label = tab.ID + " — " + tab.Title + " — " + tab.URL
					}
				}
			}
			lines = append(lines, "Selected tab: "+label)
		}
		for _, origin := range current.Origins {
			lines = append(lines, "Allowed site: "+origin.URL)
		}
		lines = append(lines, "Sign in in the browser window while access is paused. Add any sign-in redirect sites explicitly.", "Browser changes invalidate previously reviewed task grants. A stopped window cannot recover its temporary login state.")
		screen.Panel.Blocks = []uisnapshot.Block{{Title: "Current access", Lines: lines}}
		review := strings.Join(lines[:len(lines)-2], "\n")
		if current.Paused {
			screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "P", Label: "Resume agent access", Action: "resume", Args: args, Review: true, Description: review + "\nResume only after finishing private sign-in. Old element references are invalidated before access resumes."})
		} else {
			screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "P", Label: "Pause for private takeover", Action: "pause", Args: args})
			if len(current.SelectedTabs) > 0 && len(current.Origins) > 0 {
				screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "G", Label: "Use for next task", Action: "grant", Args: args, Review: true, Description: review + "\nShare these selected tabs with only the next task you submit in this chat. Page text may be sent to the selected model. This does not grant browser access to Code specialists."})
			}
		}
		screen.Actions = append(screen.Actions,
			workapp.ControlAction{Key: "A", Label: "Add allowed site", Action: "add-origin", Args: args, Review: true, Description: "Allow this browser to access the entire site origin, including its subresources. This affects the shared browser session and invalidates earlier task grants.", Fields: []workapp.ControlField{{Label: "Site URL", Kind: "text", Help: "For example https://example.com; approval covers the origin, not only a page."}}},
			workapp.ControlAction{Key: "T", Label: "Choose shared tab", Action: "tabs", Args: args},
			workapp.ControlAction{Key: "V", Label: map[bool]string{true: "Turn screenshots off", false: "Allow screenshots"}[current.VisualCapture], Action: "visual", Args: args, Review: true, Description: review + "\nToggle whether agent screenshots may be sent to the model. Page text remains readable when access is enabled."},
			workapp.ControlAction{Key: "S", Label: "Stop browser", Action: "stop", Args: args, Review: true, Description: "Stop and revoke this session. A managed window closes; a new window starts with fresh sign-in state. An attached browser is disconnected. Already completed external actions are not undone."})
		for i, origin := range current.Origins {
			screen.Actions = append(screen.Actions, workapp.ControlAction{Key: fmt.Sprint(i + 1), Label: "Remove " + origin.URL, Action: "remove-origin", Args: []string{id, policy, origin.URL}, Review: true, Description: "Remove this allowed site and invalidate earlier task grants."})
		}
	}
	screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "C", Label: "Clear next-task browser", Action: "clear-grant", Args: []string{id}}, workapp.ControlAction{Key: "R", Label: "Refresh", Action: "status", Args: []string{id}}, workapp.ControlAction{Key: "B", Label: "All browsers", Action: "status"})
	return screen, nil
}
