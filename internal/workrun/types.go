// Package workrun orchestrates provider-independent, non-Git work sessions.
// It keeps general artifact work separate from the coding workflow in
// internal/run while sharing the same agent loop and provider adapters.
package workrun

import (
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/workspace"
)

// Request describes one bounded local work session.
type Request struct {
	SourcePath       string
	Objective        string
	RunID            string
	MaxSteps         int
	Mode             action.Mode
	Contract         artifact.Contract
	StateDir         string
	System           string
	OnEvent          agent.EventSink
	Steering         <-chan string
	ConnectorIDs     []string
	ApproveAction    action.Approver
	ConversationID   string
	ParentRevisionID string
	SnapshotID       string
	RefreshSource    bool
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
}

// Executor combines a provider-independent model with private workspace
// allocation and artifact sealing.
type Executor struct {
	Model      agent.Model
	Now        func() time.Time
	StateDir   string
	Connectors connector.Runtime
}
