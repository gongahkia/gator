package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	gatorbrowser "github.com/gongahkia/gator/internal/browser"
	"github.com/gongahkia/gator/internal/journal"
)

const browserUsage = `usage:
  gator browser install
  gator browser status
  gator browser start [--headed] [--visual-capture]
  gator browser attach --cdp http://127.0.0.1:PORT [--visual-capture]
  gator browser tabs SESSION_ID
  gator browser select SESSION_ID TAB_ID [TAB_ID...]
  gator browser origins SESSION_ID list|add URL|remove URL
  gator browser visual SESSION_ID on|off
  gator browser allow-upload SESSION_ID ABSOLUTE_PATH
  gator browser artifacts SESSION_ID
  gator browser export SESSION_ID ARTIFACT_ID --out ABSOLUTE_PATH
  gator browser stop SESSION_ID`

func browserCommand(arguments []string, out io.Writer) error {
	if len(arguments) == 0 {
		return errors.New(browserUsage)
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	store, err := gatorbrowser.Open(stateDir)
	if err != nil {
		return err
	}
	switch arguments[0] {
	case "install":
		if len(arguments) != 1 {
			return errors.New("usage: gator browser install")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		status, err := gatorbrowser.InstallRuntime(ctx, store)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Installed pinned Playwright/Chromium runtime\n  directory: %s\n  node: %s\n", status.Directory, status.Node)
		return err
	case "status":
		if len(arguments) != 1 {
			return errors.New("usage: gator browser status")
		}
		return writeBrowserStatus(out, store)
	case "start":
		return startBrowserSession(arguments[1:], out, store, stateDir, gatorbrowser.ModeManaged)
	case "attach":
		return attachBrowserSession(arguments[1:], out, store, stateDir)
	case "tabs":
		if len(arguments) != 2 {
			return errors.New("usage: gator browser tabs SESSION_ID")
		}
		client, err := gatorbrowser.NewClient(store, arguments[1])
		if err != nil {
			return err
		}
		tabs, err := client.CandidateTabs(context.Background())
		if err != nil {
			return err
		}
		return writeBrowserTabs(out, tabs)
	case "select":
		return selectBrowserTabs(arguments[1:], out, store)
	case "origins":
		return browserOrigins(arguments[1:], out, store)
	case "visual":
		return browserVisual(arguments[1:], out, store)
	case "allow-upload":
		return browserAllowUpload(arguments[1:], out, store)
	case "artifacts":
		return browserArtifacts(arguments[1:], out, store)
	case "export":
		return browserExportArtifact(arguments[1:], out, store)
	case "stop":
		if len(arguments) != 2 {
			return errors.New("usage: gator browser stop SESSION_ID")
		}
		client, err := gatorbrowser.NewClient(store, arguments[1])
		if err != nil {
			return err
		}
		session, err := client.Stop(context.Background())
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Stopped browser session %s\n", session.ID)
		return err
	case "daemon":
		return browserDaemon(arguments[1:], store, stateDir)
	default:
		return errors.New(browserUsage)
	}
}

func startBrowserSession(arguments []string, out io.Writer, store *gatorbrowser.Store, stateDir string, mode gatorbrowser.Mode) error {
	flags := flag.NewFlagSet("browser start", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	headed := flags.Bool("headed", true, "open a headed browser window for developer takeover")
	visual := flags.Bool("visual-capture", false, "allow screenshots to be sent to the model for this session")
	if err := flags.Parse(arguments); err != nil || len(flags.Args()) != 0 {
		return errors.New("usage: gator browser start [--headed] [--visual-capture]")
	}
	status := gatorbrowser.Runtime(store)
	if !status.Installed {
		return errors.New(status.Error)
	}
	session, err := store.Start(gatorbrowser.StartOptions{Headed: *headed, VisualCapture: *visual})
	if err != nil {
		return err
	}
	if err := launchBrowserDaemon(store, stateDir, session, "", out); err != nil {
		_, _ = store.Stop(session.ID)
		return err
	}
	client, err := gatorbrowser.NewClient(store, session.ID)
	if err != nil {
		return err
	}
	// The sole tab in a fresh managed profile is created by this explicit start
	// command, so selecting it cannot disclose an existing user browser tab.
	candidates, err := client.CandidateTabs(context.Background())
	if err != nil {
		return err
	}
	if len(candidates) != 1 {
		return errors.New("managed browser session did not create exactly one initial tab")
	}
	if _, err := client.SelectTabs(context.Background(), candidates); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Started local browser session %s\n  mode: managed\n  selected tab: %s\n  visual capture: %t\nUse 'gator browser origins %s add URL' before a run may navigate.\n", session.ID, candidates[0].ID, *visual, session.ID)
	return err
}

func attachBrowserSession(arguments []string, out io.Writer, store *gatorbrowser.Store, stateDir string) error {
	flags := flag.NewFlagSet("browser attach", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	endpoint := flags.String("cdp", "", "literal loopback Chromium CDP endpoint")
	visual := flags.Bool("visual-capture", false, "allow screenshots to be sent to the model for this session")
	if err := flags.Parse(arguments); err != nil || len(flags.Args()) != 0 || strings.TrimSpace(*endpoint) == "" {
		return errors.New("usage: gator browser attach --cdp http://127.0.0.1:PORT [--visual-capture]")
	}
	if err := gatorbrowser.ValidateCDPEndpoint(*endpoint); err != nil {
		return err
	}
	status := gatorbrowser.Runtime(store)
	if !status.Installed {
		return errors.New(status.Error)
	}
	session, err := store.Attach(gatorbrowser.AttachOptions{CDPEndpoint: *endpoint, VisualCapture: *visual})
	if err != nil {
		return err
	}
	if err := launchBrowserDaemon(store, stateDir, session, *endpoint, out); err != nil {
		_, _ = store.Stop(session.ID)
		return err
	}
	client, err := gatorbrowser.NewClient(store, session.ID)
	if err != nil {
		return err
	}
	tabs, err := client.CandidateTabs(context.Background())
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Attached local browser session %s. No tab is shared with a run yet.\n", session.ID); err != nil {
		return err
	}
	if err := writeBrowserTabs(out, tabs); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Select one or more tabs with: gator browser select %s TAB_ID [TAB_ID...]\n", session.ID)
	return err
}

func launchBrowserDaemon(store *gatorbrowser.Store, stateDir string, session gatorbrowser.Session, endpoint string, out io.Writer) error {
	tokenFile, _, err := gatorbrowser.CreateToken(store, session.ID)
	if err != nil {
		return err
	}
	arguments := []string{"browser", "daemon", "--state-dir", stateDir, "--session", session.ID, "--mode", string(session.Mode), "--token-file", tokenFile}
	if session.Headed {
		arguments = append(arguments, "--headed")
	}
	if endpoint != "" {
		arguments = append(arguments, "--cdp", endpoint)
	}
	logDirectory := filepath.Join(store.Directory(), "logs")
	if err := os.MkdirAll(logDirectory, 0o700); err != nil {
		return fmt.Errorf("create browser daemon log directory: %w", err)
	}
	logFile, err := os.OpenFile(filepath.Join(logDirectory, session.ID+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open browser daemon log: %w", err)
	}
	command := exec.Command(os.Args[0], arguments...)
	command.Stdout = logFile
	command.Stderr = logFile
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start browser daemon: %w", err)
	}
	_ = logFile.Close()
	if err := waitBrowserReady(store, session.ID, command.Process); err != nil {
		_ = command.Process.Kill()
		return err
	}
	_, err = fmt.Fprintf(out, "Browser daemon started (pid %d).\n", command.Process.Pid)
	return err
}

func waitBrowserReady(store *gatorbrowser.Store, sessionID string, process *os.Process) error {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		client, err := gatorbrowser.NewClient(store, sessionID)
		if err == nil {
			if _, err := client.CandidateTabs(context.Background()); err == nil {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if process != nil {
		_ = process.Kill()
	}
	return errors.New("browser daemon did not become ready; inspect 'gator browser status' and its private local log")
}

func selectBrowserTabs(arguments []string, out io.Writer, store *gatorbrowser.Store) error {
	if len(arguments) < 2 {
		return errors.New("usage: gator browser select SESSION_ID TAB_ID [TAB_ID...]")
	}
	client, err := gatorbrowser.NewClient(store, arguments[0])
	if err != nil {
		return err
	}
	candidates, err := client.CandidateTabs(context.Background())
	if err != nil {
		return err
	}
	byID := make(map[string]gatorbrowser.Tab, len(candidates))
	for _, tab := range candidates {
		byID[tab.ID] = tab
	}
	selected := make([]gatorbrowser.Tab, 0, len(arguments)-1)
	for _, id := range arguments[1:] {
		tab, found := byID[id]
		if !found {
			return fmt.Errorf("browser tab %q is unavailable; run 'gator browser tabs %s'", id, arguments[0])
		}
		selected = append(selected, tab)
	}
	session, err := client.SelectTabs(context.Background(), selected)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Selected %d tab(s) for browser session %s.\n", len(session.SelectedTabs), session.ID)
	return err
}

func browserOrigins(arguments []string, out io.Writer, store *gatorbrowser.Store) error {
	if len(arguments) < 2 {
		return errors.New("usage: gator browser origins SESSION_ID list|add URL|remove URL")
	}
	client, err := gatorbrowser.NewClient(store, arguments[0])
	if err != nil {
		return err
	}
	switch arguments[1] {
	case "list":
		if len(arguments) != 2 {
			return errors.New("usage: gator browser origins SESSION_ID list")
		}
		session, err := client.Session(context.Background(), arguments[0])
		if err != nil {
			return err
		}
		for _, origin := range session.Origins {
			if _, err := fmt.Fprintln(out, origin.URL); err != nil {
				return err
			}
		}
		return nil
	case "add", "remove":
		if len(arguments) != 3 {
			return fmt.Errorf("usage: gator browser origins SESSION_ID %s URL", arguments[1])
		}
		var session gatorbrowser.Session
		if arguments[1] == "add" {
			session, err = client.AddOrigin(context.Background(), arguments[2])
		} else {
			session, err = client.RemoveOrigin(context.Background(), arguments[2])
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Browser session %s now has %d approved origin(s).\n", session.ID, len(session.Origins))
		return err
	default:
		return errors.New("usage: gator browser origins SESSION_ID list|add URL|remove URL")
	}
}

func browserVisual(arguments []string, out io.Writer, store *gatorbrowser.Store) error {
	if len(arguments) != 2 || (arguments[1] != "on" && arguments[1] != "off") {
		return errors.New("usage: gator browser visual SESSION_ID on|off")
	}
	client, err := gatorbrowser.NewClient(store, arguments[0])
	if err != nil {
		return err
	}
	session, err := client.SetVisualCapture(context.Background(), arguments[1] == "on")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Model-visible screenshots for %s: %t\n", session.ID, session.VisualCapture)
	return err
}

func browserAllowUpload(arguments []string, out io.Writer, store *gatorbrowser.Store) error {
	if len(arguments) != 2 {
		return errors.New("usage: gator browser allow-upload SESSION_ID ABSOLUTE_PATH")
	}
	client, err := gatorbrowser.NewClient(store, arguments[0])
	if err != nil {
		return err
	}
	_, upload, err := client.AllowUpload(context.Background(), arguments[1])
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Registered developer-selected upload %s (%s).\n", upload.ID, upload.Name)
	return err
}

func browserArtifacts(arguments []string, out io.Writer, store *gatorbrowser.Store) error {
	if len(arguments) != 1 {
		return errors.New("usage: gator browser artifacts SESSION_ID")
	}
	client, err := gatorbrowser.NewClient(store, arguments[0])
	if err != nil {
		return err
	}
	artifacts, err := client.Artifacts()
	if err != nil {
		return err
	}
	for _, artifact := range artifacts {
		if _, err := fmt.Fprintf(out, "%s\t%s\t%d\t%s\n", artifact.ID, artifact.Kind, artifact.Bytes, artifact.Name); err != nil {
			return err
		}
	}
	return nil
}

func browserExportArtifact(arguments []string, out io.Writer, store *gatorbrowser.Store) error {
	flags := flag.NewFlagSet("browser export", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	destination := flags.String("out", "", "new absolute output path")
	if err := flags.Parse(arguments); err != nil || len(flags.Args()) != 2 || strings.TrimSpace(*destination) == "" {
		return errors.New("usage: gator browser export SESSION_ID ARTIFACT_ID --out ABSOLUTE_PATH")
	}
	client, err := gatorbrowser.NewClient(store, flags.Args()[0])
	if err != nil {
		return err
	}
	if err := client.ExportArtifact(flags.Args()[1], *destination); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Exported browser artifact to %s\n", *destination)
	return err
}

func browserDaemon(arguments []string, store *gatorbrowser.Store, stateDir string) error {
	flags := flag.NewFlagSet("browser daemon", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	providedStateDir := flags.String("state-dir", stateDir, "internal browser state directory")
	sessionID := flags.String("session", "", "internal browser session id")
	mode := flags.String("mode", "", "internal browser mode")
	tokenFile := flags.String("token-file", "", "internal private session token")
	headed := flags.Bool("headed", false, "internal headed browser mode")
	endpoint := flags.String("cdp", "", "internal attached browser endpoint")
	if err := flags.Parse(arguments); err != nil || len(flags.Args()) != 0 {
		return errors.New("browser daemon arguments are invalid")
	}
	if *providedStateDir != stateDir || strings.TrimSpace(*sessionID) == "" || strings.TrimSpace(*tokenFile) == "" {
		return errors.New("browser daemon arguments are invalid")
	}
	if *mode != string(gatorbrowser.ModeManaged) && *mode != string(gatorbrowser.ModeAttached) {
		return errors.New("browser daemon mode is invalid")
	}
	if filepath.Clean(*tokenFile) != filepath.Clean(gatorbrowser.TokenPath(store, *sessionID)) {
		return errors.New("browser daemon token path is invalid")
	}
	token, err := gatorbrowser.ReadToken(store, *sessionID)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), serveSignals()...)
	defer stop()
	driver, err := gatorbrowser.StartSidecar(ctx, gatorbrowser.SidecarConfig{Store: store, Mode: gatorbrowser.Mode(*mode), Headed: *headed, CDPEndpoint: *endpoint})
	if err != nil {
		return err
	}
	service, err := gatorbrowser.NewService(gatorbrowser.ServiceConfig{Store: store, SessionID: *sessionID, Token: token, Driver: driver, Socket: gatorbrowser.SocketPath(store, *sessionID)})
	if err != nil {
		_ = driver.Close()
		return err
	}
	return service.Run(ctx)
}

func writeBrowserStatus(out io.Writer, store *gatorbrowser.Store) error {
	runtime := gatorbrowser.Runtime(store)
	state := "not installed"
	if runtime.Installed {
		state = "installed"
	}
	if _, err := fmt.Fprintf(out, "Browser runtime: %s\n", state); err != nil {
		return err
	}
	if runtime.Error != "" {
		if _, err := fmt.Fprintf(out, "Browser runtime detail: %s\n", runtime.Error); err != nil {
			return err
		}
	}
	sessions, err := store.List()
	if err != nil {
		return err
	}
	for _, session := range sessions {
		active := "stopped"
		if session.State == gatorbrowser.StateRunning {
			if _, err := os.Stat(gatorbrowser.SocketPath(store, session.ID)); err == nil {
				active = "running"
			} else {
				active = "unavailable"
			}
		}
		if _, err := fmt.Fprintf(out, "%s\t%s\t%s\ttabs=%d\torigins=%d\tvisual=%t\n", session.ID, session.Mode, active, len(session.SelectedTabs), len(session.Origins), session.VisualCapture); err != nil {
			return err
		}
	}
	return nil
}

func writeBrowserTabs(out io.Writer, tabs []gatorbrowser.Tab) error {
	sort.Slice(tabs, func(left, right int) bool { return tabs[left].ID < tabs[right].ID })
	for _, tab := range tabs {
		if _, err := fmt.Fprintf(out, "%s\t%s\t%s\n", tab.ID, tab.Title, tab.URL); err != nil {
			return err
		}
	}
	return nil
}
