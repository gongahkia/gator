package browser

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/hostproc"
	"github.com/gongahkia/gator/internal/privatefs"
)

const maxSidecarResponseBytes = 4 * 1024 * 1024

// SidecarConfig starts a pinned local Playwright process. CDPEndpoint is used
// only by the daemon process in attached mode and is never persisted.
type SidecarConfig struct {
	Store       *Store
	Mode        Mode
	Headed      bool
	CDPEndpoint string
}

type sidecar struct {
	process   *exec.Cmd
	input     io.WriteCloser
	output    *bufio.Reader
	pipe      io.ReadCloser
	gate      chan struct{}
	closed    chan struct{}
	exited    chan struct{}
	done      chan struct{}
	once      sync.Once
	nextID    uint64
	directory string
	closeErr  error
}

// StartSidecar verifies the explicitly installed runtime and launches the
// local JS driver. The process is owned by the daemon and exits with it.
func StartSidecar(ctx context.Context, config SidecarConfig) (Driver, error) {
	if config.Store == nil {
		return nil, errors.New("browser store is required")
	}
	if config.Mode != ModeManaged && config.Mode != ModeAttached {
		return nil, errors.New("browser mode is invalid")
	}
	if config.Mode == ModeAttached {
		if err := ValidateCDPEndpoint(config.CDPEndpoint); err != nil {
			return nil, err
		}
	}
	status := Runtime(config.Store)
	if !status.Installed {
		return nil, errors.New(status.Error)
	}
	command := hostproc.Command(status.Node, filepath.Join(status.Directory, "driver.mjs"))
	command.Dir = status.Directory
	command.Env = append(os.Environ(), "PLAYWRIGHT_BROWSERS_PATH="+filepath.Join(status.Directory, "browsers"))
	driver, err := startSidecarProcess(ctx, command, config.Store.Directory())
	if err != nil {
		return nil, err
	}
	var ignored map[string]any
	if err := driver.call(ctx, "start", map[string]any{"mode": config.Mode, "headed": config.Headed, "cdp_endpoint": config.CDPEndpoint}, &ignored); err != nil {
		_ = driver.Close()
		return nil, fmt.Errorf("start browser driver: %w", err)
	}
	return driver, nil
}

func startSidecarProcess(ctx context.Context, command *exec.Cmd, scratchRoot string) (*sidecar, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp(scratchRoot, ".process-")
	if err != nil {
		return nil, fmt.Errorf("create private browser scratch directory: %w", err)
	}
	started := false
	defer func() {
		if !started {
			_ = os.RemoveAll(directory)
		}
	}()
	if err := privatefs.Protect(directory); err != nil {
		return nil, err
	}
	if command.Env == nil {
		command.Env = os.Environ()
	}
	// Playwright normally creates profiles/downloads in the system temp folder.
	// Own their parent so forced driver termination cannot strand login data.
	command.Env = append(command.Env, "TMPDIR="+directory, "TMP="+directory, "TEMP="+directory)
	input, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open browser driver input: %w", err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		_ = input.Close()
		return nil, fmt.Errorf("open browser driver output: %w", err)
	}
	if err := command.Start(); err != nil {
		_ = input.Close()
		_ = output.Close()
		return nil, fmt.Errorf("start browser driver: %w", err)
	}
	started = true
	driver := &sidecar{process: command, input: input, pipe: output, output: bufio.NewReaderSize(output, 64*1024), gate: make(chan struct{}, 1), closed: make(chan struct{}), exited: make(chan struct{}), done: make(chan struct{}), directory: directory}
	go func() {
		_ = command.Wait()
		close(driver.exited)
		driver.shutdown()
	}()
	go func() {
		select {
		case <-ctx.Done():
			driver.shutdown()
		case <-driver.closed:
		}
	}()
	return driver, nil
}

func (driver *sidecar) SetOrigins(ctx context.Context, origins []Origin) error {
	var ignored map[string]any
	return driver.call(ctx, "set_origins", struct {
		Origins []Origin `json:"origins"`
	}{origins}, &ignored)
}

func (driver *sidecar) InvalidateReferences(ctx context.Context) error {
	var ignored map[string]any
	return driver.call(ctx, "invalidate_references", nil, &ignored)
}

func (driver *sidecar) CandidateTabs(ctx context.Context) ([]Tab, error) {
	var result []Tab
	err := driver.call(ctx, "candidate_tabs", nil, &result)
	return result, err
}

func (driver *sidecar) Snapshot(ctx context.Context, tabID string) (Snapshot, error) {
	var result Snapshot
	err := driver.call(ctx, "snapshot", struct {
		TabID string `json:"tab_id"`
	}{tabID}, &result)
	return result, err
}

func (driver *sidecar) Screenshot(ctx context.Context, tabID string) ([]byte, error) {
	var result struct {
		PNG []byte `json:"png"`
	}
	err := driver.call(ctx, "screenshot", struct {
		TabID string `json:"tab_id"`
	}{tabID}, &result)
	if err == nil && (len(result.PNG) == 0 || len(result.PNG) > 2*1024*1024) {
		err = errors.New("browser screenshot must contain 1 byte-2 MiB")
	}
	return result.PNG, err
}

func (driver *sidecar) Navigate(ctx context.Context, tabID, target string) (Snapshot, error) {
	var result Snapshot
	err := driver.call(ctx, "navigate", struct {
		TabID string `json:"tab_id"`
		URL   string `json:"url"`
	}{tabID, target}, &result)
	return result, err
}

func (driver *sidecar) Click(ctx context.Context, tabID, ref string) (Snapshot, error) {
	return driver.elementAction(ctx, "click", tabID, ref, "")
}

func (driver *sidecar) Fill(ctx context.Context, tabID, ref, value string) (Snapshot, error) {
	return driver.elementAction(ctx, "fill", tabID, ref, value)
}

func (driver *sidecar) Select(ctx context.Context, tabID, ref, value string) (Snapshot, error) {
	return driver.elementAction(ctx, "select", tabID, ref, value)
}

func (driver *sidecar) Press(ctx context.Context, tabID, key string) (Snapshot, error) {
	var result Snapshot
	err := driver.call(ctx, "press", struct {
		TabID string `json:"tab_id"`
		Key   string `json:"key"`
	}{tabID, key}, &result)
	return result, err
}

func (driver *sidecar) Download(ctx context.Context, tabID, ref string) (Download, error) {
	var result Download
	err := driver.call(ctx, "download", struct {
		TabID string `json:"tab_id"`
		Ref   string `json:"ref"`
	}{tabID, ref}, &result)
	return result, err
}

func (driver *sidecar) Upload(ctx context.Context, tabID, ref, path string) (Snapshot, error) {
	var result Snapshot
	err := driver.call(ctx, "upload", struct {
		TabID string `json:"tab_id"`
		Ref   string `json:"ref"`
		Path  string `json:"path"`
	}{tabID, ref, path}, &result)
	return result, err
}

func (driver *sidecar) elementAction(ctx context.Context, method, tabID, ref, value string) (Snapshot, error) {
	var result Snapshot
	parameters := struct {
		TabID string `json:"tab_id"`
		Ref   string `json:"ref"`
		Value string `json:"value,omitempty"`
	}{tabID, ref, value}
	err := driver.call(ctx, method, parameters, &result)
	return result, err
}

func (driver *sidecar) Close() error {
	driver.shutdown()
	<-driver.done
	return driver.closeErr
}

// Done closes as soon as this engine becomes unusable, so the controller can
// revoke its session before waiting for native browser cleanup.
func (driver *sidecar) Done() <-chan struct{} { return driver.closed }

func (driver *sidecar) shutdown() {
	driver.once.Do(func() {
		close(driver.closed)
		// EOF asks the JS driver to close its managed browser (or disconnect an
		// attached one). Closing both pipes also interrupts blocked Go I/O.
		_ = driver.input.Close()
		_ = driver.pipe.Close()
		go func() {
			defer close(driver.done)
			defer func() {
				// Windows can briefly retain Chromium's open profile handles after
				// Node exits. Retry deletion, but never report cleanup as successful
				// if those private files remain.
				deadline := time.Now().Add(2 * time.Second)
				for {
					err := os.RemoveAll(driver.directory)
					if err == nil {
						return
					}
					if time.Now().After(deadline) {
						driver.closeErr = fmt.Errorf("remove private browser scratch directory: %w", err)
						return
					}
					time.Sleep(50 * time.Millisecond)
				}
			}()
			timer := time.NewTimer(2 * time.Second)
			defer timer.Stop()
			select {
			case <-driver.exited:
				return
			case <-timer.C:
				_ = driver.process.Process.Kill()
				<-driver.exited
			}
		}()
	})
}

func (driver *sidecar) call(ctx context.Context, method string, parameters any, target any) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	select {
	case driver.gate <- struct{}{}:
		defer func() { <-driver.gate }()
	case <-ctx.Done():
		return ctx.Err()
	case <-driver.closed:
		return errors.New("browser driver is closed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-driver.closed:
		return errors.New("browser driver is closed")
	default:
	}
	// An admitted request cannot be retried safely after interruption: the
	// browser may already have acted. Discard the stream and its references.
	stopCancel := context.AfterFunc(ctx, driver.shutdown)
	defer stopCancel()
	err := driver.exchange(method, parameters, target)
	if ctx.Err() != nil {
		driver.shutdown()
		return fmt.Errorf("browser operation interrupted; outcome may be unknown; open a new session: %w", ctx.Err())
	}
	return err
}

func (driver *sidecar) exchange(method string, parameters any, target any) error {
	driver.nextID++
	request := struct {
		ID     uint64 `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}{driver.nextID, method, parameters}
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode browser driver request: %w", err)
	}
	if len(payload) > maxSidecarResponseBytes {
		return errors.New("browser driver request exceeds 4 MiB")
	}
	if _, err := driver.input.Write(append(payload, '\n')); err != nil {
		driver.shutdown()
		return fmt.Errorf("write browser driver request: %w", err)
	}
	line, err := readSidecarLine(driver.output, maxSidecarResponseBytes)
	if err != nil {
		driver.shutdown()
		return fmt.Errorf("read browser driver response: %w", err)
	}
	var response struct {
		ID     uint64          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(line, &response); err != nil {
		driver.shutdown()
		return errors.New("browser driver returned invalid JSON")
	}
	if response.ID != request.ID {
		driver.shutdown()
		return errors.New("browser driver returned a mismatched response")
	}
	if response.Error != "" {
		return errors.New(response.Error)
	}
	if target != nil && json.Unmarshal(response.Result, target) != nil {
		return errors.New("browser driver returned an invalid result")
	}
	return nil
}

func readSidecarLine(reader *bufio.Reader, maximum int) ([]byte, error) {
	line, err := reader.ReadBytes('\n')
	if len(line) > maximum {
		return nil, errors.New("browser driver response exceeds 4 MiB")
	}
	if err != nil {
		return nil, err
	}
	return line, nil
}
