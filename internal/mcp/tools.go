package mcp

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/gongahkia/paw/internal/envelope"
)

type Tool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content           []toolContent `json:"content"`
	StructuredContent any           `json:"structuredContent,omitempty"`
	IsError           bool          `json:"isError,omitempty"`
}

type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type contextArgs struct {
	Cwd         string               `json:"cwd,omitempty"`
	Instruction string               `json:"instruction,omitempty"`
	Envelope    *envelope.Envelope   `json:"envelope,omitempty"`
	Raw         *envelope.RawContext `json:"raw,omitempty"`
}

func (s *Server) callTool(ctx context.Context, raw json.RawMessage) (toolResult, *rpcError) {
	var params callToolParams
	if err := decodeStrict(raw, &params); err != nil {
		return toolResult{}, invalidParams("invalid tools/call params: %v", err)
	}
	if len(params.Arguments) == 0 {
		params.Arguments = json.RawMessage(`{}`)
	}
	switch params.Name {
	case "gather":
		return s.callGather(ctx, params.Arguments)
	case "compress":
		return s.callCompress(ctx, params.Arguments)
	case "digest":
		return s.callDigest(ctx, params.Arguments)
	default:
		return toolResult{}, invalidParams("unknown tool: %s", params.Name)
	}
}

func (s *Server) callGather(ctx context.Context, raw json.RawMessage) (toolResult, *rpcError) {
	var args contextArgs
	if err := decodeStrict(raw, &args); err != nil {
		return toolResult{}, invalidParams("invalid gather arguments: %v", err)
	}
	if err := validateCwdInstruction(args.Cwd, args.Instruction); err != nil {
		return toolResult{}, invalidParams(err.Error())
	}
	env, err := s.runner.Gather(ctx, args.Cwd, args.Instruction)
	return envelopeToolResult(env, err), nil
}

func (s *Server) callCompress(ctx context.Context, raw json.RawMessage) (toolResult, *rpcError) {
	env, rpcErr := envelopeArg(raw)
	if rpcErr != nil {
		return toolResult{}, rpcErr
	}
	out, err := s.runner.Compress(ctx, env)
	return envelopeToolResult(out, err), nil
}

func (s *Server) callDigest(ctx context.Context, raw json.RawMessage) (toolResult, *rpcError) {
	var args contextArgs
	if err := decodeStrict(raw, &args); err != nil {
		return toolResult{}, invalidParams("invalid digest arguments: %v", err)
	}
	if err := validateCwdInstruction(args.Cwd, args.Instruction); err != nil {
		return toolResult{}, invalidParams(err.Error())
	}
	env, err := s.runner.Digest(ctx, args.Cwd, args.Instruction)
	return envelopeToolResult(env, err), nil
}

func envelopeArg(raw json.RawMessage) (*envelope.Envelope, *rpcError) {
	var args contextArgs
	if err := decodeStrict(raw, &args); err != nil {
		return nil, invalidParams("invalid compress arguments: %v", err)
	}
	if args.Envelope != nil {
		if args.Envelope.SchemaVersion != envelope.SchemaVersion {
			return nil, invalidParams("unsupported envelope schema version: %q", args.Envelope.SchemaVersion)
		}
		return args.Envelope, nil
	}
	if args.Raw == nil {
		return nil, invalidParams("compress requires envelope or raw")
	}
	if err := validateCwdInstruction(args.Cwd, args.Instruction); err != nil {
		return nil, invalidParams(err.Error())
	}
	env := envelope.NewEnvelope(taskID(args.Instruction, args.Cwd), args.Instruction, args.Cwd)
	env.Stage = "gather"
	env.Raw = args.Raw
	return env, nil
}

func envelopeToolResult(env *envelope.Envelope, err error) toolResult {
	if err != nil {
		return toolResult{Content: []toolContent{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	data, err := json.Marshal(env)
	if err != nil {
		return toolResult{Content: []toolContent{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	return toolResult{
		Content:           []toolContent{{Type: "text", Text: string(data)}},
		StructuredContent: env,
	}
}

func decodeStrict(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
