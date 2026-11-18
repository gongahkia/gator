package workapp

import (
	"regexp"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/modelcatalog"
)

type nativeProviderFinished struct {
	Result modelcatalog.NativeProviderResult
}

var nativeProviderRequestID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

func (m Model) ApplyNativeProvider(a modelcatalog.NativeProviderAction) (tea.Model, tea.Cmd) {
	if !m.config.ManagedSessions || !nativeProviderRequestID.MatchString(a.RequestID) {
		return m, nil
	}
	if m.nativeProviderResult != nil && m.nativeProviderResult.RequestID == a.RequestID {
		return m, nil
	}
	if m.nativeProviderPending != "" {
		m.nativeProviderResult = &modelcatalog.NativeProviderResult{RequestID: a.RequestID, Action: a.Action, Error: "Another provider operation is pending. Wait, refresh and explicitly review again."}
		return m, nil
	}
	switch a.Action {
	case "status", "review", "review_remove", "save", "remove", "cancel":
	default:
		m.nativeProviderResult = &modelcatalog.NativeProviderResult{RequestID: a.RequestID, Action: a.Action, Error: "Unsupported provider operation."}
		return m, nil
	}
	if m.config.NativeProviders == nil {
		m.nativeProviderResult = &modelcatalog.NativeProviderResult{RequestID: a.RequestID, Action: a.Action, Error: "Native provider setup is unavailable."}
		return m, nil
	}
	m.nativeProviderPending = a.RequestID
	apply := m.config.NativeProviders
	return m, func() tea.Msg { return nativeProviderFinished{Result: apply(a)} }
}
func (m Model) finishNativeProvider(result modelcatalog.NativeProviderResult) (tea.Model, tea.Cmd) {
	if result.RequestID != m.nativeProviderPending {
		return m, nil
	}
	m.nativeProviderPending = ""
	m.nativeProviderResult = &result
	// Future configuration is refreshed without changing captured Work options.
	if result.Confirmed && result.Changed {
		m = m.RefreshDesktopPreferences()
	}
	return m, nil
}
