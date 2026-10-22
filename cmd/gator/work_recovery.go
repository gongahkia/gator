package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/hostproc"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workapp"
)

func recoveryServicePath() (string, error) {
	if runtime.GOOS == "windows" {
		return windowsRunPath, nil
	}
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
	command := hostproc.CommandContext(ctx, binary, append([]string{"job"}, args...)...)
	command.Env = append(os.Environ(), "GATOR_STATE_DIR="+stateDir)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return string(output), nil
}
func recoveryControl(ctx context.Context, stateDir, action string, args []string) (workapp.ControlScreen, error) {
	return recoveryControlWith(ctx, stateDir, action, args, managedJobCommand)
}

func recoveryControlWith(ctx context.Context, stateDir, action string, args []string, command func(context.Context, string, ...string) (string, error)) (workapp.ControlScreen, error) {
	servicePath, err := recoveryServicePath()
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	installed, ownsService, serviceDigest, err := recoveryServiceReview(servicePath, stateDir, runtime.GOOS)
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	jobID := ""
	if len(args) > 0 {
		jobID = args[0]
	}
	var notice string
	switch action {
	case "start":
		if err := startLocalSupervisor(stateDir); err != nil {
			return workapp.ControlScreen{}, err
		}
		notice = "Supervisor started. It runs while this computer is on."
	case "install", "remove":
		if len(args) != 2 || args[1] != serviceDigest {
			return workapp.ControlScreen{}, errors.New("background service changed during review; refresh recovery before changing it")
		}
		if installed && !ownsService {
			return workapp.ControlScreen{}, errors.New("the installed Gator service belongs to another state directory; manage it from that profile before enabling this one")
		}
		service := "systemd"
		if runtime.GOOS == "darwin" {
			service = "launchagent"
		}
		if runtime.GOOS == "windows" {
			service = "windows"
		}
		if action == "install" {
			_, _ = command(ctx, stateDir, "stop")
		}
		serviceArgs := []string{service, action}
		if runtime.GOOS == "windows" {
			serviceArgs = append(serviceArgs, "--expected", serviceDigest)
		} else if action == "remove" {
			serviceArgs = append(serviceArgs, "--yes")
		}
		output, err := command(ctx, stateDir, serviceArgs...)
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		notice = strings.TrimSpace(output)
	case "retry":
		if jobID == "" {
			return workapp.ControlScreen{}, errors.New("choose a check to retry")
		}
		output, err := command(ctx, stateDir, "run", jobID)
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		notice = strings.TrimSpace(output)
	case "token":
		if len(args) != 4 {
			return workapp.ControlScreen{}, errors.New("choose the check, connector, and new credential")
		}
		descriptor, _, err := configuredConnector(args[1])
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		if descriptor.CredentialRef() != args[2] {
			return workapp.ControlScreen{}, errors.New("connector resource changed during review; reconnect from a fresh recovery screen")
		}
		credentials, err := auth.New(stateDir)
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		store, err := jobs.Open(stateDir)
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		definition, err := store.Load(jobID)
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		selected := false
		for _, id := range definition.ConnectorIDs {
			if id == descriptor.ID {
				selected = true
			}
		}
		if !selected || descriptor.Authentication != connector.AuthBearer {
			return workapp.ControlScreen{}, errors.New("this check does not select that bearer connector")
		}
		credential, err := auth.NewBearerToken(args[3], time.Time{})
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		if err = credentials.Put(descriptor.CredentialRef(), credential); err != nil {
			return workapp.ControlScreen{}, err
		}
		notice = "Credential replaced for " + descriptor.Name + ". Retry the check to verify access."
	}
	store, err := jobs.Open(stateDir)
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	checks, running, err := readFollowupChecks(stateDir)
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	screen := workapp.ControlScreen{Panel: uisnapshot.Panel{Kind: "recovery", Title: "Recovery", Notice: notice}}
	serviceState := "Not configured"
	if installed {
		serviceState = "Configuration installed; supervisor health is checked separately"
		if !ownsService {
			serviceState = "Installed for another state directory; manage that profile before enabling this one"
		}
	}
	active := "Offline"
	if running {
		active = "Supervisor reachable"
	}
	lines := []string{active, "Start at login: " + serviceState, "Only this computer performs checks; it must be on and your user session must be active.", "Start-at-login uses " + servicePath}
	loginLabel, healthTone := "Not configured", "muted"
	if installed {
		loginLabel = "Configured"
		if !ownsService {
			loginLabel = "Another profile"
		}
	}
	if running {
		healthTone = "success"
	} else {
		for _, check := range checks {
			if check.Enabled {
				healthTone = "warning"
			}
		}
	}
	screen.Panel.Summary = []uisnapshot.Field{{Label: "Background checks", Value: active, Tone: healthTone}, {Label: "Start at login", Value: loginLabel}}
	screen.Panel.Subtitle = "Checks run on this computer while it is on and your user session is active."
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
	screen.Panel.Blocks = []uisnapshot.Block{{Title: "Background availability", Collapsible: true, Lines: lines}}
	gapLines := []string{"An interval without heartbeats does not reveal which source changes happened. A catch-up check reads current sources; intermediate changes may be unavailable."}
	for i := len(a.Gaps) - 1; i >= 0 && i >= len(a.Gaps)-5; i-- {
		gapLines = append(gapLines, a.Gaps[i].From.Local().Format(time.RFC1123)+" → "+a.Gaps[i].To.Local().Format(time.RFC1123)+" · no continuous monitoring confirmed")
	}
	if len(a.Gaps) == 0 {
		gapLines = append(gapLines, "No recorded coverage gaps yet; this does not establish earlier coverage.")
	}
	screen.Panel.Blocks = append(screen.Panel.Blocks, uisnapshot.Block{Title: "Monitoring coverage", Lines: []string{gapLines[0]}})
	screen.Panel.Blocks = append(screen.Panel.Blocks, uisnapshot.Block{Title: "What happened while offline", Collapsible: true, Lines: gapLines[1:]})
	for _, check := range checks {
		if jobID != "" && check.ID != jobID {
			continue
		}
		screen.Panel.Blocks = append(screen.Panel.Blocks, uisnapshot.Block{Title: check.Title, Collapsible: jobID == "", Lines: []string{check.HealthLine(), check.Issue, "Next action: " + check.NextAction}})
		if jobID == "" {
			if len(screen.Actions) < 9 {
				screen.Actions = append(screen.Actions, workapp.ControlAction{Key: strconv.Itoa(len(screen.Actions) + 1), Label: "Recover " + check.Title, Action: "select", Args: []string{check.ID}})
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
		screen.Panel.Blocks = append(screen.Panel.Blocks, uisnapshot.Block{Title: "Recent checks and catch-up", Collapsible: true, Lines: attempts})
		if check.Enabled && check.State != "goal paused" {
			screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "X", Label: "Retry this check now", Action: "retry", Args: []string{jobID}})
		}
		for index, id := range check.Definition.ConnectorIDs {
			if index >= 8 {
				break
			}
			descriptor, _, err := configuredConnector(id)
			if err != nil {
				screen.Panel.Blocks = append(screen.Panel.Blocks, uisnapshot.Block{Title: id, Lines: []string{"Connector configuration is unavailable: " + err.Error(), "Review /connector and edit the selected sources through /followup before retrying."}})
				continue
			}
			credentials, err := auth.New(stateDir)
			if err != nil {
				return screen, err
			}
			status, err := connectorAuthStatus(descriptor, credentials)
			if err != nil {
				return screen, err
			}
			screen.Panel.Blocks = append(screen.Panel.Blocks, uisnapshot.Block{Title: descriptor.Name, Lines: []string{status, "Resource: " + descriptor.Resource, "Existing OAuth scopes: " + descriptor.OAuthScopes, "Reconnect preserves this connector configuration and the check's permissions. An account change needs a reviewed agreement edit through /followup."}})
			control := workapp.ControlAction{Key: strconv.Itoa(index + 1), Label: "Reconnect " + descriptor.Name, Action: "token", Args: []string{jobID, id, descriptor.CredentialRef()}, Description: "Replace credentials only for " + descriptor.ID + " at " + descriptor.Resource + ". Existing OAuth scopes: " + descriptor.OAuthScopes + ". Reconnecting does not authorize new operations.", Review: true}
			if descriptor.Authentication == connector.AuthOAuth {
				control.ConnectorID = id
				control.ConnectorReview = connectorLoginReview(descriptor)
			} else if descriptor.Authentication == connector.AuthBearer {
				control.Fields = []workapp.ControlField{{Label: "New bearer token", Secret: true}}
			} else {
				continue
			}
			screen.Actions = append(screen.Actions, control)
		}
	}
	screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "S", Label: "Start supervisor now", Action: "start", Args: []string{jobID}})
	autostart := workapp.ControlAction{Key: "A", Label: "Enable start at login", Action: "install", Args: []string{jobID, serviceDigest}, Review: true, Description: "Install Gator's per-user background service at " + servicePath + ". It uses this state directory and the current Gator executable. This may replace an existing Gator service configuration. Checks run only while this host and user session are active."}
	if installed {
		autostart.Label = "Disable start at login"
		autostart.Action = "remove"
		autostart.Description = "Remove Gator's per-user service at " + servicePath + " and stop its supervisor. Saved checks remain and can be started manually."
	}
	if !installed || ownsService {
		screen.Actions = append(screen.Actions, autostart)
	}
	screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "R", Label: "Refresh", Action: "status", Args: []string{jobID}})
	if notice != "" {
		screen.Panel.Blocks = append([]uisnapshot.Block{{Title: "Action result", Lines: strings.Split(notice, "\n")}}, screen.Panel.Blocks...)
	}
	return screen, nil
}

// recoveryServiceReview recognizes Gator's generated service configuration.
// Unknown or ambiguous configurations are never offered for removal.
func recoveryServiceReview(filename, stateDir, platform string) (installed, owns bool, digest string, err error) {
	if platform == "windows" {
		return reviewWindowsLogin(stateDir)
	}
	data, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, "absent", nil
	}
	if err != nil {
		return false, false, "", err
	}
	sum := sha256.Sum256(data)
	digest = hex.EncodeToString(sum[:])
	selected := ""
	count := 0
	if platform == "darwin" {
		decoder := xml.NewDecoder(strings.NewReader(string(data)))
		wantValue := false
		for {
			token, readErr := decoder.Token()
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				return true, false, digest, nil
			}
			start, ok := token.(xml.StartElement)
			if !ok {
				continue
			}
			if start.Name.Local == "key" || start.Name.Local == "string" {
				var value string
				if err := decoder.DecodeElement(&value, &start); err != nil {
					return true, false, digest, nil
				}
				if start.Name.Local == "key" {
					wantValue = value == "GATOR_STATE_DIR"
				} else if wantValue {
					selected = value
					count++
					wantValue = false
				}
			}
		}
	} else {
		for _, line := range strings.Split(string(data), "\n") {
			value, ok := strings.CutPrefix(strings.TrimSpace(line), "Environment=")
			if !ok {
				continue
			}
			value, unquoteErr := strconv.Unquote(value)
			if unquoteErr != nil {
				continue
			}
			value = strings.ReplaceAll(value, "%%", "%")
			if root, ok := strings.CutPrefix(value, "GATOR_STATE_DIR="); ok {
				selected = root
				count++
			}
		}
	}
	canonical := func(value string) string {
		absolute, err := filepath.Abs(value)
		if err != nil {
			return ""
		}
		if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
			return resolved
		}
		return filepath.Clean(absolute)
	}
	return true, count == 1 && selected != "" && canonical(selected) == canonical(stateDir), digest, nil
}
