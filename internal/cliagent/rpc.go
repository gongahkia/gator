package cliagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
)

// rpcWire owns a bounded, full-duplex JSONL connection. Responses are correlated
// independently of notifications and server-initiated questions. A timeout
// closes the connection: callers must reconcile the native session rather than
// assume that an unacknowledged command did not execute.
type rpcWire struct {
	reader  io.ReadCloser
	writer  io.WriteCloser
	pi      bool
	mu      sync.Mutex
	next    uint64
	pending map[string]chan rpcReply
	failure error
	once    sync.Once
	done    chan struct{}
	events  chan json.RawMessage
	writes  chan rpcWrite
}
type rpcReply struct {
	data json.RawMessage
	err  error
}
type rpcWrite struct {
	data   []byte
	result chan error
}

func newRPCWire(reader io.ReadCloser, writer io.WriteCloser, pi bool) *rpcWire {
	w := &rpcWire{reader: reader, writer: writer, pi: pi, pending: map[string]chan rpcReply{}, done: make(chan struct{}), events: make(chan json.RawMessage, 128), writes: make(chan rpcWrite, 16)}
	go w.readLoop()
	go w.writeLoop()
	return w
}
func (w *rpcWire) stop(err error) {
	if err == nil {
		err = io.EOF
	}
	w.once.Do(func() {
		w.mu.Lock()
		w.failure = err
		w.mu.Unlock()
		close(w.done)
		_ = w.reader.Close()
		_ = w.writer.Close()
	})
}
func (w *rpcWire) err() error { w.mu.Lock(); defer w.mu.Unlock(); return w.failure }
func (w *rpcWire) Close()     { w.stop(errors.New("CLI session transport closed")) }
func (w *rpcWire) writeLoop() {
	for {
		select {
		case <-w.done:
			return
		case item := <-w.writes:
			_, err := io.Copy(w.writer, bytes.NewReader(item.data))
			item.result <- err
			if err != nil {
				w.stop(fmt.Errorf("write CLI protocol: %w", err))
				return
			}
		}
	}
}
func (w *rpcWire) write(ctx context.Context, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > 1024*1024 {
		return errors.New("CLI request exceeds 1 MiB")
	}
	item := rpcWrite{data: append(data, '\n'), result: make(chan error, 1)}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.done:
		return w.err()
	case w.writes <- item:
	}
	select {
	case err := <-item.result:
		return err
	case <-ctx.Done():
		w.stop(ctx.Err())
		return ctx.Err()
	case <-w.done:
		return w.err()
	}
}
func (w *rpcWire) Call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	w.mu.Lock()
	if len(w.pending) >= 16 {
		w.mu.Unlock()
		return nil, errors.New("too many pending CLI requests")
	}
	w.next++
	id := "gator-" + strconv.FormatUint(w.next, 10)
	reply := make(chan rpcReply, 1)
	w.pending[id] = reply
	w.mu.Unlock()
	defer func() { w.mu.Lock(); delete(w.pending, id); w.mu.Unlock() }()
	request := map[string]any{"id": id, "method": method, "params": params}
	if w.pi {
		request = map[string]any{"id": id, "type": method}
		for key, value := range params {
			if key == "id" || key == "type" {
				return nil, errors.New("reserved RPC parameter")
			}
			request[key] = value
		}
	}
	if err := w.write(ctx, request); err != nil {
		return nil, err
	}
	select {
	case result := <-reply:
		return result.data, result.err
	case <-ctx.Done():
		w.stop(ctx.Err())
		return nil, ctx.Err()
	case <-w.done:
		// A final response may have arrived immediately before EOF.
		select {
		case result := <-reply:
			return result.data, result.err
		default:
			return nil, w.err()
		}
	}
}
func (w *rpcWire) Notify(ctx context.Context, method string, params any) error {
	return w.write(ctx, map[string]any{"method": method, "params": params})
}
func (w *rpcWire) Reply(ctx context.Context, id json.RawMessage, result any) error {
	if len(id) == 0 || !json.Valid(id) {
		return errors.New("invalid CLI request ID")
	}
	return w.write(ctx, map[string]any{"id": id, "result": result})
}
func (w *rpcWire) readLoop() {
	defer close(w.events)
	scan := bufio.NewScanner(w.reader)
	scan.Buffer(make([]byte, 4096), 1024*1024)
	total := 0
	for scan.Scan() {
		line := bytes.TrimSpace(scan.Bytes())
		if len(line) == 0 {
			continue
		}
		total += len(line)
		if total > 64*1024*1024 {
			w.stop(errors.New("CLI connection output exceeds 64 MiB"))
			return
		}
		var record struct {
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Type    string          `json:"type"`
			Success bool            `json:"success"`
			Result  json.RawMessage `json:"result"`
			Data    json.RawMessage `json:"data"`
			Error   json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			w.stop(errors.New("CLI emitted malformed RPC JSON"))
			return
		}
		response := len(record.ID) > 0 && record.Method == "" && ((!w.pi && (record.Result != nil || record.Error != nil)) || (w.pi && record.Type == "response"))
		if response {
			var id string
			if err := json.Unmarshal(record.ID, &id); err != nil {
				w.stop(errors.New("invalid CLI response ID"))
				return
			}
			w.mu.Lock()
			reply := w.pending[id]
			w.mu.Unlock()
			if reply == nil {
				continue
			} // Late or duplicate responses never resolve another request.
			result := rpcReply{data: record.Result}
			if w.pi {
				result.data = record.Data
			}
			if (record.Error != nil && string(record.Error) != "null") || (w.pi && !record.Success) {
				result.err = fmt.Errorf("CLI rejected request: %s", boundedRPCError(record.Error))
			}
			select {
			case reply <- result:
			default:
			}
			continue
		}
		value := append(json.RawMessage(nil), line...)
		select {
		case w.events <- value:
		case <-w.done:
			return
		default:
			w.stop(errors.New("CLI event queue overflow; session requires reconciliation"))
			return
		}
	}
	if err := scan.Err(); err != nil {
		w.stop(fmt.Errorf("read CLI protocol: %w", err))
	} else {
		w.stop(io.EOF)
	}
}
func boundedRPCError(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "unspecified failure"
	}
	if len(raw) > 2048 {
		return "error detail exceeds limit"
	}
	return string(raw)
}
