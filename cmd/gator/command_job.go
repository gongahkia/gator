package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workrun"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/journal"
)

type jobResult struct {
	ConversationID string `json:"conversation_id"`
	RevisionID     string `json:"revision_id"`
	ManifestPath   string `json:"manifest_path"`
	Status         string `json:"status"`
	Error          string `json:"error"`
}

type supervisorState struct {
	Version   int       `json:"version"`
	PID       int       `json:"pid"`
	URL       string    `json:"url"`
	Nonce     string    `json:"nonce"`
	StartedAt time.Time `json:"started_at"`
}

type supervisorHealth struct {
	OK    bool   `json:"ok"`
	PID   int    `json:"pid"`
	Nonce string `json:"nonce"`
}

func jobCommand(arguments []string, out io.Writer) error {
	if len(arguments) == 0 || arguments[0] == "list" {
		return listJobs(out)
	}
	switch arguments[0] {
	case "add":
		return addJob(arguments[1:], out)
	case "show", "enable", "disable", "run", "history", "remove", "edit":
		if len(arguments) < 2 {
			return fmt.Errorf("job %s requires a job ID", arguments[0])
		}
		return manageJob(arguments[0], arguments[1], arguments[2:], out)
	case "supervisor":
		return runJobSupervisor(arguments[1:], out)
	case "status":
		return jobSupervisorRequest("health", out)
	case "stop":
		return jobSupervisorRequest("stop", out)
	default:
		return fmt.Errorf("unknown job command %q", arguments[0])
	}
}

func jobStore() (jobs.Store, string, error) {
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return jobs.Store{}, "", err
	}
	store, err := jobs.Open(stateDir)
	return store, stateDir, err
}

func listJobs(out io.Writer) error {
	store, _, err := jobStore()
	if err != nil {
		return err
	}
	definitions, err := store.List()
	if err != nil {
		return err
	}
	if len(definitions) == 0 {
		_, err = fmt.Fprintln(out, "No scheduled jobs.")
		return err
	}
	for _, definition := range definitions {
		state := "disabled"
		if definition.Enabled {
			state = "enabled"
		}
		fmt.Fprintf(out, "%s\t%s\t%s %s\t%s\n", definition.ID, state, definition.Schedule, definition.Timezone, definition.Name)
	}
	return nil
}

func addJob(arguments []string, out io.Writer) error {
	if len(arguments) == 0 {
		return errors.New("usage: gator job add NAME --schedule CRON [options] -- TASK")
	}
	name := arguments[0]
	settings, err := loadSettings()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("job add", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	schedule := flags.String("schedule", "", "five-field cron schedule")
	timezone := flags.String("timezone", settings.JobDefaults.Timezone, "IANA timezone")
	missed := flags.String("missed", settings.JobDefaults.Missed, "skip or run_once")
	source := flags.String("source", ".", "local source directory")
	refresh := flags.Bool("refresh-snapshot", true, "capture current source for each attempt; false freezes at creation")
	contractPath := flags.String("contract", "", "complete outcome contract JSON")
	codePath := flags.String("code-policy", "", "complete Code policy JSON")
	requests := flags.Int("max-model-requests", 256, "aggregate request bound")
	tokens := flags.Int64("max-tokens", 0, "reported token bound; zero disables")
	seconds := flags.Int("timeout-seconds", 1800, "run wall bound")
	var webOrigins connectorFlags
	flags.Var(&webOrigins, "web-origin", "permitted HTTPS origin")
	provider := flags.String("provider", "", "model provider")
	model := flags.String("model", "", "model")
	modeName := flags.String("mode", string(action.Draft), "inspect or draft")
	maxSteps := flags.Int("max-steps", 24, "maximum model turns")
	disposition := flags.String("actions", string(action.Forbid), "forbid or draft")
	var artifacts artifactFlags
	flags.Var(&artifacts, "artifact", "required artifact path")
	var connectors connectorFlags
	flags.Var(&connectors, "connector", "connector ID")
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	if *schedule == "" {
		return errors.New("job add requires --schedule")
	}
	objective := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if objective == "" {
		return errors.New("job add requires a task after its options")
	}
	mode := action.Mode(*modeName)
	contract, err := workContract(mode, action.Disposition(*disposition), artifacts, nil)
	if err != nil {
		return err
	}
	if *contractPath != "" {
		if err := readWorkJSON(*contractPath, &contract); err != nil {
			return err
		}
	}
	var codePolicy workrun.CodePolicy
	if *codePath != "" {
		if err := readWorkJSON(*codePath, &codePolicy); err != nil {
			return err
		}
	}
	absolute, err := filepath.Abs(*source)
	if err != nil {
		return err
	}
	if info, err := os.Stat(absolute); err != nil || !info.IsDir() {
		if err != nil {
			return fmt.Errorf("inspect job source: %w", err)
		}
		return errors.New("job source must be a directory")
	}
	store, _, err := jobStore()
	if err != nil {
		return err
	}
	definition, err := store.Save(jobs.Definition{Name: name, Enabled: true, Schedule: *schedule, Timezone: *timezone, Missed: *missed, SourcePath: absolute, RefreshSnapshot: *refresh, Code: codePolicy, Limits: agent.Limits{ModelRequests: *requests, Tokens: *tokens, WallSeconds: *seconds}, WebOrigins: webOrigins, Objective: objective, Provider: *provider, Model: *model, Mode: mode, Contract: contract, ConnectorIDs: connectors, MaxSteps: *maxSteps})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Added job %s (%s) on %s %s.\n", definition.ID, definition.Name, definition.Schedule, definition.Timezone)
	return err
}

func manageJob(command, id string, arguments []string, out io.Writer) error {
	store, stateDir, err := jobStore()
	if err != nil {
		return err
	}
	switch command {
	case "show":
		if len(arguments) != 0 {
			return errors.New("usage: gator job show ID")
		}
		definition, err := store.Load(id)
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(definition)
	case "enable", "disable":
		if len(arguments) != 0 {
			return fmt.Errorf("usage: gator job %s ID", command)
		}
		definition, err := store.SetEnabled(id, command == "enable")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "%s %s.\n", strings.Title(command), definition.ID)
		return err
	case "history":
		history, err := store.History(id, 50)
		if err != nil {
			return err
		}
		for _, attempt := range history {
			fmt.Fprintf(out, "%s\t%s\ttry=%d\t%s\t%s\n", attempt.ID, attempt.StartedAt.Format(time.RFC3339), attempt.Try, attempt.Status, attempt.Error)
		}
		return nil
	case "run":
		if len(arguments) != 0 {
			return errors.New("usage: gator job run ID")
		}
		definition, err := store.Load(id)
		if err != nil {
			return err
		}
		attempt, err := store.Begin(definition, time.Now().UTC())
		if err != nil {
			return err
		}
		attempt = executeJob(definition, attempt, stateDir, true)
		if err := store.Record(attempt); err != nil {
			return err
		}
		_, _ = recordJobInbox(stateDir, definition, attempt)
		encoder := json.NewEncoder(out)
		return encoder.Encode(attempt)
	case "remove":
		if len(arguments) != 1 || arguments[0] != "--yes" {
			return errors.New("job remove requires --yes")
		}
		if err := store.Remove(id); err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Removed job %s; retained history remains inspectable.\n", id)
		return err
	case "edit":
		definition, err := store.Load(id)
		if err != nil {
			return err
		}
		flags := flag.NewFlagSet("job edit", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		name := flags.String("name", "", "new name")
		schedule := flags.String("schedule", "", "new cron")
		timezone := flags.String("timezone", "", "new timezone")
		source := flags.String("source", "", "new source")
		objective := flags.String("task", "", "new task")
		missed := flags.String("missed", "", "new missed policy")
		if err := flags.Parse(arguments); err != nil {
			return err
		}
		if len(flags.Args()) != 0 {
			return errors.New("job edit received unexpected arguments")
		}
		if *name != "" {
			definition.Name = *name
		}
		if *schedule != "" {
			definition.Schedule = *schedule
		}
		if *timezone != "" {
			definition.Timezone = *timezone
		}
		if *source != "" {
			absolute, err := filepath.Abs(*source)
			if err != nil {
				return err
			}
			if info, err := os.Stat(absolute); err != nil || !info.IsDir() {
				if err != nil {
					return fmt.Errorf("inspect job source: %w", err)
				}
				return errors.New("job source must be a directory")
			}
			definition.SourcePath = absolute
		}
		if *objective != "" {
			definition.Objective = *objective
		}
		if *missed != "" {
			definition.Missed = *missed
		}
		definition, err = store.Save(definition)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Updated job %s.\n", definition.ID)
		return err
	}
	return errors.New("unsupported job command")
}

func executeJob(definition jobs.Definition, attempt jobs.Attempt, stateDir string, notify bool) jobs.Attempt {
	return executeJobContext(context.Background(), definition, attempt, stateDir, notify)
}
func executeJobContext(ctx context.Context, definition jobs.Definition, attempt jobs.Attempt, stateDir string, notify bool) jobs.Attempt {
	var result jobResult
	var runErr error
	store, runErr := jobs.Open(stateDir)
	for try := 1; runErr == nil && try <= 3; try++ {
		attempt.Try = try
		reference := jobs.RunReference{Try: try, RunID: attempt.ID + "-try-" + strconv.Itoa(try)}
		if err := store.RecordRun(attempt, reference); err != nil {
			runErr = err
			break
		}
		result, runErr = runJobProcessContext(ctx, definition, stateDir, reference.RunID)
		reference.Conversation, reference.Revision = result.ConversationID, result.RevisionID
		if result.ManifestPath != "" {
			reference.BundlePath = filepath.Dir(result.ManifestPath)
		}
		if runErr != nil {
			reference.Error = boundedJobError(runErr.Error())
		}
		attempt.Runs = append(attempt.Runs, reference)
		if err := store.RecordRun(attempt, reference); err != nil {
			runErr = err
			break
		}
		if runErr == nil {
			break
		}
		if !agent.IsTransient(runErr) {
			break
		}
		if try < 3 {
			select {
			case <-time.After(time.Duration(try) * time.Second):
				runErr = nil
			case <-ctx.Done():
				runErr = ctx.Err()
				try = 3
			}
		}
	}
	attempt.FinishedAt = time.Now().UTC()
	attempt.Conversation = result.ConversationID
	attempt.Revision = result.RevisionID
	if result.ManifestPath != "" {
		attempt.BundlePath = filepath.Dir(result.ManifestPath)
	}
	if runErr != nil {
		attempt.Status = "failed"
		attempt.Error = boundedJobError(runErr.Error())
	} else {
		attempt.Status = result.Status
		if attempt.Status == "" {
			attempt.Status = "completed"
		}
	}
	if notify {
		desktopNotification(definition.Name, attempt.Status)
	}
	return attempt
}

func runJobProcess(definition jobs.Definition, stateDir string) (jobResult, error) {
	return runJobProcessContext(context.Background(), definition, stateDir)
}
func jobWorkRequest(definition jobs.Definition) workrun.Request {
	request := workrun.Request{Limits: definition.Limits, WebOrigins: definition.WebOrigins, Project: definition.Project, SourcePath: definition.SourcePath, SnapshotID: definition.SnapshotID, Objective: definition.Objective, Mode: definition.Mode, Contract: definition.Contract, Code: definition.Code, MaxSteps: definition.MaxSteps, ConnectorIDs: definition.ConnectorIDs}
	if definition.RefreshSnapshot {
		request.SnapshotID = ""
	}
	return request
}

func runJobProcessContext(ctx context.Context, definition jobs.Definition, stateDir string, runIDs ...string) (jobResult, error) {
	request := jobWorkRequest(definition)
	if len(runIDs) > 0 {
		request.RunID = runIDs[0]
	}
	outcome, err := executeConfiguredWork(ctx, definition.Provider, definition.Model, stateDir, request)
	return jobResult{ConversationID: outcome.ConversationID, RevisionID: outcome.RevisionID, ManifestPath: outcome.Work.ManifestPath, Status: string(outcome.Manifest.Status)}, err
}

func runJobSupervisor(arguments []string, out io.Writer) error {
	settings, err := loadSettings()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("job supervisor", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	notify := flags.Bool("notify", settings.Notifications.Desktop, "emit desktop notifications when available")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("usage: gator job supervisor [--notify=true|false]")
	}
	store, stateDir, err := jobStore()
	if err != nil {
		return err
	}
	lockPath := filepath.Join(store.Root(), "supervisor.lock")
	if err := acquireSupervisorLock(lockPath); err != nil {
		return err
	}
	defer os.Remove(lockPath)
	interrupted, err := store.Reconcile()
	if err != nil {
		return err
	}
	for _, attempt := range interrupted {
		definition, err := store.Load(attempt.JobID)
		if err == nil {
			_, _ = recordJobInbox(stateDir, definition, attempt)
		}
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return err
	}
	token := hex.EncodeToString(tokenBytes)
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return err
	}
	nonce := hex.EncodeToString(nonceBytes)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	mux := http.NewServeMux()
	authorized := func(handler http.HandlerFunc) http.HandlerFunc {
		return func(writer http.ResponseWriter, request *http.Request) {
			if request.Header.Get("Authorization") != "Bearer "+token {
				http.Error(writer, "unauthorized", http.StatusUnauthorized)
				return
			}
			handler(writer, request)
		}
	}
	mux.HandleFunc("/health", authorized(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(writer, "method", http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(supervisorHealth{OK: true, PID: os.Getpid(), Nonce: nonce})
	}))
	mux.HandleFunc("/stop", authorized(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "method", http.StatusMethodNotAllowed)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
		cancel()
	}))
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	state := supervisorState{Version: 1, PID: os.Getpid(), URL: "http://" + listener.Addr().String(), Nonce: nonce, StartedAt: time.Now().UTC()}
	if err := writePrivateSupervisorFiles(store.Root(), state, token); err != nil {
		return err
	}
	defer os.Remove(filepath.Join(store.Root(), "supervisor.json"))
	defer os.Remove(filepath.Join(store.Root(), "supervisor.token"))
	fmt.Fprintf(out, "Gator job supervisor running at %s (PID %d). Keep this terminal open.\n", state.URL, state.PID)
	runDueJobsContext(ctx, store, stateDir, *notify, time.Now())
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			_ = server.Shutdown(shutdown)
			return nil
		case now := <-ticker.C:
			runDueJobsContext(ctx, store, stateDir, *notify, now)
		}
	}
}

func acquireSupervisorLock(path string) error {
	for attempt := 0; attempt < 2; attempt++ {
		lock, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, _ = fmt.Fprintln(lock, os.Getpid())
			return lock.Close()
		}
		payload, readErr := os.ReadFile(path)
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(payload)))
		if readErr != nil || parseErr != nil || pid < 1 {
			return errors.New("job supervisor lock is invalid; inspect it before removing it")
		}
		process, findErr := os.FindProcess(pid)
		alive := findErr == nil && process.Signal(syscall.Signal(0)) == nil
		if alive {
			return fmt.Errorf("job supervisor is already running with PID %d", pid)
		}
		if removeErr := os.Remove(path); removeErr != nil {
			return fmt.Errorf("remove stale job supervisor lock: %w", removeErr)
		}
	}
	return errors.New("could not acquire job supervisor lock")
}

func runDueJobs(store jobs.Store, stateDir string, notify bool, now time.Time) {
	runDueJobsContext(context.Background(), store, stateDir, notify, now)
}
func runDueJobsContext(ctx context.Context, store jobs.Store, stateDir string, notify bool, now time.Time) {
	definitions, err := store.List()
	if err != nil {
		return
	}
	for _, definition := range definitions {
		if !definition.Enabled {
			continue
		}
		scheduled, due, err := jobs.Due(definition, now)
		if err != nil || !due {
			continue
		}
		claimed, attempt, ok, err := store.ClaimBegin(definition.ID, scheduled)
		if err != nil || !ok {
			continue
		}
		attempt = executeJobContext(ctx, claimed, attempt, stateDir, notify)
		if err := store.Record(attempt); err != nil {
			continue
		}
		_, _ = recordJobInbox(stateDir, claimed, attempt)
	}
}

func recordJobInbox(stateDir string, definition jobs.Definition, attempt jobs.Attempt) (inbox.Entry, error) {
	store, err := inbox.Open(stateDir)
	if err != nil {
		return inbox.Entry{}, err
	}
	summary := "Scheduled Work completed."
	if attempt.Status != "completed" {
		summary = "Scheduled Work needs attention: " + attempt.Error
	}
	return store.Add(inbox.Entry{Kind: "job", Title: definition.Name, Summary: boundedJobError(summary), JobID: definition.ID, ConversationID: attempt.Conversation, RevisionID: attempt.Revision, Status: attempt.Status})
}

func inboxCommand(arguments []string, out io.Writer) error {
	_, stateDir, err := jobStore()
	if err != nil {
		return err
	}
	store, err := inbox.Open(stateDir)
	if err != nil {
		return err
	}
	if len(arguments) == 2 && arguments[0] == "read" {
		if err := store.MarkRead(arguments[1], time.Now()); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "Marked inbox entry %s read.\n", arguments[1])
		return err
	}
	unread := false
	if len(arguments) == 1 && arguments[0] == "--unread" {
		unread = true
	} else if len(arguments) != 0 {
		return errors.New("usage: gator inbox [--unread] | gator inbox read ENTRY_ID")
	}
	entries, err := store.List(unread, 100)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		marker := " "
		if entry.ReadAt.IsZero() {
			marker = "*"
		}
		fmt.Fprintf(out, "%s %s\t%s\t%s\t%s\n", marker, entry.ID, entry.CreatedAt.Format("2006-01-02 15:04"), entry.Status, entry.Title)
	}
	return nil
}

func jobSupervisorRequest(actionName string, out io.Writer) error {
	store, _, err := jobStore()
	if err != nil {
		return err
	}
	var state supervisorState
	payload, err := os.ReadFile(filepath.Join(store.Root(), "supervisor.json"))
	if err != nil {
		return errors.New("job supervisor is not running")
	}
	if json.Unmarshal(payload, &state) != nil || state.Version != 1 || state.PID < 1 || state.Nonce == "" {
		return errors.New("job supervisor state is invalid")
	}
	parsed, err := url.Parse(state.URL)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" || parsed.Port() == "" || parsed.User != nil || parsed.Path != "" {
		return errors.New("job supervisor endpoint is not a literal loopback address")
	}
	tokenBytes, err := os.ReadFile(filepath.Join(store.Root(), "supervisor.token"))
	if err != nil {
		return errors.New("job supervisor token is missing")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	token := strings.TrimSpace(string(tokenBytes))
	healthRequest, _ := http.NewRequest(http.MethodGet, state.URL+"/health", nil)
	healthRequest.Header.Set("Authorization", "Bearer "+token)
	healthResponse, err := client.Do(healthRequest)
	if err != nil {
		return errors.New("job supervisor state is stale or unreachable")
	}
	var health supervisorHealth
	decodeErr := json.NewDecoder(healthResponse.Body).Decode(&health)
	_ = healthResponse.Body.Close()
	if healthResponse.StatusCode != http.StatusOK || decodeErr != nil || !health.OK || health.PID != state.PID || health.Nonce != state.Nonce {
		return errors.New("job supervisor identity verification failed")
	}
	if actionName == "health" {
		_, err = fmt.Fprintf(out, "Job supervisor running (PID %d, since %s).\n", state.PID, state.StartedAt.Format(time.RFC3339))
	} else {
		request, _ := http.NewRequest(http.MethodPost, state.URL+"/stop", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response, requestErr := client.Do(request)
		if requestErr != nil {
			return errors.New("job supervisor became unreachable before stop")
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			return fmt.Errorf("job supervisor returned HTTP %d", response.StatusCode)
		}
		_, err = fmt.Fprintln(out, "Job supervisor stop requested.")
	}
	return err
}

func writePrivateSupervisorFiles(root string, state supervisorState, token string) error {
	payload, _ := json.MarshalIndent(state, "", "  ")
	for path, contents := range map[string][]byte{filepath.Join(root, "supervisor.json"): append(payload, '\n'), filepath.Join(root, "supervisor.token"): []byte(token + "\n")} {
		if err := writePrivateFileAtomic(path, contents); err != nil {
			return err
		}
	}
	return nil
}

func writePrivateFileAtomic(path string, contents []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".supervisor-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func desktopNotification(name, status string) {
	if runtime.GOOS == "darwin" {
		_ = exec.Command("osascript", "-e", fmt.Sprintf(`display notification %q with title "Gator: %s"`, name, status)).Run()
		return
	}
	if runtime.GOOS == "linux" {
		if _, err := exec.LookPath("notify-send"); err == nil {
			_ = exec.Command("notify-send", "Gator: "+status, name).Run()
		}
	}
}

func boundedJobError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 4096 {
		value = value[:4096]
	}
	return value
}
