package workrun

import (
	"errors"
	"slices"
	"strings"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/workhistory"
)

// resolveDecision projects the effective host policy into a display-safe route.
// Model inference may explain a route, but the validated Request remains the
// only source of authority and selected connected services.
func resolveDecision(request Request) (*workhistory.Decision, error) {
	if len(request.ContextRefs) > 32 || len(request.ContextItems) > 32 || len(request.ExcludedKnowledgeIDs) > 128 {
		return nil, errors.New("too many saved context references")
	}
	for _, item := range request.ContextItems {
		if len(item.ID) > 128 || len(item.Excerpt) > 2048 || len(item.Source) > 2048 || len(item.Title) > 512 {
			return nil, errors.New("saved context excerpt exceeds its bounds")
		}
	}
	for _, id := range request.ExcludedKnowledgeIDs {
		if len(id) > 128 || (!strings.HasPrefix(id, "mem-") && !strings.HasPrefix(id, "note-")) {
			return nil, errors.New("invalid excluded knowledge ID")
		}
	}
	origin := strings.TrimSpace(request.DecisionOrigin)
	if origin == "" {
		origin = "explicit"
	}
	if !slices.Contains([]string{"explicit", "auto", "model", "job"}, origin) {
		return nil, errors.New("invalid Work decision origin")
	}
	reason := strings.TrimSpace(request.DecisionReason)
	if len(reason) > 512 || strings.ContainsRune(reason, 0) {
		return nil, errors.New("invalid Work decision reason")
	}
	decision := &workhistory.Decision{Version: 1, Origin: origin, GoalID: request.GoalID, Mode: request.Mode,
		Connectors: append([]string(nil), request.ConnectorIDs...), ContextRefs: append([]string(nil), request.ContextRefs...), RecallStatus: request.RecallStatus, Reason: reason}
	decision.ToolGroups = []string{"source", "evidence"}
	decision.ContextItems = append([]workhistory.ContextItem(nil), request.ContextItems...)
	if request.Mode == action.Inspect {
		decision.Intent = "inspect"
	} else {
		decision.Intent = "deliverable"
		decision.ToolGroups = append(decision.ToolGroups, "artifact")
	}
	for _, requirement := range request.Contract.Artifacts {
		decision.Artifacts = append(decision.Artifacts, requirement.Path)
	}
	if request.GoalID != "" {
		decision.ToolGroups = append(decision.ToolGroups, "goal")
	}
	if len(request.ConnectorIDs) > 0 {
		decision.ToolGroups = append(decision.ToolGroups, "connected")
	}
	if request.codeEnabled() {
		decision.ToolGroups = append(decision.ToolGroups, "code")
	}
	if request.Mode == action.Act && request.Contract.ExternalActions == action.Approve {
		decision.ToolGroups = append(decision.ToolGroups, "approved_action")
	}
	if request.Decision != nil {
		proposed := request.Decision
		if proposed.Mode != decision.Mode || proposed.GoalID != decision.GoalID || !slices.Equal(proposed.Artifacts, decision.Artifacts) || !slices.Equal(proposed.Connectors, decision.Connectors) {
			return nil, errors.New("inferred Work decision conflicts with effective authority")
		}
	}
	return decision, nil
}
