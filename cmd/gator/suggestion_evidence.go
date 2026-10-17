package main

import (
	"encoding/json"
	"strings"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/suggestion"
	"github.com/gongahkia/gator/internal/workspace"
)

type suggestionEvidenceCheck struct {
	Status     string   `json:"status"`
	Verified   []string `json:"verified,omitempty"`
	Unverified []string `json:"unverified,omitempty"`
	Note       string   `json:"note"`
}

// checkSuggestionEvidence checks only reference integrity. Even a valid
// locator can be irrelevant to, or contradict, the suggested claim.
func checkSuggestionEvidence(stateDir string, record suggestion.Record) suggestionEvidenceCheck {
	refs := append([]string(nil), record.SourceRefs...)
	refs = append(refs, record.PlanChange.Evidence...)
	check := suggestionEvidenceCheck{Status: "no_references", Note: "A matching reference does not prove that its source supports the suggestion."}
	if len(refs) == 0 {
		return check
	}
	check.Status = "work_unavailable"
	if !workReferencePattern.MatchString(record.WorkID) {
		check.Unverified = refs
		return check
	}
	bundle, found, err := resolveWorkBundle(record.WorkID, stateDir)
	if err != nil || !found || artifact.VerifyBundle(bundle) != nil {
		check.Unverified = refs
		return check
	}
	work, err := workspace.OpenWork(bundle.Path)
	if err != nil {
		check.Unverified = refs
		return check
	}
	// The present goal plan may have changed since this Work run. Its steps
	// cannot prove what the model saw when it proposed the suggestion.
	allowed := newWatchReferenceSet(bundle.Manifest, observedSuggestionItems(bundle), goal.Plan{}, work)
	for _, ref := range refs {
		if allowed.allows(strings.TrimSpace(ref)) {
			check.Verified = append(check.Verified, ref)
		} else {
			check.Unverified = append(check.Unverified, ref)
		}
	}
	check.Status = "verified"
	if len(check.Unverified) > 0 {
		check.Status = "unverified"
	}
	return check
}

// observedSuggestionItems reconstructs bounded item references from the
// bundle's verified connected snapshots, independent of retained watch state.
func observedSuggestionItems(bundle artifact.Bundle) []jobs.WatchItem {
	items := make([]jobs.WatchItem, 0, len(bundle.Manifest.ConnectedSources))
	for _, source := range bundle.Manifest.ConnectedSources {
		if source.SnapshotPath == "" {
			continue
		}
		data, err := bundle.Root.ReadRegularFile(source.SnapshotPath, 256*1024)
		if err != nil {
			continue
		}
		switch source.Operation {
		case "gmail_get":
			var message struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(data, &message) == nil && message.ID != "" {
				items = append(items, jobs.WatchItem{Source: source.ConnectorID + "/gmail_list", ID: message.ID})
			}
		case "calendar_events":
			var events struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			if json.Unmarshal(data, &events) == nil {
				for _, event := range events.Items {
					if event.ID != "" {
						items = append(items, jobs.WatchItem{Source: source.ConnectorID + "/calendar_events", ID: event.ID})
					}
				}
			}
		case "fetch":
			items = append(items, jobs.WatchItem{Source: source.ConnectorID + "/fetch", ID: "resource"})
		}
	}
	return items
}
