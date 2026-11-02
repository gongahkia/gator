package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"
)

var errOtherResponse = errors.New("other MCP response or notification")

type rpcError struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

// Server-authored error text can contain credentials/output. Keep it out of
// ordinary diagnostics; structured codes/data are private negotiation input.
func (e *rpcError) Error() string { return fmt.Sprintf("MCP RPC error %d", e.Code) }
func parseRPCResponse(value []byte, id int64) (json.RawMessage, error) {
	if len(value) > maxWire || !utf8.Valid(value) {
		return nil, errors.New("MCP message is oversized or invalid UTF-8")
	}
	var r struct {
		Version string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Result  json.RawMessage `json:"result"`
		Error   *rpcError       `json:"error"`
	}
	if json.Unmarshal(value, &r) != nil || r.Version != "2.0" {
		return nil, errors.New("MCP message is not a valid JSON-RPC 2.0 object")
	}
	if r.Method != "" {
		if len(r.ID) != 0 {
			return nil, errors.New("MCP server-initiated requests are unsupported")
		}
		return nil, errOtherResponse
	}
	if string(r.ID) != strconv.FormatInt(id, 10) {
		return nil, errOtherResponse
	}
	if r.Error != nil {
		if len(r.Result) != 0 {
			return nil, errors.New("MCP response contains both result and error")
		}
		return nil, r.Error
	}
	if !objectJSON(r.Result) {
		return nil, errors.New("MCP RPC response has no object result")
	}
	return r.Result, nil
}
func marshalRPC(method string, params any, id *int64, version string) ([]byte, error) {
	encoded, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(encoded, &fields) != nil || fields == nil {
		return nil, errors.New("MCP parameters must be an object")
	}
	if version == modernVersion && id != nil {
		fields["_meta"], _ = json.Marshal(map[string]any{"io.modelcontextprotocol/protocolVersion": version, "io.modelcontextprotocol/clientCapabilities": map[string]any{}, "io.modelcontextprotocol/clientInfo": map[string]string{"name": "gator", "version": "dev"}})
	}
	message := map[string]any{"jsonrpc": "2.0", "method": method, "params": fields}
	if id != nil {
		message["id"] = *id
	}
	raw, err := json.Marshal(message)
	if len(raw) > maxWire {
		return nil, errors.New("MCP request exceeds 128 KiB")
	}
	return raw, err
}

// Read events incrementally; never wait for EOF after the matching response.
// Both bytes consumed and ignored notifications are bounded independently.
func readSSEResponse(ctx context.Context, reader io.Reader, id int64) (json.RawMessage, error) {
	limited := &io.LimitedReader{R: reader, N: maxCatalog + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), maxWire+1)
	var data []byte
	events := 0
	dispatch := func() (json.RawMessage, error) {
		if len(data) == 0 {
			return nil, errOtherResponse
		}
		events++
		if events > 256 {
			return nil, errors.New("MCP stream exceeds 256 events")
		}
		raw, err := parseRPCResponse(data, id)
		data = nil
		return raw, err
	}
	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if limited.N <= 0 {
			return nil, errors.New("MCP stream exceeds 1 MiB")
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			raw, err := dispatch()
			if !errors.Is(err, errOtherResponse) {
				return raw, err
			}
			continue
		}
		if line[0] == ':' {
			continue
		}
		field, value, found := bytes.Cut(line, []byte(":"))
		if !found {
			value = nil
		}
		value = bytes.TrimPrefix(value, []byte(" "))
		if string(field) == "data" {
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, value...)
			if len(data) > maxWire {
				return nil, errors.New("MCP event exceeds 128 KiB")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("MCP stream read failed or line exceeded its bound")
	}
	if limited.N <= 0 {
		return nil, errors.New("MCP stream exceeds 1 MiB")
	}
	if len(data) > 0 {
		raw, err := dispatch()
		if !errors.Is(err, errOtherResponse) {
			return raw, err
		}
	}
	return nil, errors.New("MCP stream ended without the matching response; delivery may be unknown")
}
func readJSONResponse(reader io.Reader, id int64) (json.RawMessage, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxWire+1))
	if err != nil {
		return nil, err
	}
	return parseRPCResponse(raw, id)
}
func acquire(ctx context.Context, gate chan struct{}, done <-chan struct{}) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return errors.New("MCP client is closed")
	case gate <- struct{}{}:
	}
	if ctx.Err() != nil {
		<-gate
		return ctx.Err()
	}
	select {
	case <-done:
		<-gate
		return errors.New("MCP client is closed")
	default:
	}
	return nil
}
