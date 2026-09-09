// Package action defines the authority model for general Gator work.
//
// Capability is deliberately separate from tool names. A connector or tool
// cannot avoid a high-risk approval merely by choosing a different name for
// the same class of side effect.
package action

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Capability describes the kind of authority an operation consumes.
type Capability string

const (
	SourceRead      Capability = "source_read"
	ArtifactWrite   Capability = "artifact_write"
	ProcessExecute  Capability = "process_execute"
	NetworkRead     Capability = "network_read"
	ConnectedRead   Capability = "connected_read"
	ConnectedMutate Capability = "connected_mutate"
	Publish         Capability = "publish"
)

var capabilities = map[Capability]struct{}{
	SourceRead: {}, ArtifactWrite: {}, ProcessExecute: {}, NetworkRead: {},
	ConnectedRead: {}, ConnectedMutate: {}, Publish: {},
}

// Validate rejects unknown capabilities at configuration and connector
// boundaries before a tool can be exposed to a model.
func (c Capability) Validate() error {
	if _, known := capabilities[c]; !known {
		return fmt.Errorf("unknown work capability %q", c)
	}
	return nil
}

// Mode is a monotonic ceiling on authority for one work run.
type Mode string

const (
	Inspect Mode = "inspect"
	Draft   Mode = "draft"
	Act     Mode = "act"
)

// Validate rejects unknown modes before a run creates any resources.
func (m Mode) Validate() error {
	switch m {
	case Inspect, Draft, Act:
		return nil
	default:
		return fmt.Errorf("unknown work mode %q", m)
	}
}

// Allows reports whether a mode can expose a capability. Operations may still
// require a narrower per-operation approval.
func (m Mode) Allows(capability Capability) bool {
	if _, known := capabilities[capability]; !known {
		return false
	}
	switch m {
	case Inspect:
		return capability == SourceRead || capability == NetworkRead || capability == ConnectedRead
	case Draft:
		return capability != ConnectedMutate && capability != Publish
	case Act:
		return true
	default:
		return false
	}
}

// Meet returns the least-authoritative of two valid modes. It is used to
// combine a user request with an administrative or profile ceiling.
func Meet(left, right Mode) (Mode, error) {
	if err := left.Validate(); err != nil {
		return "", err
	}
	if err := right.Validate(); err != nil {
		return "", err
	}
	if rank(left) <= rank(right) {
		return left, nil
	}
	return right, nil
}

func rank(mode Mode) int {
	switch mode {
	case Inspect:
		return 0
	case Draft:
		return 1
	case Act:
		return 2
	default:
		return -1
	}
}

// RequiresFreshApproval identifies operations whose consequences cross a
// connected-service or human communication boundary. Their approval must not
// be remembered from another operation.
func RequiresFreshApproval(capability Capability) bool {
	return capability == ConnectedMutate || capability == Publish
}

// Disposition is the outcome contract's policy for external side effects.
type Disposition string

const (
	Forbid  Disposition = "forbid"
	Propose Disposition = "draft"
	Approve Disposition = "approve"
)

// Validate rejects an unknown external-action policy.
func (d Disposition) Validate() error {
	switch d {
	case Forbid, Propose, Approve:
		return nil
	default:
		return fmt.Errorf("unknown external-action disposition %q", d)
	}
}

const (
	maxIdentifierBytes = 128
	maxTargetBytes     = 2 * 1024
	maxPreviewBytes    = 16 * 1024
)

// Proposal is a bounded, human-reviewable request to cross an external action
// boundary. PayloadSHA256 binds an approval to the exact hidden payload while
// Preview explains its effect without requiring credentials or binary data.
type Proposal struct {
	ID            string     `json:"id"`
	Capability    Capability `json:"capability"`
	ConnectorID   string     `json:"connector_id"`
	Operation     string     `json:"operation"`
	Target        string     `json:"target"`
	Preview       string     `json:"preview"`
	PayloadSHA256 string     `json:"payload_sha256"`
}

// NewProposal creates a proposal bound to payload. The payload itself remains
// in connector-local memory and must not be copied into a manifest.
func NewProposal(id string, capability Capability, connectorID, operation, target, preview string, payload []byte) (Proposal, error) {
	digest := sha256.Sum256(payload)
	proposal := Proposal{
		ID:            strings.TrimSpace(id),
		Capability:    capability,
		ConnectorID:   strings.TrimSpace(connectorID),
		Operation:     strings.TrimSpace(operation),
		Target:        strings.TrimSpace(target),
		Preview:       strings.TrimSpace(preview),
		PayloadSHA256: hex.EncodeToString(digest[:]),
	}
	if err := proposal.Validate(); err != nil {
		return Proposal{}, err
	}
	return proposal, nil
}

// Validate checks the stable proposal envelope without inspecting a secret or
// potentially large payload.
func (p Proposal) Validate() error {
	if p.Capability != ConnectedMutate && p.Capability != Publish {
		return errors.New("external action proposal must mutate a connected service or publish")
	}
	for name, value := range map[string]string{
		"proposal id": p.ID, "connector id": p.ConnectorID, "operation": p.Operation,
	} {
		if strings.TrimSpace(value) == "" || len(value) > maxIdentifierBytes || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("%s is missing or invalid", name)
		}
	}
	if strings.TrimSpace(p.Target) == "" || len(p.Target) > maxTargetBytes || strings.ContainsRune(p.Target, 0) {
		return errors.New("proposal target is missing or invalid")
	}
	if strings.TrimSpace(p.Preview) == "" || len(p.Preview) > maxPreviewBytes || strings.ContainsRune(p.Preview, 0) {
		return errors.New("proposal preview is missing or invalid")
	}
	if len(p.PayloadSHA256) != sha256.Size*2 {
		return errors.New("proposal payload digest is invalid")
	}
	if _, err := hex.DecodeString(p.PayloadSHA256); err != nil {
		return errors.New("proposal payload digest is invalid")
	}
	return nil
}

// Status is the durable disposition of a proposed action.
type Status string

const (
	Pending  Status = "pending"
	Executed Status = "executed"
	Denied   Status = "denied"
	Failed   Status = "failed"
)

// Record is safe to persist in a work manifest. It contains the bounded
// proposal envelope and outcome, never the action payload or a credential.
type Record struct {
	Proposal Proposal `json:"proposal"`
	Status   Status   `json:"status"`
	Error    string   `json:"error,omitempty"`
}

// Validate checks a durable action result without requiring access to the
// connector payload that produced it.
func (r Record) Validate() error {
	if err := r.Proposal.Validate(); err != nil {
		return err
	}
	switch r.Status {
	case Pending, Executed, Denied:
		if r.Error != "" {
			return fmt.Errorf("action status %q must not include an error", r.Status)
		}
	case Failed:
		if strings.TrimSpace(r.Error) == "" || len(r.Error) > 512 || strings.ContainsRune(r.Error, 0) {
			return errors.New("failed action requires a bounded error")
		}
	default:
		return fmt.Errorf("unknown action status %q", r.Status)
	}
	return nil
}
