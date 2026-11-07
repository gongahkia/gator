package workrun

import (
	"encoding/json"
	"runtime"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/productguide"
)

// Keep this guide aligned with desktop/src/SettingsNavigation.tsx,
// desktop/src/DesktopHelp.tsx, and desktop/src/WorkDetails.tsx. It is bundled
// application knowledge, so app help never depends on a user's workspace
// containing Gator's source or on an external documentation service.
const applicationGuide = terminalApplicationGuide

const applicationStateHeader = "Current Gator run state (host-reported, read-only):\n"

// This is an allowlist, not a serialization of Request or the user's settings.
// Credentials, endpoints, private profile paths, and attachment contents must
// never be added to app self-knowledge as a side effect of answering app help.
type applicationState struct {
	LocalTime          string             `json:"host_local_time,omitempty"`
	Platform           string             `json:"platform"`
	ProviderModel      string             `json:"provider_model"`
	Mode               action.Mode        `json:"effective_mode"`
	ExternalActions    action.Disposition `json:"external_actions"`
	SelectedConnectors []string           `json:"selected_connectors"`
	SourceAccess       string             `json:"source_access"`
	SnapshotID         string             `json:"snapshot_id,omitempty"`
	GoalID             string             `json:"goal_id,omitempty"`
	RecallStatus       string             `json:"saved_recall_status,omitempty"`
	SuppliedSavedItems int                `json:"supplied_saved_items"`
	RequestedAt        string             `json:"requested_at,omitempty"`
}

func applicationContext(request Request) string {
	provider := strings.TrimSpace(request.Provider)
	if provider == "" {
		provider = "not reported"
	}
	state := applicationState{
		Platform: runtime.GOOS, ProviderModel: provider,
		Mode: request.Mode, ExternalActions: request.Contract.ExternalActions,
		SelectedConnectors: append([]string{}, request.ConnectorIDs...),
		SourceAccess:       "read-only snapshot of this conversation's workspace; use file tools to inspect its contents",
		SnapshotID:         request.SnapshotID, GoalID: request.GoalID,
		RecallStatus: request.RecallStatus, SuppliedSavedItems: len(request.ContextRefs),
	}
	if !request.RequestedAt.IsZero() {
		state.RequestedAt = request.RequestedAt.Format(time.RFC3339)
		state.LocalTime = request.RequestedAt.In(time.Local).Format(time.RFC3339)
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	guide := productguide.Current()
	return applicationGuide + "\n\n" + guide.ModelConnections + "\n\n" + guide.ModelAvailability + "\n\n" + applicationStateHeader + string(data)
}
