package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	gatorbrowser "github.com/gongahkia/gator/internal/browser"
)

// tuiBrowserBackend keeps browser process creation in the command layer while
// giving the TUI typed, local-only lifecycle operations. It never shells out
// and never exposes a CDP endpoint or token to the agent runner.
type tuiBrowserBackend struct {
	store    *gatorbrowser.Store
	stateDir string
}

func newTUIBrowserBackend(stateDir string) (*tuiBrowserBackend, error) {
	store, err := gatorbrowser.Open(stateDir)
	if err != nil {
		return nil, err
	}
	return &tuiBrowserBackend{store: store, stateDir: stateDir}, nil
}

func (backend *tuiBrowserBackend) Sessions() ([]gatorbrowser.Session, error) {
	return backend.store.List()
}

func (backend *tuiBrowserBackend) Start(headed, visualCapture bool) (gatorbrowser.Session, error) {
	status := gatorbrowser.Runtime(backend.store)
	if !status.Installed {
		return gatorbrowser.Session{}, errors.New(status.Error)
	}
	session, err := backend.store.Start(gatorbrowser.StartOptions{Headed: headed, VisualCapture: visualCapture})
	if err != nil {
		return gatorbrowser.Session{}, err
	}
	if err := launchBrowserDaemon(backend.store, backend.stateDir, session, "", io.Discard); err != nil {
		_, _ = backend.store.Stop(session.ID)
		return gatorbrowser.Session{}, err
	}
	client, err := gatorbrowser.NewClient(backend.store, session.ID)
	if err != nil {
		return gatorbrowser.Session{}, err
	}
	tabs, err := client.CandidateTabs(context.Background())
	if err != nil || len(tabs) != 1 {
		if err == nil {
			err = errors.New("managed browser session did not create exactly one initial tab")
		}
		return gatorbrowser.Session{}, err
	}
	return client.SelectTabs(context.Background(), tabs)
}

func (backend *tuiBrowserBackend) Attach(cdpEndpoint string, visualCapture bool) (gatorbrowser.Session, []gatorbrowser.Tab, error) {
	status := gatorbrowser.Runtime(backend.store)
	if !status.Installed {
		return gatorbrowser.Session{}, nil, errors.New(status.Error)
	}
	if err := gatorbrowser.ValidateCDPEndpoint(cdpEndpoint); err != nil {
		return gatorbrowser.Session{}, nil, err
	}
	session, err := backend.store.Attach(gatorbrowser.AttachOptions{CDPEndpoint: cdpEndpoint, VisualCapture: visualCapture})
	if err != nil {
		return gatorbrowser.Session{}, nil, err
	}
	if err := launchBrowserDaemon(backend.store, backend.stateDir, session, cdpEndpoint, io.Discard); err != nil {
		_, _ = backend.store.Stop(session.ID)
		return gatorbrowser.Session{}, nil, err
	}
	client, err := gatorbrowser.NewClient(backend.store, session.ID)
	if err != nil {
		return gatorbrowser.Session{}, nil, err
	}
	tabs, err := client.CandidateTabs(context.Background())
	return session, tabs, err
}

func (backend *tuiBrowserBackend) CandidateTabs(sessionID string) ([]gatorbrowser.Tab, error) {
	client, err := backend.client(sessionID)
	if err != nil {
		return nil, err
	}
	return client.CandidateTabs(context.Background())
}

func (backend *tuiBrowserBackend) SelectTabs(sessionID string, tabIDs []string) (gatorbrowser.Session, error) {
	if len(tabIDs) == 0 {
		return gatorbrowser.Session{}, errors.New("select at least one browser tab")
	}
	client, err := backend.client(sessionID)
	if err != nil {
		return gatorbrowser.Session{}, err
	}
	candidates, err := client.CandidateTabs(context.Background())
	if err != nil {
		return gatorbrowser.Session{}, err
	}
	byID := make(map[string]gatorbrowser.Tab, len(candidates))
	for _, tab := range candidates {
		byID[tab.ID] = tab
	}
	selected := make([]gatorbrowser.Tab, 0, len(tabIDs))
	for _, id := range tabIDs {
		tab, found := byID[id]
		if !found {
			return gatorbrowser.Session{}, fmt.Errorf("browser tab %q is unavailable", id)
		}
		selected = append(selected, tab)
	}
	return client.SelectTabs(context.Background(), selected)
}

func (backend *tuiBrowserBackend) AddOrigin(sessionID, value string) (gatorbrowser.Session, error) {
	client, err := backend.client(sessionID)
	if err != nil {
		return gatorbrowser.Session{}, err
	}
	return client.AddOrigin(context.Background(), value)
}

func (backend *tuiBrowserBackend) RemoveOrigin(sessionID, value string) (gatorbrowser.Session, error) {
	client, err := backend.client(sessionID)
	if err != nil {
		return gatorbrowser.Session{}, err
	}
	return client.RemoveOrigin(context.Background(), value)
}

func (backend *tuiBrowserBackend) SetVisualCapture(sessionID string, allowed bool) (gatorbrowser.Session, error) {
	client, err := backend.client(sessionID)
	if err != nil {
		return gatorbrowser.Session{}, err
	}
	return client.SetVisualCapture(context.Background(), allowed)
}

func (backend *tuiBrowserBackend) AllowUpload(sessionID, path string) (gatorbrowser.Upload, error) {
	client, err := backend.client(sessionID)
	if err != nil {
		return gatorbrowser.Upload{}, err
	}
	_, upload, err := client.AllowUpload(context.Background(), path)
	return upload, err
}

func (backend *tuiBrowserBackend) Artifacts(sessionID string) ([]gatorbrowser.Artifact, error) {
	client, err := backend.client(sessionID)
	if err != nil {
		return nil, err
	}
	return client.Artifacts()
}

func (backend *tuiBrowserBackend) ExportArtifact(sessionID, artifactID, destination string) error {
	client, err := backend.client(sessionID)
	if err != nil {
		return err
	}
	return client.ExportArtifact(artifactID, destination)
}

func (backend *tuiBrowserBackend) Stop(sessionID string) (gatorbrowser.Session, error) {
	client, err := backend.client(sessionID)
	if err != nil {
		return gatorbrowser.Session{}, err
	}
	return client.Stop(context.Background())
}

func (backend *tuiBrowserBackend) Controller(sessionID string) (gatorbrowser.Controller, error) {
	client, err := backend.client(sessionID)
	if err != nil {
		return nil, err
	}
	if _, err := client.Session(context.Background(), sessionID); err != nil {
		return nil, err
	}
	return client, nil
}

func (backend *tuiBrowserBackend) client(sessionID string) (*gatorbrowser.Client, error) {
	return gatorbrowser.NewClient(backend.store, sessionID)
}
