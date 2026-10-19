package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/worktui"
)

func recoveryServicePath() (string, error) {
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		return filepath.Join(home, "Library", "LaunchAgents", "com.gator.work-supervisor.plist"), err
	}
	base, err := os.UserConfigDir()
	return filepath.Join(base, "systemd", "user", jobSystemdUnitName), err
}
func managedJobCommand(ctx context.Context, stateDir string, args ...string) (string, error) {
	binary, err := os.Executable()
	if err != nil {
		return "", err
	}
	command := exec.CommandContext(ctx, binary, append([]string{"job"}, args...)...)
	command.Env = append(os.Environ(), "GATOR_STATE_DIR="+stateDir)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return string(output), nil
}
func recoveryControl(ctx context.Context, stateDir, action string, args []string) (worktui.ControlScreen, error) {
	jobID := ""
	if len(args) > 0 {
		jobID = args[0]
	}
	var notice string
	switch action {
	case "start":
		if err := startLocalSupervisor(stateDir); err != nil {
			return worktui.ControlScreen{}, err
		}
		notice = "Supervisor started. It runs while this computer is on."
	case "install", "remove":
		service := "systemd"
		if runtime.GOOS == "darwin" {
			service = "launchagent"
		}
		if action == "install" {
			_, _ = managedJobCommand(ctx, stateDir, "stop")
		}
		serviceArgs := []string{service, action}
		if action == "remove" {
			serviceArgs = append(serviceArgs, "--yes")
		}
		output, err := managedJobCommand(ctx, stateDir, serviceArgs...)
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		notice = strings.TrimSpace(output)
	case "retry":
		if jobID == "" {
			return worktui.ControlScreen{}, errors.New("choose a check to retry")
		}
		output, err := managedJobCommand(ctx, stateDir, "run", jobID)
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		notice = strings.TrimSpace(output)
	case "token":
		if len(args) != 3 {
			return worktui.ControlScreen{}, errors.New("choose the check, connector, and new credential")
		}
		descriptor, credentials, err := configuredConnector(args[1])
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		store, err := jobs.Open(stateDir)
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		definition, err := store.Load(jobID)
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		selected := false
		for _, id := range definition.ConnectorIDs {
			if id == descriptor.ID {
				selected = true
			}
		}
		if !selected || descriptor.Authentication != connector.AuthBearer {
			return worktui.ControlScreen{}, errors.New("this check does not select that bearer connector")
		}
		credential, err := auth.NewBearerToken(args[2], time.Time{})
		if err != nil {
			return worktui.ControlScreen{}, err
		}
		if err = credentials.Put(descriptor.CredentialRef(), credential); err != nil {
			return worktui.ControlScreen{}, err
		}
		notice = "Credential replaced for " + descriptor.Name + ". Retry the check to verify access."
	}
	store, err := jobs.Open(stateDir)
	if err != nil {
		return worktui.ControlScreen{}, err
	}
	checks, running, err := readFollowupChecks(stateDir)
	if err != nil {
		return worktui.ControlScreen{}, err
	}
	screen := worktui.ControlScreen{Panel: uisnapshot.Panel{Kind: "recovery", Title: "Recovery and background checks", Notice: notice}}
	servicePath, err := recoveryServicePath()
	if err != nil {
		return screen, err
	}
	_, serviceErr := os.Stat(servicePath)
	installed := serviceErr == nil
	serviceState := "Not configured"
	if installed {
		serviceState = "Configuration installed; supervisor health is checked separately"
	}
	active := "Offline"
	if running {
		active = "Supervisor reachable"
	}
	lines := []string{active, "Start at login: " + serviceState, "Only this computer performs checks; it must be on and your user session must be active.", "Start-at-login uses " + servicePath}
	a, err := store.Availability()
	if err != nil {
		return screen, err
	}
	if !a.LastSeen.IsZero() {
		lines = append(lines, "Last observed supervisor heartbeat: "+a.LastSeen.Local().Format(time.RFC1123))
	}
	if !running && !a.LastSeen.IsZero() {
		lines = append(lines, "Monitoring is not confirmed after that heartbeat. Start the supervisor, then check current sources.")
	}
	screen.Panel.Blocks = []uisnapshot.Block{{Title: "Background availability", Lines: lines}}
	gapLines := []string{"An interval without heartbeats does not reveal which source changes happened. A catch-up check reads current sources; intermediate changes may be unavailable."}
	for i := len(a.Gaps) - 1; i >= 0 && i >= len(a.Gaps)-5; i-- {
		gapLines = append(gapLines, a.Gaps[i].From.Local().Format(time.RFC1123)+" → "+a.Gaps[i].To.Local().Format(time.RFC1123)+" · no continuous monitoring confirmed")
	}
	if len(a.Gaps) == 0 {
		gapLines = append(gapLines, "No recorded coverage gaps yet; this does not establish earlier coverage.")
	}
	screen.Panel.Blocks = append(screen.Panel.Blocks, uisnapshot.Block{Title: "What happened while offline", Lines: gapLines})
	for _, check := range checks {
		if jobID != "" && check.ID != jobID {
			continue
		}
		screen.Panel.Blocks = append(screen.Panel.Blocks, uisnapshot.Block{Title: check.Title, Lines: []string{check.HealthLine(), check.Issue, "Next action: " + check.NextAction}})
		if jobID == "" {
			if len(screen.Actions) < 9 {
				screen.Actions = append(screen.Actions, worktui.ControlAction{Key: strconv.Itoa(len(screen.Actions) + 1), Label: "Recover " + check.Title, Action: "select", Args: []string{check.ID}})
			}
			continue
		}
		history, err := store.History(jobID, 5)
		if err != nil {
			return screen, err
		}
		var attempts []string
		for _, attempt := range history {
			attempts = append(attempts, attempt.StartedAt.Local().Format(time.RFC1123)+" · "+attempt.Status+" · "+attempt.Summary)
		}
		if len(attempts) == 0 {
			attempts = []string{"No retained attempt yet."}
		}
		screen.Panel.Blocks = append(screen.Panel.Blocks, uisnapshot.Block{Title: "Recent checks and catch-up", Lines: attempts})
		if check.Enabled && check.State != "goal paused" {
			screen.Actions = append(screen.Actions, worktui.ControlAction{Key: "X", Label: "Retry this check now", Action: "retry", Args: []string{jobID}})
		}
		for index, id := range check.Definition.ConnectorIDs {
			if index >= 8 {
				break
			}
			descriptor, credentials, err := configuredConnector(id)
			if err != nil {
				return screen, err
			}
			status, err := connectorAuthStatus(descriptor, credentials)
			if err != nil {
				return screen, err
			}
			screen.Panel.Blocks = append(screen.Panel.Blocks, uisnapshot.Block{Title: descriptor.Name, Lines: []string{status, "Resource: " + descriptor.Resource, "Existing OAuth scopes: " + descriptor.OAuthScopes, "Reconnect preserves this connector configuration and the check's permissions. An account change needs a reviewed agreement edit through /followup."}})
			control := worktui.ControlAction{Key: strconv.Itoa(index + 1), Label: "Reconnect " + descriptor.Name, Action: "token", Args: []string{jobID, id}, Description: "Replace credentials only for " + descriptor.ID + " at " + descriptor.Resource + ". Existing OAuth scopes: " + descriptor.OAuthScopes + ". Reconnecting does not authorize new operations.", Review: true}
			if descriptor.Authentication == connector.AuthOAuth {
				control.ConnectorID = id
			} else if descriptor.Authentication == connector.AuthBearer {
				control.Fields = []worktui.ControlField{{Label: "New bearer token", Secret: true}}
			} else {
				continue
			}
			screen.Actions = append(screen.Actions, control)
		}
	}
	screen.Actions = append(screen.Actions, worktui.ControlAction{Key: "S", Label: "Start supervisor now", Action: "start", Args: []string{jobID}})
	autostart := worktui.ControlAction{Key: "A", Label: "Enable start at login", Action: "install", Args: []string{jobID}, Review: true, Description: "Install Gator's per-user background service at " + servicePath + ". It uses this state directory and the current Gator executable. This may replace an existing Gator service configuration. Checks run only while this host and user session are active."}
	if installed {
		autostart.Label = "Disable start at login"
		autostart.Action = "remove"
		autostart.Description = "Remove Gator's per-user service at " + servicePath + " and stop its supervisor. Saved checks remain and can be started manually."
	}
	screen.Actions = append(screen.Actions, autostart, worktui.ControlAction{Key: "R", Label: "Refresh", Action: "status", Args: []string{jobID}})
	if notice != "" {
		screen.Panel.Blocks = append([]uisnapshot.Block{{Title: "Action result", Lines: strings.Split(notice, "\n")}}, screen.Panel.Blocks...)
	}
	return screen, nil
}
