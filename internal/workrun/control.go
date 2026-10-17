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
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workspace"
)

const maxWorkSteeringBytes = 64 * 1024

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
// local CLI process. The token is confined to a private state file; no endpoint
// accepts approvals or extends the run's tool permissions.
func startWorkControl(ctx context.Context, stateDir, runID string, cancel context.CancelFunc, incoming <-chan string, direct chan string) (*workControl, <-chan string, error) {
	if !validInteractionRunID(runID) {
		return nil, nil, errors.New("invalid Work control run ID")
	}
	directory := filepath.Join(stateDir, "gator", "work-control")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, nil, err
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
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

// SendWorkControl checks both the active Work lease and the endpoint's nonce
// before sending a bounded stop or steering request. A stale state file cannot
// authorize or redirect a control command.
func SendWorkControl(ctx context.Context, stateDir, runID, command, text string) error {
	if !validInteractionRunID(runID) || command != "stop" && command != "steer" || command == "steer" && !validWorkSteering(text) || command == "stop" && text != "" {
		return errors.New("invalid Work control request")
	}
	history, err := workhistory.Open(stateDir)
	if err != nil {
		return err
	}
	record, err := history.Load(runID)
	if err != nil {
		return err
	}
	if record.Status != workhistory.Running || record.LiveStatus != "active" {
		return errors.New("Work run is not active")
	}
	root, err := workspace.Open(filepath.Join(stateDir, "gator", "work-control"))
	if err != nil {
		return errors.New("active Work run has no control endpoint")
	}
	payload, err := root.ReadRegularFile(runID+".json", 4096)
	if err != nil {
		return errors.New("active Work run has no readable control endpoint")
	}
	var state workControlState
	if json.Unmarshal(payload, &state) != nil || state.Version != 1 || state.RunID != runID || len(state.Token) != 64 || len(state.Nonce) != 32 {
		return errors.New("Work control state is invalid")
	}
	if _, err := hex.DecodeString(state.Token); err != nil {
		return errors.New("Work control token is invalid")
	}
	if _, err := hex.DecodeString(state.Nonce); err != nil {
		return errors.New("Work control nonce is invalid")
	}
	endpoint, err := url.Parse(state.URL)
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return errors.New("Work control endpoint must be literal loopback")
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}}
	healthRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, state.URL+"/health", nil)
	healthRequest.Header.Set("Authorization", "Bearer "+state.Token)
	healthResponse, err := client.Do(healthRequest)
	if err != nil {
		return errors.New("Work control endpoint is stale or unreachable")
	}
	var identity struct{ RunID, Nonce string }
	decodeErr := json.NewDecoder(io.LimitReader(healthResponse.Body, 1024)).Decode(&identity)
	_ = healthResponse.Body.Close()
	if healthResponse.StatusCode != http.StatusOK || decodeErr != nil || identity.RunID != runID || identity.Nonce != state.Nonce {
		return errors.New("Work control identity verification failed")
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
