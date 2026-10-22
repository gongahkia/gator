package workapp

// RestartBlockReason is checked inside the application message loop, after
// earlier submitted actions. A stale host snapshot cannot authorize shutdown.
func (m Model) RestartBlockReason() string {
	if m.running || m.cancelling || m.interaction != nil || len(m.pendingInteractions) != 0 {
		return "Finish or stop the current work before restarting."
	}
	if len(m.queue) != 0 || m.pendingPrompt != "" {
		return "Send or remove pending messages before restarting."
	}
	if m.bridgeExternal != nil || m.pendingConnectorCmd != nil {
		return "Finish the current sign-in or reviewed action first."
	}
	if m.pendingBundleAction != nil {
		return "Finish or cancel the current review before restarting."
	}
	panel := m.BridgeState().Panel
	if panel != nil {
		if panel.Kind == "confirmation" {
			return "Finish or cancel the review before restarting."
		}
		for _, row := range panel.Rows {
			if row.Input != nil {
				return "Save or close the form before restarting."
			}
		}
	}
	return ""
}
