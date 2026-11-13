package main

import (
	"errors"
	"slices"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/workapp"
	"github.com/gongahkia/gator/internal/worksession"
)

type sourceReadCheck struct {
	binding string
	at      time.Time
	passed  bool
}

func (c *connectionControls) recordSourceCheck(id, binding string, passed bool) {
	c.checkMu.Lock()
	defer c.checkMu.Unlock()
	if len(c.checks) >= 128 {
		c.checks = nil
	}
	if c.checks == nil {
		c.checks = map[string]sourceReadCheck{}
	}
	c.checks[id] = sourceReadCheck{binding: binding, at: time.Now(), passed: passed}
}

// Inventory reads local configuration only. A saved credential is never labeled
// live access. Explicit read-check observations are volatile and account-bound.
func (c *connectionControls) composerSources(settings config.Store) ([]workapp.ComposerSource, error) {
	current, err := settings.Load()
	if err != nil {
		return nil, err
	}
	credentials, err := auth.New(c.stateDir)
	if err != nil {
		return nil, err
	}
	choices := []workapp.ComposerSource{}
	for _, d := range current.Connectors {
		credential, err := connectionCredential(credentials, d)
		status, statusErr := nativeCredentialStatus(d, credentials)
		choice := workapp.ComposerSource{ID: d.ID, Label: d.Name, Service: connectionServiceLabel(d.Kind), Status: status, Access: "Read access not checked", Binding: connectorLoginReview(d) + credentialReview(credential)}
		read := false
		for _, op := range d.Operations() {
			read = read || op.Capability == action.ConnectedRead
		}
		switch {
		case err != nil || statusErr != nil:
			choice.Unavailable = "Account status unavailable"
		case !read:
			choice.Unavailable = "No supported read operations; manage this action connection in Settings"
		case d.Authentication != connector.AuthNone && credential == nil:
			choice.Unavailable = "Connect this account in Settings"
		case credential != nil && credential.Expired(time.Now()) && credential.Refresh == "":
			choice.Unavailable = "Reconnect this expired account in Settings"
		}
		c.checkMu.Lock()
		check, ok := c.checks[d.ID]
		c.checkMu.Unlock()
		if ok && check.binding == choice.Binding && choice.Unavailable == "" {
			choice.CheckedAt = check.at.UTC().Format(time.RFC3339)
			if !check.passed {
				choice.Access = "Last read check failed; manage this connection"
			} else if time.Since(check.at) > 5*time.Minute {
				choice.Access = "Previous read check is stale"
			} else {
				choice.Access = "Read access checked recently; permissions still apply"
			}
		}
		choices = append(choices, choice)
	}
	return choices, nil
}
func selectComposerSources(settings config.Store, stateDir, id string, expected, ids []string) error {
	release, err := state.Guard(stateDir, false)
	if err != nil {
		return err
	}
	defer release()
	current, err := settings.Load()
	if err != nil {
		return err
	}
	for _, selected := range ids {
		found := false
		for _, d := range current.Connectors {
			found = found || d.ID == selected
		}
		if !found {
			return errors.New("a connection was removed; refresh sources")
		}
	}
	sessions, err := worksession.Open(stateDir)
	if err != nil {
		return err
	}
	prior, err := sessions.LoadPreferences(id)
	if err != nil {
		return err
	}
	if prior.ConnectorIDs != nil && !slices.Equal(*prior.ConnectorIDs, expected) {
		return errors.New("connection selection changed in another request; reopen this chat")
	}
	selected := append([]string{}, ids...)
	next := prior
	next.ConnectorIDs = &selected
	return sessions.SavePreferences(id, next, &prior)
}
