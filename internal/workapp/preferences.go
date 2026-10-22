package workapp

// RefreshDesktopPreferences is a private host notification after a user edits
// voice preferences. It exposes no command or settings payload to the renderer.
func (m Model) RefreshDesktopPreferences() Model {
	if m.config.ReadDesktop != nil {
		if preferences, err := m.config.ReadDesktop(); err == nil {
			m.desktop = preferences
		} else {
			m.bridgeNotice = "Could not reload desktop preferences: " + err.Error()
		}
	}
	return m
}
