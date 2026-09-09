// Package workrun orchestrates provider-independent, non-Git work sessions.
// It keeps general artifact work separate from the coding workflow in
// internal/run while sharing the same agent loop and provider adapters.
package workrun

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/snapshot"
	"github.com/gongahkia/gator/internal/workspace"
)

// Request describes one bounded local work session.
type Request struct {
	SourcePath           string
	Objective            string
	RunID                string
	MaxSteps             int
	Mode                 action.Mode
	Contract             artifact.Contract
	StateDir             string
	System               string
	OnEvent              agent.EventSink
	Steering             <-chan string
	ConnectorIDs         []string
	ApproveAction        action.Approver
	ApproveConnectorRead func(context.Context, string, string, json.RawMessage) (bool, error)
	ConversationID       string
	ParentRevisionID     string
	SnapshotID           string
	RefreshSource        bool
	ConnectorPermissions connector.PermissionSet
	SnapshotOptions      snapshot.Options
	OnSnapshot           func(snapshot.Manifest)
}

// Outcome retains staged files and trusted evidence even when model execution
// fails. Source is never modified by the executor.
type Outcome struct {
	Work           workspace.Work
	Manifest       artifact.Manifest
	Result         agent.Result
	Events         []agent.Event
	ConversationID string
	RevisionID     string
	SnapshotID     string
	SourceSnapshot snapshot.Manifest
}

// CodeRequest asks the existing Gator Code engine to work against the exact
// frozen source selected by a Work run. The implementation must isolate all
// writes and return a reviewable patch rather than mutate SourcePath.
type CodeRequest struct {
	ID          string
	SourcePath  string
	ScratchPath string
	Task        string
	ParentRunID string
	MaxSteps    int
}

// CodeResult is the bounded handoff from Gator Code back to Gator Work.
type CodeResult struct {
	Summary      string
	Patch        []byte
	ChangedPaths []string
	Steps        int
}

// CodeDelegate adapts Gator's isolated coding workflow into a Work specialist.
type CodeDelegate func(context.Context, CodeRequest) (CodeResult, error)

// Executor combines a provider-independent model with private workspace
// allocation and artifact sealing.
type Executor struct {
	Model      agent.Model
	Now        func() time.Time
	StateDir   string
	Connectors connector.Runtime
	Code       CodeDelegate
}
