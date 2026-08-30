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
	process *exec.Cmd
	input   io.WriteCloser
	output  *bufio.Reader
	mu      sync.Mutex
	nextID  uint64
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
	command := exec.CommandContext(ctx, status.Node, filepath.Join(status.Directory, "driver.mjs"))
	command.Dir = status.Directory
	command.Env = append(os.Environ(), "PLAYWRIGHT_BROWSERS_PATH="+filepath.Join(status.Directory, "browsers"))
	input, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open browser driver input: %w", err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open browser driver output: %w", err)
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start browser driver: %w", err)
	}
	driver := &sidecar{process: command, input: input, output: bufio.NewReaderSize(output, 64*1024)}
	var ignored map[string]any
	if err := driver.call(ctx, "start", map[string]any{"mode": config.Mode, "headed": config.Headed, "cdp_endpoint": config.CDPEndpoint}, &ignored); err != nil {
		_ = driver.Close()
		return nil, fmt.Errorf("start browser driver: %w", err)
	}
	return driver, nil
}

func (driver *sidecar) SetOrigins(ctx context.Context, origins []Origin) error {
	var ignored map[string]any
	return driver.call(ctx, "set_origins", struct {
		Origins []Origin `json:"origins"`
	}{origins}, &ignored)
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
	driver.mu.Lock()
	defer driver.mu.Unlock()
	if driver.process == nil {
		return nil
	}
	_ = driver.input.Close()
	err := driver.process.Process.Kill()
	_, waitErr := driver.process.Process.Wait()
	driver.process = nil
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return waitErr
}

func (driver *sidecar) call(_ context.Context, method string, parameters any, target any) error {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	if driver.process == nil {
		return errors.New("browser driver is closed")
	}
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
		return fmt.Errorf("write browser driver request: %w", err)
	}
	line, err := readSidecarLine(driver.output, maxSidecarResponseBytes)
	if err != nil {
		return fmt.Errorf("read browser driver response: %w", err)
	}
	var response struct {
		ID     uint64          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(line, &response); err != nil {
		return errors.New("browser driver returned invalid JSON")
	}
	if response.ID != request.ID {
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
