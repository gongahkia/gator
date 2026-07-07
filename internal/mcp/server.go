package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
)

const protocolVersion = "2025-11-25"

const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

type Server struct {
	runner Runner
}

func NewServer(runner Runner) *Server {
	return &Server{runner: runner}
}

func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Bytes()
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			if err := enc.Encode(errorResponse(nil, codeParseError, "parse error", err.Error())); err != nil {
				return err
			}
			continue
		}
		resp, ok := s.handle(ctx, req)
		if ok {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func (s *Server) handle(ctx context.Context, req rpcRequest) (rpcResponse, bool) {
	if req.JSONRPC != "2.0" || req.Method == "" {
		return errorResponse(req.ID, codeInvalidRequest, "invalid request", nil), true
	}
	if !req.hasID() && isNotification(req.Method) {
		return rpcResponse{}, false
	}
	if !req.hasID() {
		return rpcResponse{}, false
	}
	switch req.Method {
	case "initialize":
		return resultResponse(req.ID, initializeResult()), true
	case "ping":
		return resultResponse(req.ID, map[string]any{}), true
	case "tools/list":
		return resultResponse(req.ID, map[string]any{"tools": toolDefinitions()}), true
	case "tools/call":
		result, rpcErr := s.callTool(ctx, req.Params)
		if rpcErr != nil {
			return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: rpcErr}, true
		}
		return resultResponse(req.ID, result), true
	default:
		return errorResponse(req.ID, codeMethodNotFound, "method not found", req.Method), true
	}
}

func initializeResult() map[string]any {
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":        "paw",
			"title":       "Paw Context Tools",
			"version":     "dev",
			"description": "Context-only MCP server for gather/compress/digest.",
		},
		"instructions": "Exposes context-only tools. Does not plan, edit, patch, or verify.",
	}
}

func isNotification(method string) bool {
	return method == "notifications/initialized" || method == "notifications/cancelled"
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r rpcRequest) hasID() bool {
	return len(r.ID) > 0
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func resultResponse(id json.RawMessage, result any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id json.RawMessage, code int, message string, data any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message, Data: data}}
}

func invalidParams(format string, args ...any) *rpcError {
	return &rpcError{Code: codeInvalidParams, Message: fmt.Sprintf(format, args...)}
}
