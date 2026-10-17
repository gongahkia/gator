package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
)

// probeGoalAccess performs bounded, inspect-only reads of the goal's selected
// watch sources. It does not update watch fingerprints or retain response data.
func probeGoalAccess(ctx context.Context, stateDir, goalID string, out io.Writer) error {
	goals, err := goal.Open(stateDir)
	if err != nil {
		return err
	}
	if _, err := goals.Load(goalID); err != nil {
		return err
	}
	store, err := jobs.Open(stateDir)
	if err != nil {
		return err
	}
	definitions, err := store.List()
	if err != nil {
		return err
	}
	settings, err := loadSettings()
	if err != nil {
		return err
	}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		return err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return err
	}
	runtime := connector.Runtime{Registry: registry, Credentials: credentials}
	permissions := connector.PermissionSet(settings.ConnectorPermissions)
	if _, err := fmt.Fprintf(out, "Selected goal watch access for %s (live read; no watch state changed):\n", goalID); err != nil {
		return err
	}
	count := 0
	for _, definition := range definitions {
		if definition.GoalID != goalID || !definition.GoalWatch {
			continue
		}
		pinErr := verifyGoalOAuthPins(definition, registry, credentials)
		boundRuntime := runtime
		boundRuntime.OAuthRefreshPins = definition.OAuthRefreshPins
		for _, source := range definition.WatchSources {
			count++
			if count > 8 {
				continue
			}
			if pinErr != nil {
				if _, err := fmt.Fprintf(out, "  %s %s/%s: %s\n", definition.ID, source.ConnectorID, source.Operation, activityLine(pinErr.Error(), 160)); err != nil {
					return err
				}
				continue
			}
			probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			status := probeGoalWatchSource(probeCtx, boundRuntime, registry, permissions, credentials, source, time.Now())
			cancel()
			if _, err := fmt.Fprintf(out, "  %s %s/%s: %s\n", definition.ID, source.ConnectorID, source.Operation, status); err != nil {
				return err
			}
		}
	}
	if count == 0 {
		_, err = fmt.Fprintln(out, "  No selected connected watch sources.")
		return err
	}
	if count > 8 {
		_, err = fmt.Fprintf(out, "  ... and %d more selected sources; narrow the goal or inspect its other watches.\n", count-8)
	}
	return err
}

func probeGoalWatchSource(ctx context.Context, runtime connector.Runtime, registry connector.Registry, permissions connector.PermissionSet, credentials auth.Store, source jobs.WatchSource, now time.Time) string {
	descriptor, found := registry.Get(source.ConnectorID)
	if !found {
		return "connector configuration missing; edit or disable this watch"
	}
	status, err := connectorAuthStatus(descriptor, credentials)
	if err != nil {
		return "credential status unavailable; inspect gator provider connector status " + source.ConnectorID
	}
	if status == "login required" || status == "credential expired; login required" {
		return status + "; run gator provider connector login " + source.ConnectorID
	}
	if err := checkWatchRead(registry, permissions, source.ConnectorID, source.Operation); err != nil {
		return "selected read is unavailable or permission is missing; inspect gator provider connector status " + source.ConnectorID
	}
	if source.Operation == "gmail_list" {
		if err := checkWatchRead(registry, permissions, source.ConnectorID, "gmail_get"); err != nil {
			return "Gmail detail read permission is missing; inspect gator provider connector status " + source.ConnectorID
		}
	}
	var input []byte
	switch source.Operation {
	case "fetch":
		input = []byte(`{}`)
	case "gmail_list":
		input, _ = json.Marshal(map[string]any{"query": source.Query, "limit": 1})
	case "calendar_events":
		day := now.UTC().Truncate(24 * time.Hour)
		input, _ = json.Marshal(map[string]any{
			"time_min": day.Add(-24 * time.Hour).Format(time.RFC3339),
			"time_max": day.Add(time.Duration(source.Days+1) * 24 * time.Hour).Format(time.RFC3339),
			"limit":    1,
		})
	default:
		return "unsupported selected watch operation"
	}
	result, err := runtime.Invoke(ctx, action.Inspect, source.ConnectorID, source.Operation, input)
	if err != nil {
		return goalAccessFailure(err, source.ConnectorID)
	}
	switch source.Operation {
	case "gmail_list":
		var listing struct {
			Messages []struct {
				ID string `json:"id"`
			} `json:"messages"`
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal(result.Data, &listing) != nil || listing.Error != nil {
			return "read returned an invalid Gmail list; inspect the connector"
		}
		if len(listing.Messages) == 0 {
			return "list reachable; no matching message to test detail read"
		}
		id := listing.Messages[0].ID
		if !validWatchItemID(id) {
			return "read returned an invalid Gmail message ID"
		}
		getInput, _ := json.Marshal(map[string]string{"resource_id": id})
		detail, err := runtime.Invoke(ctx, action.Inspect, source.ConnectorID, "gmail_get", getInput)
		if err != nil {
			return "list reachable; detail " + goalAccessFailure(err, source.ConnectorID)
		}
		var message struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(detail.Data, &message) != nil || message.ID != id {
			return "list reachable; detail response did not match the selected message"
		}
		return "list and one message detail reachable"
	case "calendar_events":
		var listing struct {
			Items []json.RawMessage `json:"items"`
			Error json.RawMessage   `json:"error"`
		}
		if json.Unmarshal(result.Data, &listing) != nil || listing.Items == nil || listing.Error != nil {
			return "read returned an invalid Calendar event list; inspect the connector"
		}
		return "event list reachable"
	default:
		return "JSON source reachable"
	}
}

func goalAccessFailure(err error, connectorID string) string {
	if err == nil {
		return "read succeeded"
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "HTTP 401"):
		return "unauthorized; run gator provider connector login " + connectorID
	case strings.Contains(message, "HTTP 403"):
		return "forbidden; check connector scopes and read permission"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "read timed out or was cancelled; try again when the service is available"
	default:
		return "read failed; inspect gator provider connector status " + connectorID
	}
}
