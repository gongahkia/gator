// Package productguide owns shared, non-secret product descriptions. The desktop
// imports the same JSON that the runtime embeds; neither copy grants authority.
package productguide

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed capabilities.json
var bundled []byte

type Guide struct {
	Version           int      `json:"version"`
	ConnectionsTitle  string   `json:"connections_title"`
	ConnectionsSetup  string   `json:"connections_setup"`
	EmptySources      string   `json:"empty_sources"`
	PilotStart        string   `json:"pilot_start"`
	Help              string   `json:"help"`
	ModelConnections  string   `json:"model_connections"`
	ModelAvailability string   `json:"model_availability"`
	GoogleOperations  []string `json:"google_operations"`
}

var read = sync.OnceValue(func() Guide {
	var guide Guide
	if err := json.Unmarshal(bundled, &guide); err != nil || guide.Version != 1 {
		panic("invalid bundled Gator capability guide")
	}
	for _, text := range []string{guide.ConnectionsTitle, guide.ConnectionsSetup, guide.EmptySources, guide.PilotStart, guide.Help, guide.ModelConnections, guide.ModelAvailability} {
		if strings.TrimSpace(text) == "" {
			panic("incomplete bundled Gator capability guide")
		}
	}
	return guide
})

// Current returns a private copy of the bundled description, not the user's
// live configuration. Available tools and verified state determine run access.
func Current() Guide {
	guide := read()
	guide.GoogleOperations = append([]string(nil), guide.GoogleOperations...)
	return guide
}
