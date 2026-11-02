package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

type wireMessage struct {
	raw []byte
	err error
}
type stdioClient struct {
	command       *exec.Cmd
	input, output *os.File
	cleanup       func()
	kill          func()
	gate          chan struct{}
	done, exited  chan struct{}
	messages      chan wireMessage
	closeOnce     sync.Once
	closeErr      error
	mu            sync.Mutex
	version       string
	next          int64
}

func startStdio(ctx context.Context, command *exec.Cmd, cleanup func()) (client, error) {
	input, childInput, err := os.Pipe()
	if err != nil {
		cleanup()
		return nil, err
	}
	// os.Pipe returns (reader,writer): the parent writes to child stdin.
	childOutput, output, err := os.Pipe()
	if err != nil {
		input.Close()
		childInput.Close()
		cleanup()
		return nil, err
	}
	command.Stdin = input
	command.Stdout = output
	c := &stdioClient{command: command, input: childInput, output: childOutput, cleanup: cleanup, gate: make(chan struct{}, 1), done: make(chan struct{}), exited: make(chan struct{}), messages: make(chan wireMessage, 8), version: modernVersion}
	c.kill = ownMCPProcess(command)
	if err := command.Start(); err != nil {
		input.Close()
		childInput.Close()
		childOutput.Close()
		output.Close()
		cleanup()
		return nil, err
	}
	input.Close()
	output.Close()
	go func() { _ = command.Wait(); close(c.exited) }()
	go c.readMessages()
	go func() {
		select {
		case <-ctx.Done():
			_ = c.Close()
		case <-c.done:
		}
	}()
	return c, nil
}
func (c *stdioClient) readMessages() {
	defer close(c.messages)
	scanner := bufio.NewScanner(c.output)
	scanner.Buffer(make([]byte, 4096), maxWire+1)
	for scanner.Scan() {
		raw := append([]byte(nil), scanner.Bytes()...)
		select {
		case c.messages <- wireMessage{raw: raw}:
		case <-c.done:
			return
		}
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	} else {
		err = errors.New("MCP stdio read failed or message exceeded 128 KiB")
	}
	select {
	case c.messages <- wireMessage{err: err}:
	case <-c.done:
	}
}
func (c *stdioClient) setProtocol(version string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.version = version
}
func (c *stdioClient) write(ctx context.Context, raw []byte) error {
	complete := make(chan error, 1)
	go func() { _, err := c.input.Write(append(raw, '\n')); complete <- err }()
	select {
	case err := <-complete:
		return err
	case <-c.done:
		return errors.New("MCP client is closed")
	case <-ctx.Done():
		_ = c.Close()
		return ctx.Err()
	}
}
func (c *stdioClient) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	if err := acquire(ctx, c.gate, c.done); err != nil {
		return nil, err
	}
	defer func() { <-c.gate }()
	c.mu.Lock()
	version := c.version
	c.mu.Unlock()
	c.next++
	id := c.next
	raw, err := marshalRPC(method, params, &id, version)
	if err != nil {
		return nil, err
	}
	if err := c.write(ctx, raw); err != nil {
		return nil, err
	}
	consumed := 0
	for count := 0; count < 256; count++ {
		select {
		case <-ctx.Done():
			// Read-only discovery can time out on a legacy server. Keep the same
			// stream for its handshake; late probe replies are ignored by exact ID.
			notification, _ := marshalRPC("notifications/cancelled", map[string]any{"requestId": id}, nil, version)
			bounded, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
			_ = c.write(bounded, notification)
			stop()
			if method != "server/discover" {
				_ = c.Close()
			}
			return nil, ctx.Err()
		case <-c.done:
			return nil, errors.New("MCP client closed; dispatch outcome may be unknown")
		case message, ok := <-c.messages:
			if !ok {
				return nil, io.EOF
			}
			if message.err != nil {
				return nil, message.err
			}
			consumed += len(message.raw)
			if consumed > maxCatalog {
				return nil, errors.New("MCP stdio response exceeds 1 MiB")
			}
			result, err := parseRPCResponse(message.raw, id)
			if errors.Is(err, errOtherResponse) {
				continue
			}
			if err == nil {
				err = normalizeResult(result, version)
			}
			return result, err
		}
	}
	return nil, errors.New("MCP stdio response exceeds 256 messages")
}
func (c *stdioClient) Notify(ctx context.Context, method string, params any) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	if err := acquire(ctx, c.gate, c.done); err != nil {
		return err
	}
	defer func() { <-c.gate }()
	raw, err := marshalRPC(method, params, nil, protocolVersion)
	if err != nil {
		return err
	}
	return c.write(ctx, raw)
}
func (c *stdioClient) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.input.Close()
		_ = c.output.Close()
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-c.exited:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
			c.kill()
		}
		select {
		case <-c.exited:
		case <-time.After(250 * time.Millisecond):
			c.closeErr = errors.New("MCP subprocess did not exit after forced shutdown")
			go func() { <-c.exited; c.cleanup() }()
			return
		}
		c.cleanup()
	})
	return c.closeErr
}
