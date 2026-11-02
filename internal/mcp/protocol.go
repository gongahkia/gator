package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	maxPages     = 32
	maxTools     = 256
	maxCatalog   = 1024 * 1024
	maxCursor    = 4096
	maxWire      = 128 * 1024
	callTimeout  = 2 * time.Minute
	probeTimeout = 2 * time.Second
)

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

// ValidToolName validates native tool names without treating them as server IDs.
func ValidToolName(name string) bool { return toolNamePattern.MatchString(name) }

// Protocol selection happens only during read-only setup, never by repeating a
// tool call. Unknown versions and unsupported interactions fail closed.
type protocolClient interface {
	client
	setProtocol(string)
	Notify(context.Context, string, any) error
}

func initialize(ctx context.Context, c client) error {
	pc, ok := c.(protocolClient)
	if !ok {
		return errors.New("MCP transport cannot negotiate protocol versions")
	}
	pc.setProtocol(modernVersion)
	probe, cancel := context.WithTimeout(ctx, probeTimeout)
	raw, err := c.Call(probe, "server/discover", map[string]any{})
	cancel()
	if err == nil {
		var result struct {
			Supported    []string                   `json:"supportedVersions"`
			Capabilities map[string]json.RawMessage `json:"capabilities"`
		}
		if json.Unmarshal(raw, &result) != nil || !slices.Contains(result.Supported, modernVersion) || !objectJSON(result.Capabilities["tools"]) {
			return errors.New("MCP discovery has no supported protocol/tool capability")
		}
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var rpc *rpcError
	if errors.As(err, &rpc) && rpc.Code == -32022 {
		var versions struct {
			Supported []string `json:"supported"`
		}
		if json.Unmarshal(rpc.Data, &versions) != nil || !slices.Contains(versions.Supported, protocolVersion) {
			return errors.New("MCP server advertises no mutually supported protocol version")
		}
	} else {
		var status *httpStatusError
		_, stdio := c.(*stdioClient)
		legacy := errors.As(err, &rpc) && (rpc.Code != -32020 && rpc.Code != -32021 && rpc.Code != -32022)
		legacy = legacy || errors.As(err, &status) && (status.Code == 400 || status.Code == 404 || status.Code == 405 || status.Code == 415)
		legacy = legacy || stdio && errors.Is(err, context.DeadlineExceeded)
		if !legacy {
			return err
		}
	}
	pc.setProtocol(protocolVersion)
	raw, err = c.Call(ctx, "initialize", map[string]any{"protocolVersion": protocolVersion, "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "gator", "version": "dev"}})
	if err != nil {
		return err
	}
	var result struct {
		Version      string                     `json:"protocolVersion"`
		Capabilities map[string]json.RawMessage `json:"capabilities"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Version != protocolVersion || !objectJSON(result.Capabilities["tools"]) {
		return errors.New("MCP initialization has no supported protocol/tool capability")
	}
	return pc.Notify(ctx, "notifications/initialized", map[string]any{})
}
func listTools(ctx context.Context, c client) ([]toolDescription, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	if err := initialize(ctx, c); err != nil {
		return nil, err
	}
	var tools []toolDescription
	seen, cursors := map[string]bool{}, map[string]bool{}
	cursor, total := "", 0
	for page := 0; page < maxPages; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := c.Call(ctx, "tools/list", params)
		if err != nil {
			return nil, err
		}
		total += len(raw)
		if total > maxCatalog {
			return nil, errors.New("MCP tool catalog exceeds 1 MiB")
		}
		var result struct {
			Tools []toolDescription `json:"tools"`
			Next  string            `json:"nextCursor"`
		}
		if json.Unmarshal(raw, &result) != nil || result.Tools == nil {
			return nil, errors.New("MCP server returned an invalid tool catalog")
		}
		for _, tool := range result.Tools {
			identity := toolAlias("", tool.Name)
			if !toolNamePattern.MatchString(tool.Name) || !objectJSON(tool.InputSchema) || seen[identity] {
				return nil, errors.New("MCP server returned an invalid or colliding tool definition")
			}
			seen[identity] = true
			tools = append(tools, tool)
			if len(tools) > maxTools {
				return nil, errors.New("MCP tool catalog exceeds 256 tools")
			}
		}
		if result.Next == "" {
			if http, ok := c.(*httpClient); ok {
				if err := http.rememberTools(tools); err != nil {
					return nil, err
				}
			}
			return tools, nil
		}
		if len(result.Next) > maxCursor || cursors[result.Next] {
			return nil, errors.New("MCP tool pagination cursor is oversized or repeated")
		}
		cursors[result.Next] = true
		cursor = result.Next
	}
	return nil, errors.New("MCP tool catalog exceeds 32 pages; no partial catalog was enabled")
}
func objectJSON(raw json.RawMessage) bool {
	var value map[string]json.RawMessage
	return json.Unmarshal(raw, &value) == nil && value != nil
}
func validateToolResult(raw json.RawMessage) error {
	if len(raw) > maxToolOutput {
		return errors.New("MCP tool response exceeds 64 KiB")
	}
	var result struct {
		IsError bool `json:"isError"`
	}
	if !objectJSON(raw) || json.Unmarshal(raw, &result) != nil {
		return errors.New("MCP tool returned an invalid result")
	}
	if result.IsError {
		return errors.New("MCP tool reported an execution error; do not automatically retry")
	}
	return nil
}
func normalizeResult(raw json.RawMessage, version string) error {
	var result struct {
		Type string `json:"resultType"`
	}
	if !objectJSON(raw) || json.Unmarshal(raw, &result) != nil {
		return errors.New("MCP result is not an object")
	}
	if result.Type == "input_required" {
		return errors.New("MCP requires unsupported additional interaction; outcome may be partial; no automatic retry")
	}
	if result.Type != "complete" && (version == modernVersion || result.Type != "") {
		return fmt.Errorf("MCP result type is unsupported for protocol %s", version)
	}
	return nil
}

// Preserve existing short names; long names use a deterministic digest suffix.
// Catalog admission rejects aliases that would otherwise identify two tools.
func toolAlias(server, name string) string {
	alias := "mcp_" + strings.NewReplacer("-", "_", ".", "_").Replace(server+"_"+name)
	if len(alias) > 64 {
		hash := sha256.Sum256([]byte(server + "\x00" + name))
		alias = fmt.Sprintf("%s_%x", alias[:47], hash[:8])
	}
	return alias
}
