package tmux

import (
	"context"
	"os/exec"
	"sync"
	"time"
)

type ExecRunner struct{}
type boundedWriter struct {
	mu       sync.Mutex
	bytes    []byte
	limit    int
	cancel   context.CancelFunc
	overflow bool
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	remaining := w.limit - len(w.bytes)
	if len(p) > remaining {
		w.bytes = append(w.bytes, p[:remaining]...)
		w.overflow = true
		w.cancel()
	} else {
		w.bytes = append(w.bytes, p...)
	}
	return n, nil
}
func (ExecRunner) Run(ctx context.Context, bin string, args []string, limit int) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out := &boundedWriter{limit: limit, cancel: cancel}
	diagnostic := &boundedWriter{limit: 4096, cancel: cancel}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "LANG=C", "TERM=dumb"}
	cmd.Stdout = out
	cmd.Stderr = diagnostic
	cmd.WaitDelay = 250 * time.Millisecond
	err := cmd.Run()
	result := Result{Bytes: out.bytes, Truncated: out.overflow}
	if out.overflow {
		return result, ErrLimit
	}
	if err != nil || diagnostic.overflow {
		return Result{}, ErrUnavailable
	}
	return result, nil
}
