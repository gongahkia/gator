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
	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/projectcapture"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/snapshot"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workspace"
)

const (
	CodeCapabilityLSP       = "lsp"
	CodeCapabilityMCP       = "mcp"
	CodeCapabilityExtension = "extension"
	CodeCapabilityHTTP      = "http"
	CodeCapabilityBrowser   = "browser"
	CodeCapabilityTerminal  = "terminal"
	CodeCapabilityHooks     = "hooks"
)

// CodePolicy is the user-owned capability envelope for Gator's internal Code
// specialist. The Work manager can decide whether to delegate, but it cannot
// widen this policy. Empty policies deliberately resolve to strict, offline
// execution with only the built-in patch tools and verifier commands.
type CodePolicy struct {
	MaxSteps               int
	Verification           [][]string
	Scopes                 []string
	Profile                string
	Setup                  [][]string
	AllowedCommands        [][]string
	AllowedCommandPrefixes [][]string
	Sandbox                sandbox.Policy
	Capabilities           []string
	BrowserSession         string
}

// HasCapability reports an explicit per-Work-run grant. Configuration and
// project trust remain necessary but are never sufficient by themselves.
func (p CodePolicy) HasCapability(name string) bool {
	for _, capability := range p.Capabilities {
		if capability == name {
			return true
		}
	}
	return false
}

// Request describes one bounded local work session.
type Request struct {
	RoleConfiguration    map[string]orchestrator.RoleConfiguration
	OnSupervisor         func(*orchestrator.Supervisor)
	OTLPEndpoint         string
	Limits               agent.Limits
	Budget               *agent.Budget
	WebOrigins           []string
	catalog              *evidenceCatalog
	researchTools        []agent.Tool
	DisableDelegation    bool
	integration          *codeIntegration
	Project              *projectcapture.Bundle
	Provider             string
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
	Images               []agent.Image
	Attachments          []agent.Attachment
	// RequireCode is used by the compatibility `gator code` route. It keeps the
	// main Work manager user-facing while requiring concrete Code-specialist
	// evidence before the run may complete.
	RequireCode        bool
	Code               CodePolicy
	ApproveCodeCommand func(context.Context, []string) (tools.CommandDecision, error)
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
	OnEvent     agent.EventSink
	Budget      *agent.Budget
	Baseline    []byte
	Project     *projectcapture.Bundle
	ID          string
	SourcePath  string
	ScratchPath string
	Task        string
	ParentRunID string
	MaxSteps    int
	Policy      CodePolicy
	Approve     func(context.Context, []string) (tools.CommandDecision, error)
}

// CodeResult is the bounded handoff from Gator Code back to Gator Work.
type CodeResult struct {
	BaselineSHA256 string
	Usage          agent.Usage
	Summary        string
	Patch          []byte
	ChangedPaths   []string
	Steps          int
}

// CodeDelegate adapts Gator's isolated coding workflow into a Work specialist.
type CodeDelegate func(context.Context, CodeRequest) (CodeResult, error)

// Executor combines a provider-independent model with private workspace
// allocation and artifact sealing.
type Executor struct {
	RoleConfiguration map[string]orchestrator.RoleConfiguration
	RoleSteps         map[string]int
	HTTP              tools.HTTPFetchOptions
	RoleModels        map[string]agent.Model
	Model             agent.Model
	Now               func() time.Time
	StateDir          string
	Connectors        connector.Runtime
	Code              CodeDelegate
}
