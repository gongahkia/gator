// Package terminal manages bounded, sandboxed pseudo-terminal tasks for one
// Gator execution. It deliberately is not a general host shell facility.
package terminal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workspace"
)

const (
	defaultMaxTasks       = 4
	defaultLifetime       = 15 * time.Minute
	defaultOutputBytes    = 256 * 1024
	defaultReadBytes      = 16 * 1024
	defaultTerminalRows   = 24
	defaultTerminalCols   = 80
	maxTerminalRows       = 300
	maxTerminalCols       = 500
	maxTerminalInputBytes = 16 * 1024
)

// Config fixes the resource and policy limits for all terminal tasks belonging
// to one agent run.
type Config struct {
	Root           workspace.Root
	Policy         sandbox.Policy
	MaxTasks       int
	Lifetime       time.Duration
	MaxOutputBytes int
	MaxReadBytes   int
	Rows           int
	Columns        int
	Now            func() time.Time
	OnExit         func(Task)
	// OnDeveloperInput records a user-attached terminal write without exposing
	// its raw input. Model tool writes do not use this callback.
	OnDeveloperInput func(Task, DeveloperInput)
}

// Manager owns every pseudo-terminal task started during a run. Close cancels
// all active tasks and waits briefly for their sandbox scratch cleanup.
type Manager struct {
	root             workspace.Root
	policy           sandbox.Policy
	maxTasks         int
	lifetime         time.Duration
	maxOutputBytes   int
	maxReadBytes     int
	rows             int
	columns          int
	now              func() time.Time
	onExit           func(Task)
	onDeveloperInput func(Task, DeveloperInput)

	mu     sync.Mutex
	nextID uint64
	tasks  map[string]*task
}

// Attachment is the restricted developer-facing view of an active terminal
// manager. It cannot create a process or alter the run's sandbox policy.
type Attachment interface {
	List() []Task
	Read(string, int64) (ReadResult, error)
	Resize(string, int, int) (Task, error)
	WriteDeveloper(string, []byte) (Task, error)
	Stop(string) (Task, error)
}

type attachment struct {
	manager *Manager
}

func (a attachment) List() []Task {
	if a.manager == nil {
		return nil
	}
	return a.manager.List()
}

func (a attachment) Read(id string, cursor int64) (ReadResult, error) {
	if a.manager == nil {
		return ReadResult{}, errors.New("terminal attachment is unavailable")
	}
	return a.manager.Read(id, cursor)
}

func (a attachment) Resize(id string, rows, columns int) (Task, error) {
	if a.manager == nil {
		return Task{}, errors.New("terminal attachment is unavailable")
	}
	return a.manager.Resize(id, rows, columns)
}

func (a attachment) WriteDeveloper(id string, input []byte) (Task, error) {
	if a.manager == nil {
		return Task{}, errors.New("terminal attachment is unavailable")
	}
	return a.manager.WriteDeveloper(id, input)
}

func (a attachment) Stop(id string) (Task, error) {
	if a.manager == nil {
		return Task{}, errors.New("terminal attachment is unavailable")
	}
	return a.manager.Stop(id)
}

// Attachment exposes only operations needed to view and interact with tasks
// that the model has already started.
func (m *Manager) Attachment() Attachment {
	return attachment{manager: m}
}

// DeveloperInput identifies direct terminal input without retaining the bytes.
// It is suitable for activity/journal metadata only.
type DeveloperInput struct {
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// Task describes bounded, non-sensitive terminal state. Output is retrieved
// separately with Read so status events never put raw terminal data in the
// journal.
type Task struct {
	ID              string    `json:"id"`
	Argv            []string  `json:"argv"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at,omitempty"`
	Status          string    `json:"status"`
	ExitCode        *int      `json:"exit_code,omitempty"`
	Error           string    `json:"error,omitempty"`
	OutputTruncated bool      `json:"output_truncated"`
	Rows            int       `json:"rows"`
	Columns         int       `json:"columns"`
}

// ReadResult returns output beginning at Cursor. If Dropped is true, the
// requested cursor fell outside the bounded in-memory scrollback.
type ReadResult struct {
	Task    Task   `json:"task"`
	Cursor  int64  `json:"cursor"`
	Next    int64  `json:"next"`
	Output  string `json:"output"`
	Dropped bool   `json:"dropped"`
}

type task struct {
	manager *Manager
	id      string
	argv    []string
	started time.Time

	mu       sync.Mutex
	file     *os.File
	command  *exec.Cmd
	cancel   context.CancelFunc
	cleanup  func()
	done     chan struct{}
	ended    time.Time
	status   string
	exitCode *int
	err      string
	rows     int
	columns  int
	output   terminalBuffer
	inputMu  sync.Mutex
}

// New creates an empty task manager. Invalid or missing values use intentionally
// conservative defaults; sandbox policy validation still happens at start.
func New(config Config) *Manager {
	if config.MaxTasks <= 0 {
		config.MaxTasks = defaultMaxTasks
	}
	if config.Lifetime <= 0 {
		config.Lifetime = defaultLifetime
	}
	if config.MaxOutputBytes <= 0 {
		config.MaxOutputBytes = defaultOutputBytes
	}
	if config.MaxReadBytes <= 0 {
		config.MaxReadBytes = defaultReadBytes
	}
	if config.Rows <= 0 {
		config.Rows = defaultTerminalRows
	}
	if config.Columns <= 0 {
		config.Columns = defaultTerminalCols
	}
	config.Rows = minTerminalDimension(config.Rows, maxTerminalRows)
	config.Columns = minTerminalDimension(config.Columns, maxTerminalCols)
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Manager{
		root:             config.Root,
		policy:           config.Policy,
		maxTasks:         config.MaxTasks,
		lifetime:         config.Lifetime,
		maxOutputBytes:   config.MaxOutputBytes,
		maxReadBytes:     config.MaxReadBytes,
		rows:             config.Rows,
		columns:          config.Columns,
		now:              config.Now,
		onExit:           config.OnExit,
		onDeveloperInput: config.OnDeveloperInput,
		tasks:            make(map[string]*task),
	}
}

// Start creates a new pseudo-terminal task under the configured sandbox. It
// returns as soon as the process is running; use Read or List to observe it.
func (m *Manager) Start(ctx context.Context, argv []string) (Task, error) {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return Task{}, errors.New("terminal task argv is required")
	}
	if m.root.Path() == "" {
		return Task{}, errors.New("terminal task workspace root is required")
	}
	m.mu.Lock()
	if m.activeLocked() >= m.maxTasks {
		m.mu.Unlock()
		return Task{}, fmt.Errorf("at most %d terminal tasks may run concurrently", m.maxTasks)
	}
	m.nextID++
	id := fmt.Sprintf("term-%03d", m.nextID)
	m.mu.Unlock()

	taskContext, cancel := context.WithTimeout(ctx, m.lifetime)
	prepared, err := sandbox.Prepare(taskContext, sandbox.Request{Dir: m.root.Path(), Argv: append([]string(nil), argv...), Policy: m.policy})
	if err != nil {
		cancel()
		return Task{}, fmt.Errorf("prepare terminal task: %w", err)
	}
	file, err := pty.StartWithSize(prepared.Command, &pty.Winsize{Rows: uint16(m.rows), Cols: uint16(m.columns)})
	if err != nil {
		prepared.Cleanup()
		cancel()
		return Task{}, fmt.Errorf("start terminal task: %w", err)
	}
	started := m.now()
	running := &task{
		manager: m,
		id:      id,
		argv:    append([]string(nil), argv...),
		started: started,
		file:    file,
		command: prepared.Command,
		cancel:  cancel,
		cleanup: prepared.Cleanup,
		done:    make(chan struct{}),
		status:  "running",
		rows:    m.rows,
		columns: m.columns,
		output:  terminalBuffer{limit: m.maxOutputBytes},
	}
	m.mu.Lock()
	m.tasks[id] = running
	m.mu.Unlock()
	go running.copyOutput()
	go running.wait(taskContext)
	return running.snapshot(), nil
}

// Resize updates the pseudo-terminal's window size for an attached task. It
// cannot create a process or change its sandbox; it only lets a real terminal
// program adapt its layout to the developer's bounded TUI viewport.
func (m *Manager) Resize(id string, rows, columns int) (Task, error) {
	if rows < 1 || rows > maxTerminalRows || columns < 1 || columns > maxTerminalCols {
		return Task{}, fmt.Errorf("terminal size must be between 1x1 and %dx%d", maxTerminalRows, maxTerminalCols)
	}
	running, err := m.lookup(id)
	if err != nil {
		return Task{}, err
	}
	running.mu.Lock()
	if running.status != "running" || running.file == nil {
		running.mu.Unlock()
		return Task{}, fmt.Errorf("terminal task %q is not running", id)
	}
	if running.rows == rows && running.columns == columns {
		snapshot := running.snapshotLocked()
		running.mu.Unlock()
		return snapshot, nil
	}
	file := running.file
	running.mu.Unlock()
	running.inputMu.Lock()
	err = pty.Setsize(file, &pty.Winsize{Rows: uint16(rows), Cols: uint16(columns)})
	running.inputMu.Unlock()
	if err != nil {
		return Task{}, fmt.Errorf("resize terminal task %q: %w", id, err)
	}
	running.mu.Lock()
	if running.status == "running" && running.file == file {
		running.rows = rows
		running.columns = columns
	}
	snapshot := running.snapshotLocked()
	running.mu.Unlock()
	return snapshot, nil
}

// Read returns a bounded increment of terminal output. A negative cursor is
// invalid; a cursor past the latest output returns an empty increment.
func (m *Manager) Read(id string, cursor int64) (ReadResult, error) {
	if cursor < 0 {
		return ReadResult{}, errors.New("terminal output cursor must not be negative")
	}
	running, err := m.lookup(id)
	if err != nil {
		return ReadResult{}, err
	}
	running.mu.Lock()
	defer running.mu.Unlock()
	output, actual, next, dropped := running.output.read(cursor, m.maxReadBytes)
	return ReadResult{Task: running.snapshotLocked(), Cursor: actual, Next: next, Output: string(output), Dropped: dropped}, nil
}

// Write sends bounded input to a still-running pseudo-terminal. Authorization
// belongs to the tool layer; this manager only owns the process resource.
func (m *Manager) Write(id string, input []byte) (Task, error) {
	return m.write(id, input, false)
}

// WriteDeveloper sends direct developer input to an existing attached task.
// It follows the task's already-fixed sandbox but does not reuse the agent's
// command approval because a developer performed the input action locally.
func (m *Manager) WriteDeveloper(id string, input []byte) (Task, error) {
	return m.write(id, input, true)
}

func (m *Manager) write(id string, input []byte, developer bool) (Task, error) {
	if len(input) == 0 {
		return Task{}, errors.New("terminal input is required")
	}
	if len(input) > maxTerminalInputBytes {
		return Task{}, fmt.Errorf("terminal input exceeds %d bytes", maxTerminalInputBytes)
	}
	running, err := m.lookup(id)
	if err != nil {
		return Task{}, err
	}
	running.mu.Lock()
	if running.status != "running" || running.file == nil {
		running.mu.Unlock()
		return Task{}, fmt.Errorf("terminal task %q is not running", id)
	}
	file := running.file
	running.mu.Unlock()
	running.inputMu.Lock()
	_, err = file.Write(input)
	running.inputMu.Unlock()
	if err != nil {
		return Task{}, fmt.Errorf("write terminal task %q: %w", id, err)
	}
	snapshot := running.snapshot()
	if developer && m.onDeveloperInput != nil {
		digest := sha256.Sum256(input)
		m.onDeveloperInput(snapshot, DeveloperInput{Bytes: len(input), SHA256: hex.EncodeToString(digest[:])})
	}
	return snapshot, nil
}

// Stop requests cancellation. It does not wait indefinitely; use List or Read
// to observe the final exit state.
func (m *Manager) Stop(id string) (Task, error) {
	running, err := m.lookup(id)
	if err != nil {
		return Task{}, err
	}
	running.mu.Lock()
	if running.status == "running" && running.cancel != nil {
		running.cancel()
	}
	running.mu.Unlock()
	return running.snapshot(), nil
}

// List returns all terminal task states in stable creation order.
func (m *Manager) List() []Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Task, 0, len(m.tasks))
	for index := uint64(1); index <= m.nextID; index++ {
		if running, ok := m.tasks[fmt.Sprintf("term-%03d", index)]; ok {
			result = append(result, running.snapshot())
		}
	}
	return result
}

// Close cancels all tasks and allows their cleanup goroutines a bounded grace
// period. It is safe to call more than once.
func (m *Manager) Close() {
	m.mu.Lock()
	tasks := make([]*task, 0, len(m.tasks))
	for _, running := range m.tasks {
		tasks = append(tasks, running)
	}
	m.mu.Unlock()
	for _, running := range tasks {
		_, _ = m.Stop(running.id)
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for _, running := range tasks {
		select {
		case <-running.done:
		case <-deadline.C:
			return
		}
	}
}

func (m *Manager) lookup(id string) (*task, error) {
	m.mu.Lock()
	running, ok := m.tasks[id]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("terminal task %q does not exist", id)
	}
	return running, nil
}

func (m *Manager) activeLocked() int {
	active := 0
	for _, running := range m.tasks {
		running.mu.Lock()
		if running.status == "running" {
			active++
		}
		running.mu.Unlock()
	}
	return active
}

func (t *task) copyOutput() {
	buffer := make([]byte, 4096)
	for {
		count, err := t.file.Read(buffer)
		if count > 0 {
			t.mu.Lock()
			t.output.append(buffer[:count])
			t.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (t *task) wait(taskContext context.Context) {
	err := t.command.Wait()
	_ = t.file.Close()
	t.cleanup()
	t.cancel()
	t.mu.Lock()
	t.ended = t.manager.now()
	t.status = "exited"
	if err != nil {
		var exitError *exec.ExitError
		switch {
		case errors.Is(taskContext.Err(), context.DeadlineExceeded):
			code := -1
			t.exitCode = &code
			t.err = "terminal task exceeded its lifetime limit"
		case errors.Is(taskContext.Err(), context.Canceled):
			code := -1
			t.exitCode = &code
			t.err = "terminal task cancelled"
		case errors.As(err, &exitError):
			code := exitError.ExitCode()
			t.exitCode = &code
		default:
			t.err = err.Error()
		}
	} else {
		code := 0
		t.exitCode = &code
	}
	snapshot := t.snapshotLocked()
	t.mu.Unlock()
	if t.manager.onExit != nil {
		t.manager.onExit(snapshot)
	}
	close(t.done)
}

func (t *task) snapshot() Task {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshotLocked()
}

func (t *task) snapshotLocked() Task {
	return Task{
		ID:              t.id,
		Argv:            append([]string(nil), t.argv...),
		StartedAt:       t.started,
		EndedAt:         t.ended,
		Status:          t.status,
		ExitCode:        cloneExitCode(t.exitCode),
		Error:           t.err,
		OutputTruncated: t.output.start > 0,
		Rows:            t.rows,
		Columns:         t.columns,
	}
}

func minTerminalDimension(value, maximum int) int {
	if value > maximum {
		return maximum
	}
	return value
}

func cloneExitCode(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

type terminalBuffer struct {
	limit int
	start int64
	end   int64
	data  []byte
}

func (b *terminalBuffer) append(value []byte) {
	if len(value) == 0 {
		return
	}
	value = []byte(strings.ToValidUTF8(string(value), "�"))
	b.end += int64(len(value))
	if len(value) >= b.limit {
		b.data = append(b.data[:0], value[len(value)-b.limit:]...)
		b.start = b.end - int64(len(b.data))
		return
	}
	overflow := len(b.data) + len(value) - b.limit
	if overflow > 0 {
		copy(b.data, b.data[overflow:])
		b.data = b.data[:len(b.data)-overflow]
		b.start += int64(overflow)
	}
	b.data = append(b.data, value...)
}

func (b *terminalBuffer) read(cursor int64, maximum int) ([]byte, int64, int64, bool) {
	dropped := cursor < b.start
	if cursor < b.start {
		cursor = b.start
	}
	if cursor > b.end {
		cursor = b.end
	}
	start := int(cursor - b.start)
	end := len(b.data)
	if end-start > maximum {
		end = start + maximum
	}
	result := append([]byte(nil), b.data[start:end]...)
	next := cursor + int64(len(result))
	return result, cursor, next, dropped
}
