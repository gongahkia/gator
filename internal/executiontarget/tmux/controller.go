// Copyright 2026 gongahkia
// Licensed under the Apache License, Version 2.0. See third_party/onibi/LICENSE.
// Adapted from Onibi internal/tmux/attach.go; modified for exact pane identity,
// no-start/hook-aware reads, bounded output and non-sensitive errors.
package tmux

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("tmux identity or safe read unavailable")
var ErrLimit = errors.New("tmux output limit reached")
var panePattern = regexp.MustCompile(`^%[0-9]{1,10}$`)
var sessionPattern = regexp.MustCompile(`^\$[0-9]{1,10}$`)

const MaxLines = 200
const MaxBytes = 16 * 1024
const processBytes = 64 * 1024
const nativeFormat = "#{pid}|#{start_time}|#{session_id}|#{session_created}|#{window_id}|#{pane_id}|#{pane_pid}|#{pane_dead}|#{version}|#{pane_height}|#{history_size}"

type Config struct {
	Review *Review `json:"review,omitempty"`
	// A trusted local administrator must review canonical command aliases before
	// any connection. tmux expands aliases before a guard can inspect options.
	ReadConfigReviewed bool   `json:"read_config_reviewed"`
	Executable         string `json:"executable"`
	Socket             string `json:"socket"`
}
type Result struct {
	Bytes     []byte
	Truncated bool
}
type Runner interface {
	Run(context.Context, string, []string, int) (Result, error)
}

// Probe supplies OS-backed evidence. Unknown/unsupported evidence fails closed.
type Probe interface {
	Socket(string) (string, error)
	Process(int) (string, error)
}
type Controller struct {
	ReviewCurrent func() bool // optional host catalog revocation check, before each invocation
	Config        Config
	Runner        Runner
	Probe         Probe
}
type Native struct {
	ServerPID      int    `json:"server_pid"`
	ServerTime     string `json:"server_time"`
	ServerStart    string `json:"server_start"`
	SocketEvidence string `json:"socket_evidence"`
	Session        string `json:"session"`
	SessionCreated string `json:"session_created"`
	Window         string `json:"window"`
	Pane           string `json:"pane"`
	PanePID        int    `json:"pane_pid"`
	PaneStart      string `json:"pane_start"`
	Version        string `json:"version"`
	Height         int    `json:"height"`
	History        int    `json:"history"`
}

func (n Native) Incarnation() string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d/%s/%s/%s/%s/%s/%s/%d/%s", n.ServerPID, n.ServerTime, n.ServerStart, n.SocketEvidence, n.Session, n.SessionCreated, n.Pane, n.PanePID, n.PaneStart)))
	return hex.EncodeToString(h[:])
}

type Capture struct {
	Text       string
	Truncated  bool
	CapturedAt time.Time
}

func New(config Config) (*Controller, error) {
	if !config.ReadConfigReviewed || !config.Review.Valid() || !filepath.IsAbs(config.Executable) || !filepath.IsAbs(config.Socket) || strings.ContainsAny(config.Executable+config.Socket, "\x00\r\n") {
		return nil, ErrUnavailable
	}
	digest, err := ExecutableFingerprint(config.Executable)
	c := &Controller{Config: config, Runner: ExecRunner{}, Probe: OSProbe{}}
	if err != nil || digest != config.Review.ExecutableDigest || !c.reviewed() {
		return nil, ErrUnavailable
	}
	return c, nil
}
func (c *Controller) run(ctx context.Context, args ...string) (Result, error) {
	if !c.reviewed() {
		return Result{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	v, e := c.Runner.Run(ctx, c.Config.Executable, []string{"-V"}, 256)
	if e != nil || v.Truncated || !regexp.MustCompile(`^tmux 3\.[4-8][a-z]?\s*$`).Match(v.Bytes) {
		return Result{}, ErrUnavailable
	}
	if !c.reviewed() {
		return Result{}, ErrUnavailable
	}
	// No environment-based socket/config selection. -N refuses server startup;
	// -f /dev/null prevents configuration loading even on a misbehaving old client.
	args = append([]string{"-N", "-f", os.DevNull, "-S", c.Config.Socket}, args...)
	r, err := c.Runner.Run(ctx, c.Config.Executable, args, processBytes)
	if !c.reviewed() {
		return Result{}, ErrUnavailable
	}
	if err != nil {
		if errors.Is(err, ErrLimit) {
			return r, ErrLimit
		}
		return Result{}, ErrUnavailable
	}
	return r, nil
}

// -F is mandatory: if-shell evaluates ONLY a fixed tmux format, never an OS
// shell. Its CANFAIL target and absent after-hook avoid errors for missing panes.
// Reject nonempty read/error hooks before running the nested read command.
// Only validated native numeric IDs and bounded integers enter command text.
func (c *Controller) guarded(ctx context.Context, pane, command string) (Result, error) {
	condition := "#{&&:#{==:#{command-error},},#{&&:#{==:#{after-list-panes},},#{&&:#{==:#{after-display-message},},#{==:#{after-capture-pane},}}}}"
	args := []string{"if-shell", "-F"}
	if pane != "" {
		if !panePattern.MatchString(pane) {
			return Result{}, ErrUnavailable
		}
		args = append(args, "-t", pane)
		condition = "#{&&:#{==:#{pane_id}," + pane + "}," + condition + "}"
	}
	args = append(args, condition, command)
	return c.run(ctx, args...)
}
func (c *Controller) parse(line string, socket string) (Native, error) {
	p := strings.Split(strings.TrimSpace(line), "|")
	if len(p) != 11 || !sessionPattern.MatchString(p[2]) || !regexp.MustCompile(`^@[0-9]{1,10}$`).MatchString(p[4]) || !panePattern.MatchString(p[5]) || p[7] != "0" || !regexp.MustCompile(`^3\.[4-8][a-z]?$`).MatchString(p[8]) {
		return Native{}, ErrUnavailable
	}
	pid, e := strconv.Atoi(p[0])
	ppid, e2 := strconv.Atoi(p[6])
	start, e3 := strconv.ParseInt(p[1], 10, 64)
	created, e4 := strconv.ParseInt(p[3], 10, 64)
	if e != nil || e2 != nil || e3 != nil || e4 != nil || pid <= 0 || ppid <= 0 || start <= 0 || created <= 0 {
		return Native{}, ErrUnavailable
	}
	server, err := c.Probe.Process(pid)
	if err != nil {
		return Native{}, ErrUnavailable
	}
	process, err := c.Probe.Process(ppid)
	if err != nil {
		return Native{}, ErrUnavailable
	}
	history, he := strconv.Atoi(p[10])
	height, e := strconv.Atoi(p[9])
	if he != nil || history < 0 || history > 100000000 || e != nil || height < 1 || height > 100000 {
		return Native{}, ErrUnavailable
	}
	return Native{ServerPID: pid, ServerTime: p[1], ServerStart: server, SocketEvidence: socket, Session: p[2], SessionCreated: p[3], Window: p[4], Pane: p[5], PanePID: ppid, PaneStart: process, Version: p[8], Height: height, History: history}, nil
}
func (c *Controller) Candidates(ctx context.Context) ([]Native, error) {
	socket, err := c.Probe.Socket(c.Config.Socket)
	if err != nil {
		return nil, ErrUnavailable
	}
	r, err := c.guarded(ctx, "", "list-panes -a -F '"+nativeFormat+"'")
	if err != nil || r.Truncated {
		return nil, ErrUnavailable
	}
	lines := strings.Split(strings.TrimSpace(string(r.Bytes)), "\n")
	if len(lines) > 256 {
		return nil, ErrLimit
	}
	out := []Native{}
	for _, line := range lines {
		if line == "" {
			continue
		}
		n, err := c.parse(line, socket)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if after, e := c.Probe.Socket(c.Config.Socket); e != nil || after != socket {
		return nil, ErrUnavailable
	}
	return out, nil
}
func (c *Controller) Resolve(ctx context.Context, pane string) (Native, error) {
	if !panePattern.MatchString(pane) {
		return Native{}, ErrUnavailable
	}
	socket, err := c.Probe.Socket(c.Config.Socket)
	if err != nil {
		return Native{}, ErrUnavailable
	}
	r, err := c.guarded(ctx, pane, "display-message -p -t "+pane+" '"+nativeFormat+"'")
	if err != nil || r.Truncated {
		return Native{}, ErrUnavailable
	}
	n, err := c.parse(string(r.Bytes), socket)
	if err != nil || n.Pane != pane {
		return Native{}, ErrUnavailable
	}
	if after, e := c.Probe.Socket(c.Config.Socket); e != nil || after != socket {
		return Native{}, ErrUnavailable
	}
	return n, nil
}
func (c *Controller) Capture(ctx context.Context, expected Native, lines int) (Capture, error) {
	if lines < 1 || lines > MaxLines {
		return Capture{}, ErrLimit
	}
	before, err := c.Resolve(ctx, expected.Pane)
	if err != nil || before.Incarnation() != expected.Incarnation() {
		return Capture{}, ErrUnavailable
	}
	// Print a fixed guard-success marker even for a blank pane. capture -p never
	// writes tmux paste buffers; no style/hyperlink/control output is requested.
	r, err := c.guarded(ctx, expected.Pane, "display-message -p -t "+expected.Pane+" GATOR_CAPTURE_OK ; capture-pane -p -t "+expected.Pane+" -S "+strconv.Itoa(before.Height-MaxLines)+" -E "+strconv.Itoa(before.Height-1))
	if err != nil && !errors.Is(err, ErrLimit) {
		return Capture{}, ErrUnavailable
	}
	if !strings.HasPrefix(string(r.Bytes), "GATOR_CAPTURE_OK\n") {
		return Capture{}, ErrUnavailable
	}
	after, e := c.Resolve(ctx, expected.Pane)
	if e != nil || after.Incarnation() != before.Incarnation() {
		return Capture{}, ErrUnavailable
	}
	text, truncated := PlainTail(r.Bytes[len("GATOR_CAPTURE_OK\n"):], lines, MaxBytes)
	return Capture{Text: text, Truncated: r.Truncated || truncated || before.Height+before.History > MaxLines, CapturedAt: time.Now().UTC()}, nil
}

// ExecutableFingerprint is private enrollment configuration, not phone input.
func ExecutableFingerprint(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", ErrUnavailable
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 || info.Size() > 32*1024*1024 {
		return "", ErrUnavailable
	}
	h := sha256.New()
	if _, err = io.Copy(h, io.LimitReader(f, 32*1024*1024+1)); err != nil {
		return "", ErrUnavailable
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
