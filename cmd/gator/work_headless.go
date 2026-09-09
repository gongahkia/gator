package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/workrun"
	"io"
)

type workFrame struct {
	Version       int        `json:"version"`
	Type          string     `json:"type"`
	Request       *workInput `json:"request,omitempty"`
	Text          string     `json:"text,omitempty"`
	InteractionID int        `json:"interaction_id,omitempty"`
	Approved      bool       `json:"approved,omitempty"`
}
type workInput struct {
	Source           string             `json:"source"`
	Objective        string             `json:"objective"`
	Provider         string             `json:"provider,omitempty"`
	Model            string             `json:"model,omitempty"`
	ConversationID   string             `json:"conversation_id,omitempty"`
	ParentRevisionID string             `json:"parent_revision_id,omitempty"`
	SnapshotID       string             `json:"snapshot_id,omitempty"`
	Refresh          bool               `json:"refresh_source,omitempty"`
	Mode             action.Mode        `json:"mode"`
	Contract         artifact.Contract  `json:"contract"`
	Code             workrun.CodePolicy `json:"code"`
	MaxSteps         int                `json:"max_steps"`
	Connectors       []string           `json:"connectors,omitempty"`
}

func workHeadless(ctx context.Context, in io.Reader, out io.Writer) error {
	stateDir, err := journal.ResolveStateDir("")
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	if !scanner.Scan() {
		return errors.New("Work protocol requires a start frame")
	}
	var first workFrame
	if err := json.Unmarshal(scanner.Bytes(), &first); err != nil {
		return err
	}
	if first.Version != 1 || first.Type != "start" || first.Request == nil {
		return errors.New("expected Work v1 start frame")
	}
	input := first.Request
	request := workrun.Request{SourcePath: input.Source, Objective: input.Objective, ConversationID: input.ConversationID, ParentRevisionID: input.ParentRevisionID, SnapshotID: input.SnapshotID, RefreshSource: input.Refresh, Mode: input.Mode, Contract: input.Contract, Code: input.Code, MaxSteps: input.MaxSteps, ConnectorIDs: input.Connectors}
	service, err := configuredWorkService(input.Provider, input.Model, stateDir, &request)
	if err != nil {
		return err
	}
	return serveWorkOperation(ctx, service, request, scanner, out)
}
func serveWorkOperation(ctx context.Context, service workrun.Service, request workrun.Request, scanner *bufio.Scanner, out io.Writer) error {
	operation := service.Start(ctx, request)
	defer operation.Cancel()
	frames := make(chan workFrame, 16)
	inputErrors := make(chan error, 1)
	go func() {
		defer close(frames)
		for scanner.Scan() {
			var frame workFrame
			if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
				inputErrors <- err
				return
			}
			select {
			case frames <- frame:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			inputErrors <- err
		}
	}()
	encoder := json.NewEncoder(out)
	send := func(kind string, payload any) error {
		return encoder.Encode(struct {
			Version int    `json:"version"`
			Type    string `json:"type"`
			Data    any    `json:"data"`
		}{1, kind, payload})
	}
	events, interactions := operation.Events, operation.Interactions
	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				frames = nil
				continue
			}
			var err error
			if frame.Version != 1 {
				err = errors.New("unsupported Work protocol version")
			} else {
				switch frame.Type {
				case "cancel":
					operation.Cancel()
				case "steer":
					err = operation.Steer(frame.Text)
				case "approval":
					err = operation.Respond(frame.InteractionID, frame.Approved)
				default:
					err = fmt.Errorf("unknown Work control %q", frame.Type)
				}
			}
			if err != nil {
				if err := send("control_error", err.Error()); err != nil {
					return err
				}
			}
		case err := <-inputErrors:
			operation.Cancel()
			if err := send("protocol_error", err.Error()); err != nil {
				return err
			}
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if err := send("event", event); err != nil {
				return err
			}
		case interaction, ok := <-interactions:
			if !ok {
				interactions = nil
				continue
			}
			if err := send("approval", interaction); err != nil {
				return err
			}
		case completed := <-operation.Done:
			for events != nil {
				event, ok := <-events
				if !ok {
					break
				}
				if err := send("event", event); err != nil {
					return err
				}
			}
			result := map[string]any{"conversation_id": completed.Outcome.ConversationID, "revision_id": completed.Outcome.RevisionID, "snapshot_id": completed.Outcome.SnapshotID, "manifest_path": completed.Outcome.Work.ManifestPath, "status": completed.Outcome.Manifest.Status, "final_text": completed.Outcome.Result.FinalText}
			if completed.Err != nil {
				result["error"] = completed.Err.Error()
			}
			return send("result", result)
		}
	}
}
