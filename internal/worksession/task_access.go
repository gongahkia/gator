package worksession

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

// TaskAccessNeed is a request for setup, never an execution grant. IDs and
// operations are revalidated against current canonical connection controls.
type TaskAccessNeed struct {
	ConnectorID string `json:"connector_id,omitempty"`
	Kind        string `json:"kind"`
	Operation   string `json:"operation"`
}

func ValidateTaskAccessNeeds(needs []TaskAccessNeed) error {
	if len(needs) > 16 {
		return errors.New("too many task access requirements")
	}
	seen := map[string]bool{}
	for _, need := range needs {
		key := need.ConnectorID + ":" + need.Kind + ":" + need.Operation
		if seen[key] || need.ConnectorID != "" && !planSourceID(need.ConnectorID) || !slices.Contains([]string{"http_json", "http_webhook", "google_workspace", "slack", "atlassian", "notion", "remote_mcp"}, need.Kind) || need.Operation == "" || len(need.Operation) > 128 || strings.ContainsAny(need.Operation, "\x00\r\n") {
			return errors.New("invalid task access requirement")
		}
		seen[key] = true
	}
	return nil
}

// PlanAccessChange records an explicitly reviewed source/configuration change
// while preserving earlier options and the original processing destination.
type TaskAccessSelection struct {
	Need        TaskAccessNeed `json:"need"`
	ConnectorID string         `json:"connector_id"`
}

type PlanReviewSnapshot struct {
	Revision uint64        `json:"revision"`
	Digest   string        `json:"digest"`
	Content  *PlanContent  `json:"content"`
	Approval *PlanApproval `json:"approval,omitempty"`
}

type PlanAccessChange struct {
	PriorPlan        *PlanReviewSnapshot   `json:"prior_plan,omitempty"`
	Requirements     []TaskAccessSelection `json:"requirements,omitempty"`
	ConnectorIDs     []string              `json:"connector_ids"`
	BeforeScope      string                `json:"before_scope"`
	AfterScope       string                `json:"after_scope"`
	ProcessingDigest string                `json:"processing_digest"`
	ReviewID         string                `json:"review_id"`
	OAuthPins        map[string]string     `json:"oauth_pins,omitempty"`
	CredentialPins   map[string]string     `json:"credential_pins,omitempty"`
}

func (a *PlanAccessChange) validate() error {
	if a == nil {
		return nil
	}
	if !planDigest(a.BeforeScope) || !planDigest(a.AfterScope) || !planDigest(a.ProcessingDigest) || !planDigest(a.ReviewID) || a.ConnectorIDs == nil || len(a.ConnectorIDs) > 16 {
		return errors.New("invalid reviewed task access")
	}
	seen := map[string]bool{}
	for _, id := range a.ConnectorIDs {
		if seen[id] || !planSourceID(id) {
			return errors.New("invalid reviewed connection selection")
		}
		seen[id] = true
	}
	if len(a.Requirements) > 16 {
		return errors.New("too many reviewed task access selections")
	}
	needs := []TaskAccessNeed{}
	for _, selection := range a.Requirements {
		if !seen[selection.ConnectorID] {
			return errors.New("task requirement references an unselected account")
		}
		needs = append(needs, selection.Need)
	}
	if err := ValidateTaskAccessNeeds(needs); err != nil {
		return err
	}
	for _, pins := range []map[string]string{a.OAuthPins, a.CredentialPins} {
		for id, pin := range pins {
			if !seen[id] || !planDigest(pin) {
				return errors.New("invalid reviewed account binding")
			}
		}
	}
	return nil
}
func updatePlanConnections(raw json.RawMessage, ids []string) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, errors.New("invalid original connection options")
	}
	key := "ConnectorIDs"
	for name := range fields {
		if strings.EqualFold(name, key) {
			key = name
			break
		}
	}
	fields[key], _ = json.Marshal(ids)
	return json.Marshal(fields)
}
func (p *TaskPlan) LatestAccessChange() *PlanAccessChange {
	if p == nil {
		return nil
	}
	for i := len(p.InputChanges) - 1; i >= 0; i-- {
		if a := p.InputChanges[i].Access; a != nil {
			return a
		}
	}
	return nil
}

// SelectionFor is keyed by the retained requirement, not a title or array row.
func (a *PlanAccessChange) SelectionFor(need TaskAccessNeed) (string, bool) {
	if a == nil {
		return "", false
	}
	for _, selection := range a.Requirements {
		if selection.Need == need {
			return selection.ConnectorID, true
		}
	}
	return "", false
}

func (a *PlanAccessChange) validatePriorPlan(before json.RawMessage) error {
	if a == nil || a.PriorPlan == nil {
		return nil
	}
	prior := a.PriorPlan
	var options struct{ ConnectorIDs []string }
	if json.Unmarshal(before, &options) != nil || prior.Revision == 0 || !planDigest(prior.Digest) || prior.Content == nil || !validPlanContent(*prior.Content, options.ConnectorIDs) {
		return errors.New("invalid retained account-review proposal")
	}
	if prior.Approval != nil && (prior.Approval.Revision != prior.Revision || prior.Approval.Digest != prior.Digest || prior.Approval.At.IsZero()) {
		return errors.New("invalid historical plan approval")
	}
	return nil
}
