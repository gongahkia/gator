package browser

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const playwrightVersion = "1.56.1"

//go:embed runtime/package.json
var runtimePackageJSON []byte

//go:embed runtime/package-lock.json
var runtimePackageLock []byte

//go:embed runtime/driver.mjs
var runtimeDriver []byte

// RuntimeStatus reports only local runtime metadata. It performs no download
// and never treats a system browser as an implicit Gator browser capability.
type RuntimeStatus struct {
	Installed bool
	Directory string
	Node      string
	Error     string
}

func runtimeDirectory(store *Store) string {
	return filepath.Join(store.Directory(), "runtime", "playwright-"+playwrightVersion)
}

func Runtime(store *Store) RuntimeStatus {
	if store == nil {
		return RuntimeStatus{Error: "browser store is unavailable"}
	}
	directory := runtimeDirectory(store)
	status := RuntimeStatus{Directory: directory}
	node, err := exec.LookPath("node")
	if err != nil {
		status.Error = "Node.js is required; install Node.js 18 or newer before running 'gator browser install'"
		return status
	}
	status.Node = node
	for _, relative := range []string{"driver.mjs", "node_modules/playwright/package.json", "browsers"} {
		if _, err := os.Stat(filepath.Join(directory, relative)); err != nil {
			status.Error = "runtime is not installed; run 'gator browser install'"
			return status
		}
	}
	status.Installed = true
	return status
}

// InstallRuntime explicitly installs the lockfile-pinned Playwright package
// and Chromium below Gator's private state directory. It intentionally never
// runs during `gator run`, `gator tui`, or session start.
func InstallRuntime(ctx context.Context, store *Store) (RuntimeStatus, error) {
	if store == nil {
		return RuntimeStatus{}, errors.New("browser store is required")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return RuntimeStatus{}, errors.New("Node.js is required; install Node.js 18 or newer before running 'gator browser install'")
	}
	npm, err := exec.LookPath("npm")
	if err != nil {
		return RuntimeStatus{}, errors.New("npm is required; install Node.js/npm before running 'gator browser install'")
	}
	directory := runtimeDirectory(store)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return RuntimeStatus{}, fmt.Errorf("create browser runtime directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return RuntimeStatus{}, fmt.Errorf("protect browser runtime directory: %w", err)
	}
	for _, file := range []struct {
		name     string
		contents []byte
	}{
		{"package.json", runtimePackageJSON},
		{"package-lock.json", runtimePackageLock},
		{"driver.mjs", runtimeDriver},
	} {
		if err := os.WriteFile(filepath.Join(directory, file.name), file.contents, 0o600); err != nil {
			return RuntimeStatus{}, fmt.Errorf("write pinned browser runtime %s: %w", file.name, err)
		}
	}
	if err := runRuntimeCommand(ctx, directory, npm, []string{"ci", "--ignore-scripts", "--omit=dev"}, nil); err != nil {
		return RuntimeStatus{}, fmt.Errorf("install pinned Playwright package: %w", err)
	}
	browsers := filepath.Join(directory, "browsers")
	if err := os.MkdirAll(browsers, 0o700); err != nil {
		return RuntimeStatus{}, fmt.Errorf("create browser binary directory: %w", err)
	}
	if err := runRuntimeCommand(ctx, directory, node, []string{filepath.Join(directory, "node_modules", "playwright", "cli.js"), "install", "chromium"}, []string{"PLAYWRIGHT_BROWSERS_PATH=" + browsers}); err != nil {
		return RuntimeStatus{}, fmt.Errorf("install pinned Chromium binary: %w", err)
	}
	status := Runtime(store)
	if !status.Installed {
		return status, errors.New(status.Error)
	}
	return status, nil
}

func runRuntimeCommand(ctx context.Context, directory, executable string, arguments, extraEnvironment []string) error {
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), extraEnvironment...)
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if len(message) > 2048 {
		message = message[len(message)-2048:]
	}
	if message == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, message)
}
