package connector

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/action"
)

type Permission string

const (
	PermissionAllow Permission = "allow"
	PermissionAsk   Permission = "ask"
	PermissionDeny  Permission = "deny"
	PermissionDraft Permission = "draft"
)

// PermissionRule narrows one connector operation. Empty connector/operation values are wildcards.
type PermissionRule struct {
	ConnectorID string     `json:"connector_id,omitempty"`
	Operation   string     `json:"operation,omitempty"`
	Read        Permission `json:"read,omitempty"`
	Write       Permission `json:"write,omitempty"`
}

func (r PermissionRule) Validate() error {
	if r.ConnectorID != "" && !idPattern.MatchString(r.ConnectorID) {
		return errors.New("connector permission has an invalid connector ID")
	}
	if len(r.Operation) > 128 || strings.ContainsAny(r.Operation, "\x00\r\n") {
		return errors.New("connector permission has an invalid operation")
	}
	if r.Read != "" && r.Read != PermissionAllow && r.Read != PermissionAsk && r.Read != PermissionDeny {
		return fmt.Errorf("invalid connector read permission %q", r.Read)
	}
	if r.Write != "" && r.Write != PermissionAsk && r.Write != PermissionDeny && r.Write != PermissionDraft {
		return fmt.Errorf("invalid connector write permission %q", r.Write)
	}
	if r.Read == "" && r.Write == "" {
		return errors.New("connector permission rule is empty")
	}
	return nil
}

type PermissionSet []PermissionRule

// Resolve applies every matching rule monotonically. Rules can narrow authority but never restore it.
func (p PermissionSet) Resolve(connectorID, operation string, capability action.Capability) Permission {
	write := action.RequiresFreshApproval(capability)
	result := PermissionAllow
	if write {
		result = PermissionAsk
	}
	for _, rule := range p {
		if rule.ConnectorID != "" && rule.ConnectorID != connectorID || rule.Operation != "" && rule.Operation != operation {
			continue
		}
		candidate := rule.Read
		if write {
			candidate = rule.Write
		}
		if candidate == "" {
			continue
		}
		if permissionRank(candidate, write) < permissionRank(result, write) {
			result = candidate
		}
	}
	return result
}

func permissionRank(value Permission, write bool) int {
	if write {
		switch value {
		case PermissionAsk:
			return 2
		case PermissionDraft:
			return 1
		default:
			return 0
		}
	}
	switch value {
	case PermissionAllow:
		return 2
	case PermissionAsk:
		return 1
	default:
		return 0
	}
}
