package workrun

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/privatefs"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workspace"
)

const (
	maxWorkSteeringBytes       = 64 * 1024
	maxSpecialistSteeringBytes = 8 * 1024
)

type workControlState struct {
	Version int    `json:"version"`
	RunID   string `json:"run_id"`
	URL     string `json:"url"`
	Token   string `json:"token"`
	Nonce   string `json:"nonce"`
}

type workControl struct {
	server   *http.Server
	listener net.Listener
	root     workspace.Root
	name     string
}

// startWorkControl makes one active foreground run addressable from another
// local CLI process. The token is confined to a private state file. A decision
// endpoint exists only when the foreground operation has an exact pending
// interaction and checks the original in-memory preview digest.
func startWorkControl(ctx context.Context, stateDir, runID string, cancel context.CancelFunc, incoming <-chan string, direct chan string, respond func(int, string, bool) error, cancelTask func(string) error, steerTask func(string, string) error, answerTask func(string, string, map[string][]string) error) (*workControl, <-chan string, error) {
	if !validInteractionRunID(runID) {
		return nil, nil, errors.New("invalid Work control run ID")
	}
	directory := filepath.Join(stateDir, "gator", "work-control")
	if err := privatefs.MkdirAll(directory); err != nil {
		return nil, nil, err
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || privatefs.Check(directory) != nil {
		return nil, nil, errors.New("Work control directory must be private and regular")
	}
	root, err := workspace.Open(directory)
	if err != nil {
		return nil, nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	tokenBytes, nonceBytes := make([]byte, 32), make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		listener.Close()
		return nil, nil, err
	}
	if _, err := rand.Read(nonceBytes); err != nil {
		listener.Close()
		return nil, nil, err
	}
	state := workControlState{Version: 1, RunID: runID, URL: "http://" + listener.Addr().String(), Token: hex.EncodeToString(tokenBytes), Nonce: hex.EncodeToString(nonceBytes)}
	steering := direct
	if steering == nil {
		steering = make(chan string, 16)
	}
	if incoming != nil && direct == nil {
		go func() {
			for {
				select {
				case value, ok := <-incoming:
					if !ok {
						return
					}
					select {
					case steering <- value:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	mux := http.NewServeMux()
	authorized := func(next http.HandlerFunc) http.HandlerFunc {
		return func(writer http.ResponseWriter, request *http.Request) {
			if request.Header.Get("Authorization") != "Bearer "+state.Token {
				http.Error(writer, "unauthorized", http.StatusUnauthorized)
				return
			}
			next(writer, request)
		}
	}
	mux.HandleFunc("/health", authorized(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(writer, "method", http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(struct{ RunID, Nonce string }{state.RunID, state.Nonce})
	}))
	mux.HandleFunc("/stop", authorized(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "method", http.StatusMethodNotAllowed)
			return
		}
		if ctx.Err() != nil {
			http.Error(writer, "run stopped", http.StatusConflict)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
		cancel()
	}))
	mux.HandleFunc("/steer", authorized(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "method", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(request.Body, maxWorkSteeringBytes+1))
		if err != nil || !validWorkSteering(string(body)) {
			http.Error(writer, "invalid steering text", http.StatusBadRequest)
			return
		}
		select {
		case <-ctx.Done():
			http.Error(writer, "run stopped", http.StatusConflict)
		case steering <- string(body):
			writer.WriteHeader(http.StatusAccepted)
		default:
			http.Error(writer, "steering queue is full", http.StatusTooManyRequests)
		}
	}))
	mux.HandleFunc("/decision", authorized(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "method", http.StatusMethodNotAllowed)
			return
		}
		if respond == nil || ctx.Err() != nil {
			http.Error(writer, "decision unavailable", http.StatusConflict)
			return
		}
		var answer struct {
			ID       int    `json:"id"`
			Digest   string `json:"digest"`
			Approved *bool  `json:"approved"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&answer) != nil || answer.ID < 1 || len(answer.Digest) != 64 || answer.Approved == nil {
			http.Error(writer, "invalid decision", http.StatusBadRequest)
			return
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			http.Error(writer, "invalid decision", http.StatusBadRequest)
			return
		}
		record, err := ReadInteractionRecord(stateDir, runID, answer.ID)
		if err != nil {
			http.Error(writer, "decision is unavailable", http.StatusConflict)
			return
		}
		digest, err := InteractionDigest(record.Interaction)
		if record.Status != "pending" || record.LiveStatus != "active" || err != nil || digest != answer.Digest {
			http.Error(writer, "pending decision changed", http.StatusConflict)
			return
		}
		if respond(answer.ID, answer.Digest, *answer.Approved) != nil {
			http.Error(writer, "decision is no longer pending", http.StatusConflict)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	}))
	mux.HandleFunc("/task/cancel", authorized(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "method", http.StatusMethodNotAllowed)
			return
		}
		if cancelTask == nil || ctx.Err() != nil {
			http.Error(writer, "task cancellation unavailable", http.StatusConflict)
			return
		}
		var command struct {
			ID string `json:"id"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 256))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&command) != nil || !validSpecialistTaskID(command.ID) {
			http.Error(writer, "invalid task ID", http.StatusBadRequest)
			return
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			http.Error(writer, "invalid task cancellation request", http.StatusBadRequest)
			return
		}
		if cancelTask(command.ID) != nil {
			http.Error(writer, "task is not active", http.StatusConflict)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	}))
	mux.HandleFunc("/task/steer", authorized(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "method", http.StatusMethodNotAllowed)
			return
		}
		if steerTask == nil || ctx.Err() != nil {
			http.Error(writer, "task steering unavailable", http.StatusConflict)
			return
		}
		var command struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 64*1024))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&command) != nil || !validSpecialistTaskID(command.ID) || !validSpecialistSteering(command.Text) {
			http.Error(writer, "invalid specialist steering", http.StatusBadRequest)
			return
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			http.Error(writer, "invalid specialist steering request", http.StatusBadRequest)
			return
		}
		if steerTask(command.ID, command.Text) != nil {
			http.Error(writer, "specialist cannot be steered", http.StatusConflict)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	}))
	mux.HandleFunc("/task/answer", authorized(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "method", http.StatusMethodNotAllowed)
			return
		}
		if answerTask == nil || ctx.Err() != nil {
			http.Error(writer, "task answers unavailable", http.StatusConflict)
			return
		}
		var command taskAnswer
		decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 32*1024))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&command) != nil || !command.valid() {
			http.Error(writer, "invalid task answer", http.StatusBadRequest)
			return
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			http.Error(writer, "invalid task answer", http.StatusBadRequest)
			return
		}
		if answerTask(command.ID, command.QuestionID, command.Answers) != nil {
			http.Error(writer, "question changed or cannot be answered", http.StatusConflict)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	}))
	control := &workControl{server: &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}, listener: listener, root: root, name: runID + ".json"}
	go func() { _ = control.server.Serve(listener) }()
	payload, err := json.Marshal(state)
	if err == nil {
		err = root.WriteRegularFileAtomic(control.name, payload, 4096)
	}
	if err != nil {
		_ = control.Close()
		return nil, nil, err
	}
	return control, steering, nil
}

func (c *workControl) Close() error {
	shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := c.server.Shutdown(shutdown)
	if closeErr := c.listener.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
		err = errors.Join(err, closeErr)
	}
	root, openErr := os.OpenRoot(c.root.Path())
	if openErr != nil {
		return errors.Join(err, openErr)
	}
	defer root.Close()
	if removeErr := root.Remove(c.name); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		err = errors.Join(err, removeErr)
	}
	return err
}

func validWorkSteering(text string) bool {
	return strings.TrimSpace(text) != "" && len(text) <= maxWorkSteeringBytes && utf8.ValidString(text) && !strings.ContainsRune(text, 0)
}

func validSpecialistTaskID(id string) bool {
	if !strings.HasPrefix(id, "subagent-") {
		return false
	}
	number, err := strconv.Atoi(strings.TrimPrefix(id, "subagent-"))
	return err == nil && number >= 1 && number <= 32 && id == fmt.Sprintf("subagent-%03d", number)
}

func validSpecialistSteering(text string) bool {
	return strings.TrimSpace(text) != "" && len(text) <= maxSpecialistSteeringBytes && utf8.ValidString(text) && !strings.ContainsRune(text, 0)
}

// SendWorkControl checks both the active Work lease and the endpoint's nonce
// before sending a bounded stop or steering request. A stale state file cannot
// authorize or redirect a control command.
func SendWorkControl(ctx context.Context, stateDir, runID, command, text string) error {
	if !validInteractionRunID(runID) || command != "stop" && command != "steer" || command == "steer" && !validWorkSteering(text) || command == "stop" && text != "" {
		return errors.New("invalid Work control request")
	}
	client, state, err := connectWorkControl(ctx, stateDir, runID)
	if err != nil {
		return err
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, state.URL+"/"+command, bytes.NewBufferString(text))
	request.Header.Set("Authorization", "Bearer "+state.Token)
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("Work control %s failed: %w", command, err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("Work control %s returned HTTP %d", command, response.StatusCode)
	}
	return nil
}

// SendWorkDecision answers only the exact, still-pending interaction that was
// displayed to the user. The live operation compares the digest against its
// original in-memory request before it releases the approval callback.
func SendWorkDecision(ctx context.Context, stateDir, runID string, id int, digest string, approved bool) error {
	if id < 1 || len(digest) != 64 {
		return errors.New("invalid Work decision")
	}
	client, state, err := connectWorkControl(ctx, stateDir, runID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(struct {
		ID       int    `json:"id"`
		Digest   string `json:"digest"`
		Approved bool   `json:"approved"`
	}{id, digest, approved})
	if err != nil {
		return err
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, state.URL+"/decision", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+state.Token)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("Work decision failed: %w", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("Work decision returned HTTP %d", response.StatusCode)
	}
	return nil
}

// SendWorkTaskCancel requests cancellation of one active specialist in a live
// foreground Work run. Descendants are cancelled by the supervisor as well.
func SendWorkTaskCancel(ctx context.Context, stateDir, runID, taskID string) error {
	if !validSpecialistTaskID(taskID) {
		return errors.New("invalid specialist task ID")
	}
	client, state, err := connectWorkControl(ctx, stateDir, runID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(struct {
		ID string `json:"id"`
	}{taskID})
	if err != nil {
		return err
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, state.URL+"/task/cancel", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+state.Token)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("Work task cancellation failed: %w", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("Work task cancellation returned HTTP %d", response.StatusCode)
	}
	return nil
}

// SendWorkTaskSteer queues a bounded instruction for one active, steerable
// specialist. Acceptance means it was queued, not that a later model turn ran.
func SendWorkTaskSteer(ctx context.Context, stateDir, runID, taskID, instruction string) error {
	if !validSpecialistTaskID(taskID) || !validSpecialistSteering(instruction) {
		return errors.New("invalid specialist steering request")
	}
	client, state, err := connectWorkControl(ctx, stateDir, runID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}{taskID, instruction})
	if err != nil {
		return err
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, state.URL+"/task/steer", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+state.Token)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("Work specialist steering failed: %w", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("Work specialist steering returned HTTP %d", response.StatusCode)
	}
	return nil
}

type taskAnswer struct {
	ID         string              `json:"id"`
	QuestionID string              `json:"question_id"`
	Answers    map[string][]string `json:"answers"`
}

func (a taskAnswer) valid() bool {
	if !validSpecialistTaskID(a.ID) || len(a.QuestionID) != 64 || len(a.Answers) < 1 || len(a.Answers) > 4 {
		return false
	}
	if _, err := hex.DecodeString(a.QuestionID); err != nil {
		return false
	}
	for id, values := range a.Answers {
		if id == "" || len(id) > 128 || len(values) != 1 || !validSpecialistSteering(values[0]) {
			return false
		}
	}
	return true
}

func SendWorkTaskAnswer(ctx context.Context, stateDir, runID, taskID, questionID string, answers map[string][]string) error {
	command := taskAnswer{taskID, questionID, answers}
	if !command.valid() {
		return errors.New("invalid task answer")
	}
	client, state, err := connectWorkControl(ctx, stateDir, runID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(command)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, state.URL+"/task/answer", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+state.Token)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("Work task answer failed: %w", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("Work task answer returned HTTP %d", response.StatusCode)
	}
	return nil
}

func connectWorkControl(ctx context.Context, stateDir, runID string) (*http.Client, workControlState, error) {
	if !validInteractionRunID(runID) {
		return nil, workControlState{}, errors.New("invalid Work control run ID")
	}
	history, err := workhistory.Open(stateDir)
	if err != nil {
		return nil, workControlState{}, err
	}
	record, err := history.Load(runID)
	if err != nil {
		return nil, workControlState{}, err
	}
	if record.Status != workhistory.Running || record.LiveStatus != "active" {
		return nil, workControlState{}, errors.New("Work run is not active")
	}
	root, err := workspace.Open(filepath.Join(stateDir, "gator", "work-control"))
	if err != nil {
		return nil, workControlState{}, errors.New("active Work run has no control endpoint")
	}
	payload, err := root.ReadRegularFile(runID+".json", 4096)
	if err != nil {
		return nil, workControlState{}, errors.New("active Work run has no readable control endpoint")
	}
	var state workControlState
	if json.Unmarshal(payload, &state) != nil || state.Version != 1 || state.RunID != runID || len(state.Token) != 64 || len(state.Nonce) != 32 {
		return nil, workControlState{}, errors.New("Work control state is invalid")
	}
	if _, err := hex.DecodeString(state.Token); err != nil {
		return nil, workControlState{}, errors.New("Work control token is invalid")
	}
	if _, err := hex.DecodeString(state.Nonce); err != nil {
		return nil, workControlState{}, errors.New("Work control nonce is invalid")
	}
	endpoint, err := url.Parse(state.URL)
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, workControlState{}, errors.New("Work control endpoint must be literal loopback")
	}
	port, err := strconv.Atoi(endpoint.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, workControlState{}, errors.New("Work control endpoint port is invalid")
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	healthRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, state.URL+"/health", nil)
	healthRequest.Header.Set("Authorization", "Bearer "+state.Token)
	healthResponse, err := client.Do(healthRequest)
	if err != nil {
		return nil, workControlState{}, errors.New("Work control endpoint is stale or unreachable")
	}
	var identity struct{ RunID, Nonce string }
	decodeErr := json.NewDecoder(io.LimitReader(healthResponse.Body, 1024)).Decode(&identity)
	_ = healthResponse.Body.Close()
	if healthResponse.StatusCode != http.StatusOK || decodeErr != nil || identity.RunID != runID || identity.Nonce != state.Nonce {
		return nil, workControlState{}, errors.New("Work control identity verification failed")
	}
	return client, state, nil
}
