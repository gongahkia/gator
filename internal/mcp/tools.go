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
	IncludeRaw  bool                 `json:"include_raw,omitempty"`
}

type compactEnvelope struct {
	SchemaVersion string                  `json:"schema_version"`
	TaskID        string                  `json:"task_id"`
	Instruction   string                  `json:"instruction,omitempty"`
	Cwd           string                  `json:"cwd,omitempty"`
	Stage         string                  `json:"stage"`
	Turn          int                     `json:"turn"`
	Digest        *envelope.ContextDigest `json:"digest,omitempty"`
	Budget        envelope.Budget         `json:"budget"`
	RawTotalBytes int                     `json:"raw_total_bytes,omitempty"`
	Provenance    []rawUnitProvenance     `json:"provenance,omitempty"`
}

type rawUnitProvenance struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Path      string `json:"path,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	Bytes     int    `json:"bytes"`
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
	env, includeRaw, rpcErr := envelopeArg(raw)
	if rpcErr != nil {
		return toolResult{}, rpcErr
	}
	out, err := s.runner.Compress(ctx, env)
	return compactEnvelopeToolResult(out, err, includeRaw), nil
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
	return compactEnvelopeToolResult(env, err, args.IncludeRaw), nil
}

func envelopeArg(raw json.RawMessage) (*envelope.Envelope, bool, *rpcError) {
	var args contextArgs
	if err := decodeStrict(raw, &args); err != nil {
		return nil, false, invalidParams("invalid compress arguments: %v", err)
	}
	if args.Envelope != nil {
		if args.Envelope.SchemaVersion != envelope.SchemaVersion {
			return nil, false, invalidParams("unsupported envelope schema version: %q", args.Envelope.SchemaVersion)
		}
		return args.Envelope, args.IncludeRaw, nil
	}
	if args.Raw == nil {
		return nil, false, invalidParams("compress requires envelope or raw")
	}
	if err := validateCwdInstruction(args.Cwd, args.Instruction); err != nil {
		return nil, false, invalidParams(err.Error())
	}
	env := envelope.NewEnvelope(taskID(args.Instruction, args.Cwd), args.Instruction, args.Cwd)
	env.Stage = "gather"
	env.Raw = args.Raw
	return env, args.IncludeRaw, nil
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

func compactEnvelopeToolResult(env *envelope.Envelope, err error, includeRaw bool) toolResult {
	if includeRaw {
		return envelopeToolResult(env, err)
	}
	if err != nil {
		return toolResult{Content: []toolContent{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	compact := compactEnvelopeFrom(env)
	data, err := json.Marshal(compact)
	if err != nil {
		return toolResult{Content: []toolContent{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	return toolResult{
		Content:           []toolContent{{Type: "text", Text: string(data)}},
		StructuredContent: compact,
	}
}

func compactEnvelopeFrom(env *envelope.Envelope) compactEnvelope {
	if env == nil {
		return compactEnvelope{}
	}
	compact := compactEnvelope{
		SchemaVersion: env.SchemaVersion,
		TaskID:        env.TaskID,
		Instruction:   env.Instruction,
		Cwd:           env.Cwd,
		Stage:         env.Stage,
		Turn:          env.Turn,
		Digest:        env.Digest,
		Budget:        env.Budget,
	}
	if env.Raw != nil {
		compact.RawTotalBytes = env.Raw.TotalBytes
		for _, unit := range env.Raw.Units {
			compact.Provenance = append(compact.Provenance, rawUnitProvenance{
				ID:        unit.ID,
				Kind:      unit.Kind,
				Path:      unit.Path,
				StartLine: unit.StartLine,
				EndLine:   unit.EndLine,
				Bytes:     len(unit.Text),
			})
		}
	}
	return compact
}

func decodeStrict(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
