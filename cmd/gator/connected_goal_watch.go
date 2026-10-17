package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/jobs"
)

// connectedGoalWatchHash polls only the user-selected read operations. The
// digest is retained in a job attempt, so a restart can skip an unchanged
// remote result without retaining private message bodies in scheduler state.
// The Work run reads live evidence again after the gate; the next poll catches
// any change between these reads.
func connectedGoalWatchHash(ctx context.Context, definition jobs.Definition, stateDir string, now time.Time) (string, error) {
	hash, _, err := connectedGoalWatchSnapshot(ctx, definition, stateDir, now)
	return hash, err
}

func connectedGoalWatchSnapshot(ctx context.Context, definition jobs.Definition, stateDir string, now time.Time) (string, []jobs.WatchItem, error) {
	local, err := localJobInputHashAt(definition, stateDir, now)
	if err != nil {
		return "", nil, err
	}
	settings, err := loadSettings()
	if err != nil {
		return "", nil, err
	}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		return "", nil, err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return "", nil, err
	}
	if err := verifyGoalOAuthPins(ctx, definition, registry, credentials); err != nil {
		return "", nil, err
	}
	runtime := connector.Runtime{Registry: registry, Credentials: credentials, OAuthRefreshPins: definition.OAuthRefreshPins, OAuthAccountPins: definition.OAuthAccountPins}
	permissions := connector.PermissionSet(settings.ConnectorPermissions)
	digest := sha256.New()
	_, _ = digest.Write([]byte(local))
	items := []jobs.WatchItem{{Source: "local", ID: "source-and-goal", SHA256: local}}
	for _, source := range definition.WatchSources {
		if err := writeWatchHash(digest, source); err != nil {
			return "", nil, err
		}
		if err := checkWatchRead(registry, permissions, source.ConnectorID, source.Operation); err != nil {
			return "", nil, err
		}
		descriptor, _ := registry.Get(source.ConnectorID)
		if err := writeWatchHash(digest, descriptor.Resource); err != nil {
			return "", nil, err
		}
		key := source.ConnectorID + "/" + source.Operation
		switch source.Operation {
		case "fetch":
			result, err := runtime.Invoke(ctx, action.Inspect, source.ConnectorID, "fetch", json.RawMessage(`{}`))
			if err != nil {
				return "", nil, fmt.Errorf("poll connector %s: %w", source.ConnectorID, err)
			}
			var value any
			if err := json.Unmarshal(result.Data, &value); err != nil {
				return "", nil, fmt.Errorf("decode connector %s result: %w", source.ConnectorID, err)
			}
			if err := writeWatchHash(digest, value); err != nil {
				return "", nil, err
			}
			fingerprint, err := watchItemHash(value)
			if err != nil {
				return "", nil, err
			}
			items = append(items, jobs.WatchItem{Source: key, ID: "resource", SHA256: fingerprint})
		case "gmail_list":
			found, err := hashGmailWatch(ctx, digest, runtime, registry, permissions, source)
			if err != nil {
				return "", nil, err
			}
			items = append(items, found...)
		case "calendar_events":
			found, err := hashCalendarWatch(ctx, digest, runtime, source, now)
			if err != nil {
				return "", nil, err
			}
			items = append(items, found...)
		default:
			return "", nil, errors.New("unsupported goal watch operation")
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Source == items[j].Source {
			return items[i].ID < items[j].ID
		}
		return items[i].Source < items[j].Source
	})
	return hex.EncodeToString(digest.Sum(nil)), items, nil
}

func checkWatchRead(registry connector.Registry, permissions connector.PermissionSet, connectorID, operationID string) error {
	_, operation, err := registry.Operation(connectorID, operationID)
	if err != nil {
		return err
	}
	if operation.Capability != action.ConnectedRead || permissions.Resolve(connectorID, operationID, operation.Capability) != connector.PermissionAllow {
		return fmt.Errorf("connector %q operation %q cannot be read unattended", connectorID, operationID)
	}
	return nil
}

func hashGmailWatch(ctx context.Context, digest hash.Hash, runtime connector.Runtime, registry connector.Registry, permissions connector.PermissionSet, source jobs.WatchSource) ([]jobs.WatchItem, error) {
	if err := checkWatchRead(registry, permissions, source.ConnectorID, "gmail_get"); err != nil {
		return nil, err
	}
	input, _ := json.Marshal(map[string]any{"query": source.Query, "limit": 100})
	result, err := runtime.Invoke(ctx, action.Inspect, source.ConnectorID, "gmail_list", input)
	if err != nil {
		return nil, fmt.Errorf("poll Gmail %s: %w", source.ConnectorID, err)
	}
	var listing struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal(result.Data, &listing); err != nil {
		return nil, fmt.Errorf("decode Gmail list: %w", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(result.Data, &envelope); err != nil || envelope["error"] != nil {
		return nil, errors.New("Gmail watch returned an error instead of messages")
	}
	if listing.NextPageToken != "" || len(listing.Messages) > 100 {
		return nil, errors.New("Gmail watch query has more than 100 matches; narrow the query before relying on this watch")
	}
	ids := make([]string, 0, len(listing.Messages))
	seen := make(map[string]bool)
	for _, message := range listing.Messages {
		if !validWatchItemID(message.ID) || seen[message.ID] {
			return nil, errors.New("Gmail watch received an invalid or duplicate message ID")
		}
		seen[message.ID] = true
		ids = append(ids, message.ID)
	}
	sort.Strings(ids)
	items := make([]jobs.WatchItem, 0, len(ids))
	for _, id := range ids {
		input, _ := json.Marshal(map[string]string{"resource_id": id})
		message, err := runtime.Invoke(ctx, action.Inspect, source.ConnectorID, "gmail_get", input)
		if err != nil {
			return nil, fmt.Errorf("read watched Gmail message: %w", err)
		}
		var detail struct {
			ID           string `json:"id"`
			Snippet      string `json:"snippet"`
			InternalDate string `json:"internalDate"`
			Payload      any    `json:"payload"`
		}
		if err := json.Unmarshal(message.Data, &detail); err != nil || detail.ID != id {
			return nil, errors.New("watched Gmail message was invalid or changed identity")
		}
		if err := writeWatchHash(digest, detail); err != nil {
			return nil, err
		}
		fingerprint, err := watchItemHash(detail)
		if err != nil {
			return nil, err
		}
		items = append(items, jobs.WatchItem{Source: source.ConnectorID + "/gmail_list", ID: id, SHA256: fingerprint})
	}
	return items, nil
}

func hashCalendarWatch(ctx context.Context, digest hash.Hash, runtime connector.Runtime, source jobs.WatchSource, now time.Time) ([]jobs.WatchItem, error) {
	day := now.UTC().Truncate(24 * time.Hour)
	input, _ := json.Marshal(map[string]any{
		"time_min": day.Add(-24 * time.Hour).Format(time.RFC3339),
		"time_max": day.Add(time.Duration(source.Days+1) * 24 * time.Hour).Format(time.RFC3339),
		"limit":    100,
	})
	result, err := runtime.Invoke(ctx, action.Inspect, source.ConnectorID, "calendar_events", input)
	if err != nil {
		return nil, fmt.Errorf("poll Calendar %s: %w", source.ConnectorID, err)
	}
	var listing struct {
		Items         []json.RawMessage `json:"items"`
		NextPageToken string            `json:"nextPageToken"`
	}
	if err := json.Unmarshal(result.Data, &listing); err != nil {
		return nil, fmt.Errorf("decode Calendar events: %w", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(result.Data, &envelope); err != nil || envelope["error"] != nil || envelope["items"] == nil || listing.Items == nil {
		return nil, errors.New("Calendar watch returned an invalid events response")
	}
	if listing.NextPageToken != "" || len(listing.Items) > 100 {
		return nil, errors.New("Calendar watch has more than 100 events; narrow its window before relying on this watch")
	}
	items := make([]string, 0, len(listing.Items))
	watchItems := make([]jobs.WatchItem, 0, len(listing.Items))
	seen := make(map[string]bool)
	for _, item := range listing.Items {
		var value map[string]any
		if err := json.Unmarshal(item, &value); err != nil || value == nil {
			return nil, errors.New("Calendar watch received an invalid event")
		}
		canonical, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		items = append(items, string(canonical))
		fingerprint := sha256.Sum256(canonical)
		id, _ := value["id"].(string)
		if id == "" {
			id = "content-" + hex.EncodeToString(fingerprint[:8])
		}
		if !validWatchItemID(id) || seen[id] {
			return nil, errors.New("Calendar watch received an invalid or duplicate event ID")
		}
		seen[id] = true
		watchItems = append(watchItems, jobs.WatchItem{Source: source.ConnectorID + "/calendar_events", ID: id, SHA256: hex.EncodeToString(fingerprint[:])})
	}
	sort.Strings(items)
	if err := writeWatchHash(digest, items); err != nil {
		return nil, err
	}
	return watchItems, nil
}

func watchItemHash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func validWatchItemID(value string) bool {
	return value != "" && len(value) <= 256 && utf8.ValidString(value) && !strings.ContainsFunc(value, unicode.IsControl)
}

func writeWatchHash(digest hash.Hash, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = digest.Write(append(data, '\n'))
	return err
}
