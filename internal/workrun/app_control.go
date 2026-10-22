package workrun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/gongahkia/gator/internal/agent"
)

// AppController is the foreground application's narrow settings boundary.
// Implementations expose display-safe projections, never arbitrary commands,
// files, credentials, or configuration documents.
type AppController interface {
	Inspect(context.Context, AppQuery) (any, error)
	Prepare(context.Context, AppChange) (AppMutation, error)
}

type AppQuery struct {
	Topic string `json:"topic"`
	ID    string `json:"id,omitempty"`
}

type AppChange struct {
	Setting string          `json:"setting"`
	Value   json.RawMessage `json:"value"`
}

type AppMutation struct {
	Preview    any
	Review     bool
	AfterReply bool
	AfterRun   bool
	// Apply revalidates any settings or targets captured by Prepare.
	Apply func(context.Context) (any, error)
}

type appTool struct {
	controller AppController
	request    Request
	write      bool
}

func applicationTools(controller AppController, request Request) []agent.Tool {
	if controller == nil || !request.ForegroundApp {
		return nil
	}
	return []agent.Tool{appTool{controller: controller, request: request}, appTool{controller: controller, request: request, write: true}}
}

func (t appTool) Definition() agent.ToolDefinition {
	if !t.write {
		return agent.ToolDefinition{Name: "gator_app_inspect", Description: "Read Gator's actual settings, available models, usage, data/retention, recovery, connections, outputs, shortcuts or voice preferences. Use before answering current-state questions or preparing changes. No secrets are returned. Optional id selects a retained output run.", Parameters: json.RawMessage(`{"type":"object","properties":{"topic":{"type":"string","enum":["settings","models","usage","data","recovery","connections","outputs","shortcuts","voice"]},"id":{"type":"string","maxLength":128}},"required":["topic"],"additionalProperties":false}`)}
	}
	return agent.ToolDefinition{Name: "gator_app_update", Description: "Change a Gator-owned setting only when the user asks. Inspect first; use exact existing provider/model or connection IDs. setting/value: model {provider,model}; theme {theme}; usage_limits {daily_requests,daily_tokens,concurrent_requests}; usage_pause {paused}; retention {history_days,knowledge_days,snapshot_days}; connections {ids:[...]}; shortcuts {action,shortcut}; voice {spoken_replies,provider,model,language}; recovery {action}; data_action {action,path}; output_action {action,id,path}. Higher-impact changes pause for the user's review. Changes affect subsequent requests, not this run's permissions. Never use to modify credentials or to grant unrequested capabilities.", Parameters: json.RawMessage(`{"type":"object","properties":{"setting":{"type":"string","enum":["model","theme","usage_limits","usage_pause","retention","connections","shortcuts","voice","recovery","data_action","output_action"]},"value":{"type":"object"}},"required":["setting","value"],"additionalProperties":false}`)}
}

// DecodeAppArguments also protects adapters from unknown keys and trailing
// values. Tool arguments are data and are never interpreted by a shell.
func DecodeAppArguments(data json.RawMessage, target any) error {
	if len(data) == 0 || len(data) > 16*1024 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("app request must be a JSON object of at most 16 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("unexpected trailing app request data")
	}
	return nil
}

func (t appTool) Execute(ctx context.Context, args json.RawMessage) (agent.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return agent.ToolResult{}, err
	}
	var result any
	var err error
	if !t.write {
		var query AppQuery
		if err = DecodeAppArguments(args, &query); err != nil {
			return agent.ToolResult{}, err
		}
		result, err = t.controller.Inspect(ctx, query)
	} else {
		var change AppChange
		if err = DecodeAppArguments(args, &change); err != nil {
			return agent.ToolResult{}, err
		}
		prepared, prepareErr := t.controller.Prepare(ctx, change)
		if prepareErr != nil {
			return agent.ToolResult{}, prepareErr
		}
		if prepared.Apply == nil {
			return agent.ToolResult{}, errors.New("app change has no validated operation")
		}
		if prepared.Review {
			if t.request.ApproveAppChange == nil {
				return agent.ToolResult{}, errors.New("this change requires review in the foreground Gator app")
			}
			approved, approvalErr := t.request.ApproveAppChange(ctx, prepared.Preview)
			if approvalErr != nil {
				return agent.ToolResult{}, approvalErr
			}
			if !approved {
				return appResult(map[string]any{"status": "declined", "setting": change.Setting})
			}
		}
		if err = ctx.Err(); err != nil {
			return agent.ToolResult{}, err
		}
		if prepared.AfterReply {
			if t.request.queueAppChange == nil {
				return agent.ToolResult{}, errors.New("this change requires a foreground reply lifecycle")
			}
			t.request.queueAppChange(change, prepared)
			return appResult(map[string]any{"status": "scheduled_after_reply", "setting": change.Setting, "preview": prepared.Preview, "note": "The host will apply this only after this response completes successfully. Do not claim it is already saved."})
		}
		if prepared.AfterRun {
			if t.request.AfterAppRun == nil {
				return agent.ToolResult{}, errors.New("this change needs the foreground app to finish its run first")
			}
			t.request.AfterAppRun(change, prepared)
			return appResult(map[string]any{"status": "scheduled_after_reply", "setting": change.Setting, "preview": prepared.Preview, "note": "The host will apply this after the current run releases its profile lease. Do not claim it is already saved."})
		}
		result, err = prepared.Apply(ctx)
		if err == nil {
			if t.request.OnAppChange != nil {
				t.request.OnAppChange(change)
			}
			result = map[string]any{"status": "applied", "setting": change.Setting, "result": result}
		}
	}
	if err != nil {
		return agent.ToolResult{}, err
	}
	return appResult(result)
}

func appResult(value any) (agent.ToolResult, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if len(data) > 128*1024 {
		return agent.ToolResult{}, errors.New("app result is too large; request a specific item")
	}
	return agent.ToolResult{Content: string(data)}, nil
}
