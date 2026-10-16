// measure_cli records process cost for one CLI invocation without retaining its output.
// Run with: go run ./scripts/measure_cli --label gator-help -- gator --help
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

type measurement struct {
	Label            string    `json:"label"`
	Executable       string    `json:"executable"`
	OperatingSystem  string    `json:"operating_system"`
	Architecture     string    `json:"architecture"`
	MeasuredAt       time.Time `json:"measured_at"`
	WallMilliseconds int64     `json:"wall_milliseconds"`
	FirstOutputMS    *int64    `json:"first_output_milliseconds,omitempty"`
	PeakRSSBytes     int64     `json:"peak_rss_bytes"`
	ExitCode         int       `json:"exit_code"`
	StoppedAfterIdle bool      `json:"stopped_after_idle,omitempty"`
}

type outputClock struct {
	start time.Time
	first atomic.Int64
}

func (clock *outputClock) Write(p []byte) (int, error) {
	if len(p) > 0 {
		clock.first.CompareAndSwap(0, time.Since(clock.start).Nanoseconds()+1)
	}
	return len(p), nil
}

func main() {
	label := flag.String("label", "", "stable comparison label")
	idle := flag.Duration("idle", 0, "stop a long-running process after this duration")
	timeout := flag.Duration("timeout", 60*time.Second, "maximum measurement duration")
	flag.Parse()
	arguments := flag.Args()
	if len(arguments) == 0 || *timeout <= 0 || *idle < 0 || (*idle > 0 && *idle >= *timeout) {
		fmt.Fprintln(os.Stderr, "usage: measure_cli --label NAME [--idle 5s] [--timeout 60s] -- COMMAND [ARGS...]")
		os.Exit(2)
	}
	result, err := measure(arguments, *label, *idle, *timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func measure(arguments []string, label string, idle, timeout time.Duration) (measurement, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return measurement{}, errors.New("process RSS measurement supports macOS and Linux")
	}
	command := exec.Command(arguments[0], arguments[1:]...)
	start := time.Now()
	clock := &outputClock{start: start}
	command.Stdout, command.Stderr = clock, clock
	if err := command.Start(); err != nil {
		return measurement{}, fmt.Errorf("start measured process: %w", err)
	}
	var stopped atomic.Bool
	stop := func() {
		stopped.Store(true)
		_ = command.Process.Signal(syscall.SIGTERM)
	}
	var idleTimer *time.Timer
	if idle > 0 {
		idleTimer = time.AfterFunc(idle, stop)
	}
	timeoutTimer := time.AfterFunc(timeout, func() {
		_ = command.Process.Kill()
	})
	waitErr := command.Wait()
	if idleTimer != nil {
		idleTimer.Stop()
	}
	timeoutTimer.Stop()
	if time.Since(start) >= timeout && !stopped.Load() {
		return measurement{}, fmt.Errorf("measured process exceeded %s timeout", timeout)
	}
	result := measurement{
		Label: strings.TrimSpace(label), Executable: arguments[0],
		OperatingSystem: runtime.GOOS, Architecture: runtime.GOARCH,
		MeasuredAt: start.UTC(), WallMilliseconds: time.Since(start).Milliseconds(),
		ExitCode: command.ProcessState.ExitCode(), StoppedAfterIdle: stopped.Load(),
	}
	if first := clock.first.Load(); first > 0 {
		milliseconds := (first - 1) / int64(time.Millisecond)
		result.FirstOutputMS = &milliseconds
	}
	if usage, ok := command.ProcessState.SysUsage().(*syscall.Rusage); ok {
		result.PeakRSSBytes = usage.Maxrss
		if runtime.GOOS == "linux" {
			result.PeakRSSBytes *= 1024
		}
	}
	if waitErr != nil && !result.StoppedAfterIdle {
		return result, fmt.Errorf("measured process exited %d: %w", result.ExitCode, waitErr)
	}
	return result, nil
}

var _ io.Writer = (*outputClock)(nil)
