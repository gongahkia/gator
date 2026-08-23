package terminal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestManagerRunsInteractiveTaskAndBoundsScrollback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the PTY dependency reports unsupported on Windows")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is unavailable")
	}
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var exited []Task
	var exitedMu sync.Mutex
	manager := New(Config{
		Root:           root,
		Policy:         sandbox.Policy{Mode: sandbox.Off},
		MaxOutputBytes: 24,
		MaxReadBytes:   8,
		OnExit: func(task Task) {
			exitedMu.Lock()
			exited = append(exited, task)
			exitedMu.Unlock()
		},
	})
	defer manager.Close()
	task, err := manager.Start(context.Background(), []string{sh, "-lc", "printf 'ready\\n'; IFS= read line; printf 'received:%s\\n' \"$line\""})
	if err != nil {
		t.Fatalf("start terminal: %v", err)
	}
	first := awaitTerminalOutput(t, manager, task.ID, 0, "ready")
	if first.Task.Status != "running" {
		t.Fatalf("task after ready = %#v", first.Task)
	}
	if _, err := manager.Write(task.ID, []byte("Ada\r")); err != nil {
		t.Fatalf("write terminal: %v", err)
	}
	second := awaitTerminalOutput(t, manager, task.ID, first.Next, "received:Ada")
	if second.Next <= first.Next || strings.Contains(second.Output, "ready") {
		t.Fatalf("incremental terminal output = %#v", second)
	}
	awaitTerminalExit(t, manager, task.ID)
	manager.Close()
	exitedMu.Lock()
	defer exitedMu.Unlock()
	if len(exited) != 1 || exited[0].Status != "exited" || exited[0].ExitCode == nil || *exited[0].ExitCode != 0 {
		t.Fatalf("terminal exit callback = %#v", exited)
	}
}

func TestManagerMarksDroppedOutputAndCancelsTasksOnClose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the PTY dependency reports unsupported on Windows")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is unavailable")
	}
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := New(Config{Root: root, Policy: sandbox.Policy{Mode: sandbox.Off}, MaxOutputBytes: 12, MaxReadBytes: 12})
	task, err := manager.Start(context.Background(), []string{sh, "-lc", "printf '0123456789abcdef'"})
	if err != nil {
		t.Fatalf("start output task: %v", err)
	}
	awaitTerminalExit(t, manager, task.ID)
	read, err := manager.Read(task.ID, 0)
	if err != nil {
		t.Fatalf("read terminal: %v", err)
	}
	if !read.Dropped || read.Cursor == 0 || read.Output != "456789abcdef" || !read.Task.OutputTruncated {
		t.Fatalf("bounded output = %#v", read)
	}

	running, err := manager.Start(context.Background(), []string{sh, "-lc", "sleep 30"})
	if err != nil {
		t.Fatalf("start long terminal: %v", err)
	}
	manager.Close()
	state := awaitTerminalExit(t, manager, running.ID)
	if state.Status != "exited" || state.ExitCode == nil || *state.ExitCode != -1 || !strings.Contains(state.Error, "cancelled") {
		t.Fatalf("closed terminal state = %#v", state)
	}
}

func TestManagerRecordsDeveloperInputWithoutRetainingIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the PTY dependency reports unsupported on Windows")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is unavailable")
	}
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var inputs []DeveloperInput
	var inputMu sync.Mutex
	manager := New(Config{
		Root:   root,
		Policy: sandbox.Policy{Mode: sandbox.Off},
		OnDeveloperInput: func(_ Task, input DeveloperInput) {
			inputMu.Lock()
			inputs = append(inputs, input)
			inputMu.Unlock()
		},
	})
	defer manager.Close()
	task, err := manager.Start(context.Background(), []string{sh, "-lc", "IFS= read line; printf 'received:%s\\n' \"$line\""})
	if err != nil {
		t.Fatalf("start terminal: %v", err)
	}
	secret := []byte("developer-only value\r")
	if _, err := manager.WriteDeveloper(task.ID, secret); err != nil {
		t.Fatalf("write developer terminal input: %v", err)
	}
	_ = awaitTerminalOutput(t, manager, task.ID, 0, "received:developer-only value")
	inputMu.Lock()
	defer inputMu.Unlock()
	if len(inputs) != 1 || inputs[0].Bytes != len(secret) {
		t.Fatalf("developer input metadata = %#v", inputs)
	}
	digest := sha256.Sum256(secret)
	if inputs[0].SHA256 != hex.EncodeToString(digest[:]) || strings.Contains(inputs[0].SHA256, "developer-only") {
		t.Fatalf("developer input digest = %#v", inputs[0])
	}
}

func TestManagerAggregatesRawDeveloperInputUntilFlush(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the PTY dependency reports unsupported on Windows")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is unavailable")
	}
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var inputs []DeveloperInput
	var inputMu sync.Mutex
	manager := New(Config{
		Root:   root,
		Policy: sandbox.Policy{Mode: sandbox.Off},
		OnDeveloperInput: func(_ Task, input DeveloperInput) {
			inputMu.Lock()
			inputs = append(inputs, input)
			inputMu.Unlock()
		},
	})
	defer manager.Close()
	task, err := manager.Start(context.Background(), []string{sh, "-lc", "IFS= read line; printf 'received:%s\\n' \"$line\""})
	if err != nil {
		t.Fatalf("start terminal: %v", err)
	}
	raw := []byte("developer raw")
	if _, err := manager.WriteDeveloperRaw(task.ID, raw[:9]); err != nil {
		t.Fatalf("write first raw input: %v", err)
	}
	if _, err := manager.WriteDeveloperRaw(task.ID, raw[9:]); err != nil {
		t.Fatalf("write second raw input: %v", err)
	}
	if _, err := manager.FlushDeveloperInput(task.ID); err != nil {
		t.Fatalf("flush raw input: %v", err)
	}
	inputMu.Lock()
	if len(inputs) != 1 || !inputs[0].Raw || inputs[0].Bytes != len(raw) {
		inputMu.Unlock()
		t.Fatalf("raw developer input metadata = %#v", inputs)
	}
	digest := sha256.Sum256(raw)
	if inputs[0].SHA256 != hex.EncodeToString(digest[:]) || strings.Contains(inputs[0].SHA256, "developer raw") {
		inputMu.Unlock()
		t.Fatalf("raw developer input digest = %#v", inputs[0])
	}
	inputMu.Unlock()
	if _, err := manager.WriteDeveloper(task.ID, []byte{'\r'}); err != nil {
		t.Fatalf("finish raw terminal line: %v", err)
	}
	_ = awaitTerminalOutput(t, manager, task.ID, 0, "received:developer raw")
	inputMu.Lock()
	defer inputMu.Unlock()
	if len(inputs) != 2 || inputs[1].Raw || inputs[1].Bytes != 1 {
		t.Fatalf("raw and line developer metadata = %#v", inputs)
	}
}

func TestManagerSetsAndResizesPseudoTerminalViewport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the PTY dependency reports unsupported on Windows")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is unavailable")
	}
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := New(Config{Root: root, Policy: sandbox.Policy{Mode: sandbox.Off}, Rows: 31, Columns: 97})
	defer manager.Close()
	task, err := manager.Start(context.Background(), []string{sh, "-lc", "stty size; IFS= read line; stty size"})
	if err != nil {
		t.Fatalf("start terminal task: %v", err)
	}
	first := awaitTerminalOutput(t, manager, task.ID, 0, "31 97")
	if first.Task.Rows != 31 || first.Task.Columns != 97 {
		t.Fatalf("initial terminal size = %#v", first.Task)
	}
	resized, err := manager.Attachment().Resize(task.ID, 17, 53)
	if err != nil {
		t.Fatalf("resize terminal task: %v", err)
	}
	if resized.Rows != 17 || resized.Columns != 53 {
		t.Fatalf("resized terminal task = %#v", resized)
	}
	if _, err := manager.WriteDeveloper(task.ID, []byte("continue\r")); err != nil {
		t.Fatalf("write resized terminal task: %v", err)
	}
	second := awaitTerminalOutput(t, manager, task.ID, first.Next, "17 53")
	if second.Task.Rows != 17 || second.Task.Columns != 53 {
		t.Fatalf("resized terminal read = %#v", second.Task)
	}
	if _, err := manager.Resize(task.ID, 0, 53); err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("invalid terminal resize error = %v", err)
	}
}

func awaitTerminalOutput(t *testing.T, manager *Manager, id string, cursor int64, want string) ReadResult {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var last ReadResult
	var output strings.Builder
	next := cursor
	for time.Now().Before(deadline) {
		read, err := manager.Read(id, next)
		if err != nil {
			t.Fatal(err)
		}
		last = read
		if read.Next > next {
			output.WriteString(read.Output)
			next = read.Next
		}
		if strings.Contains(output.String(), want) {
			last.Cursor = cursor
			last.Next = next
			last.Output = output.String()
			return last
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("terminal %s did not emit %q; collected output = %q, last = %#v", id, want, output.String(), last)
	return ReadResult{}
}

func awaitTerminalExit(t *testing.T, manager *Manager, id string) Task {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, task := range manager.List() {
			if task.ID == id && task.Status == "exited" {
				return task
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("terminal %s did not exit", id)
	return Task{}
}
