package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
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
	"github.com/gongahkia/gator/internal/artifact"
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
	StartedAt time.Time `json:"started_at"`
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
	absolute, err := filepath.Abs(*source)
	if err != nil {
		return err
	}
	store, _, err := jobStore()
	if err != nil {
		return err
	}
	definition, err := store.Save(jobs.Definition{Name: name, Enabled: true, Schedule: *schedule, Timezone: *timezone, Missed: *missed, SourcePath: absolute, RefreshSnapshot: true, Objective: objective, Provider: *provider, Model: *model, Mode: mode, Contract: contract, ConnectorIDs: connectors, MaxSteps: *maxSteps})
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
		attempt := executeJob(definition, time.Now().UTC(), stateDir, true)
		_ = store.Record(attempt)
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
			definition.SourcePath, _ = filepath.Abs(*source)
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

func executeJob(definition jobs.Definition, scheduledAt time.Time, stateDir string, notify bool) jobs.Attempt {
	attempt := jobs.Attempt{Version: jobs.Version, JobID: definition.ID, ScheduledAt: scheduledAt, StartedAt: time.Now().UTC()}
	var result jobResult
	var runErr error
	for try := 1; try <= 3; try++ {
		attempt.Try = try
		result, runErr = runJobProcess(definition, stateDir)
		if runErr == nil {
			break
		}
		if strings.Contains(strings.ToLower(runErr.Error()), "outcome uncertain") || strings.Contains(strings.ToLower(runErr.Error()), "unknown") {
			break
		}
		if try < 3 {
			time.Sleep(time.Duration(try) * time.Second)
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
	arguments := []string{"work", "--json", "--source", definition.SourcePath, "--mode", string(definition.Mode), "--actions", string(definition.Contract.ExternalActions), "--max-steps", strconv.Itoa(definition.MaxSteps)}
	if definition.Provider != "" {
		arguments = append(arguments, "--provider", definition.Provider)
	}
	if definition.Model != "" {
		arguments = append(arguments, "--model", definition.Model)
	}
	for _, requirement := range definition.Contract.Artifacts {
		arguments = append(arguments, "--artifact", requirement.Path)
		for _, validation := range requirement.Validations {
			if validation.Kind == artifact.Contains {
				arguments = append(arguments, "--require-contains", requirement.Path+"="+validation.Value)
			}
		}
	}
	for _, connectorID := range definition.ConnectorIDs {
		arguments = append(arguments, "--connector", connectorID)
	}
	arguments = append(arguments, "--", definition.Objective)
	executable, err := os.Executable()
	if err != nil {
		return jobResult{}, err
	}
	command := exec.Command(executable, arguments...)
	command.Env = append(os.Environ(), "GATOR_STATE_DIR="+stateDir)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	runErr := command.Run()
	var result jobResult
	decodeErr := json.Unmarshal(output.Bytes(), &result)
	if runErr != nil {
		if result.Error != "" {
			return result, errors.New(result.Error)
		}
		return result, fmt.Errorf("scheduled Work process: %w: %s", runErr, boundedJobError(output.String()))
	}
	if decodeErr != nil {
		return result, fmt.Errorf("decode scheduled Work result: %w", decodeErr)
	}
	if result.Error != "" {
		return result, errors.New(result.Error)
	}
	return result, nil
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
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return err
	}
	token := hex.EncodeToString(tokenBytes)
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
	mux.HandleFunc("/health", authorized(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"ok":true}`)
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
	state := supervisorState{Version: 1, PID: os.Getpid(), URL: "http://" + listener.Addr().String(), StartedAt: time.Now().UTC()}
	if err := writePrivateSupervisorFiles(store.Root(), state, token); err != nil {
		return err
	}
	defer os.Remove(filepath.Join(store.Root(), "supervisor.json"))
	defer os.Remove(filepath.Join(store.Root(), "supervisor.token"))
	fmt.Fprintf(out, "Gator job supervisor running at %s (PID %d). Keep this terminal open.\n", state.URL, state.PID)
	runDueJobs(store, stateDir, *notify, time.Now())
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
			runDueJobs(store, stateDir, *notify, now)
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
		claimed, ok, err := store.Claim(definition.ID, scheduled)
		if err != nil || !ok {
			continue
		}
		attempt := executeJob(claimed, scheduled, stateDir, notify)
		_ = store.Record(attempt)
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
	unread := false
	if len(arguments) == 1 && arguments[0] == "--unread" {
		unread = true
	} else if len(arguments) != 0 {
		return errors.New("usage: gator inbox [--unread]")
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
	if json.Unmarshal(payload, &state) != nil {
		return errors.New("job supervisor state is invalid")
	}
	tokenBytes, err := os.ReadFile(filepath.Join(store.Root(), "supervisor.token"))
	if err != nil {
		return errors.New("job supervisor token is missing")
	}
	method := http.MethodGet
	if actionName == "stop" {
		method = http.MethodPost
	}
	request, _ := http.NewRequest(method, state.URL+"/"+actionName, nil)
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(tokenBytes)))
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("job supervisor state is stale or unreachable")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("job supervisor returned HTTP %d", response.StatusCode)
	}
	if actionName == "health" {
		_, err = fmt.Fprintf(out, "Job supervisor running (PID %d, since %s).\n", state.PID, state.StartedAt.Format(time.RFC3339))
	} else {
		_, err = fmt.Fprintln(out, "Job supervisor stop requested.")
	}
	return err
}

func writePrivateSupervisorFiles(root string, state supervisorState, token string) error {
	payload, _ := json.MarshalIndent(state, "", "  ")
	for path, contents := range map[string][]byte{filepath.Join(root, "supervisor.json"): append(payload, '\n'), filepath.Join(root, "supervisor.token"): []byte(token + "\n")} {
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			return err
		}
	}
	return nil
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
