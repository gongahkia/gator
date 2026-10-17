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
	hash, items, _, err := connectedGoalWatchSnapshotWithPrevious(ctx, definition, stateDir, now, nil)
	return hash, items, err
}

func connectedGoalWatchSnapshotWithPrevious(ctx context.Context, definition jobs.Definition, stateDir string, now time.Time, previous *jobs.Attempt) (string, []jobs.WatchItem, []jobs.WatchCursor, error) {
	local, err := localJobInputHashAt(definition, stateDir, now)
	if err != nil {
		return "", nil, nil, err
	}
	settings, err := loadSettings()
	if err != nil {
		return "", nil, nil, err
	}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		return "", nil, nil, err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return "", nil, nil, err
	}
	if err := verifyGoalOAuthPins(ctx, definition, registry, credentials); err != nil {
		return "", nil, nil, err
	}
	runtime := connector.Runtime{Registry: registry, Credentials: credentials, OAuthRefreshPins: definition.OAuthRefreshPins, OAuthAccountPins: definition.OAuthAccountPins}
	permissions := connector.PermissionSet(settings.ConnectorPermissions)
	digest := sha256.New()
	_, _ = digest.Write([]byte(local))
	items := []jobs.WatchItem{{Source: "local", ID: "source-and-goal", SHA256: local}}
	var cursors []jobs.WatchCursor
	for _, source := range definition.WatchSources {
		if err := writeWatchHash(digest, source); err != nil {
			return "", nil, nil, err
		}
		if err := checkWatchRead(registry, permissions, source.ConnectorID, source.Operation); err != nil {
			return "", nil, nil, err
		}
		descriptor, _ := registry.Get(source.ConnectorID)
		if err := writeWatchHash(digest, descriptor.Resource); err != nil {
			return "", nil, nil, err
		}
		key := source.ConnectorID + "/" + source.Operation
		switch source.Operation {
		case "fetch":
			result, err := runtime.Invoke(ctx, action.Inspect, source.ConnectorID, "fetch", json.RawMessage(`{}`))
			if err != nil {
				return "", nil, nil, fmt.Errorf("poll connector %s: %w", source.ConnectorID, err)
			}
			var value any
			if err := json.Unmarshal(result.Data, &value); err != nil {
				return "", nil, nil, fmt.Errorf("decode connector %s result: %w", source.ConnectorID, err)
			}
			if err := writeWatchHash(digest, value); err != nil {
				return "", nil, nil, err
			}
			fingerprint, err := watchItemHash(value)
			if err != nil {
				return "", nil, nil, err
			}
			items = append(items, jobs.WatchItem{Source: key, ID: "resource", SHA256: fingerprint})
		case "gmail_list":
			cursorAllowed := descriptor.Authentication == connector.AuthNone || definition.OAuthAccountPins[source.ConnectorID] != "" || definition.OAuthRefreshPins[source.ConnectorID] != ""
			found, cursor, err := hashGmailWatch(ctx, runtime, registry, permissions, source, now, previous, cursorAllowed)
			if err != nil {
				return "", nil, nil, err
			}
			if err := writeWatchHash(digest, found); err != nil {
				return "", nil, nil, err
			}
			items = append(items, found...)
			cursors = append(cursors, cursor)
		case "calendar_events":
			found, err := hashCalendarWatch(ctx, digest, runtime, source, now)
			if err != nil {
				return "", nil, nil, err
			}
			items = append(items, found...)
		default:
			return "", nil, nil, errors.New("unsupported goal watch operation")
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Source == items[j].Source {
			return items[i].ID < items[j].ID
		}
		return items[i].Source < items[j].Source
	})
	return hex.EncodeToString(digest.Sum(nil)), items, cursors, nil
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

const maxGmailWatchMessages = 500

func hashGmailWatch(ctx context.Context, runtime connector.Runtime, registry connector.Registry, permissions connector.PermissionSet, source jobs.WatchSource, now time.Time, previous *jobs.Attempt, cursorAllowed bool) ([]jobs.WatchItem, jobs.WatchCursor, error) {
	if err := checkWatchRead(registry, permissions, source.ConnectorID, "gmail_get"); err != nil {
		return nil, jobs.WatchCursor{}, err
	}
	key := source.ConnectorID + "/gmail_list"
	descriptor, _ := registry.Get(source.ConnectorID)
	scopeData, _ := json.Marshal([]string{source.ConnectorID, descriptor.Resource, descriptor.Authentication, source.Query})
	scopeDigest := sha256.Sum256(scopeData)
	scope := hex.EncodeToString(scopeDigest[:])
	priorHashes := map[string]string{}
	var priorCursor jobs.WatchCursor
	if previous != nil && cursorAllowed {
		for _, item := range previous.WatchItems {
			decoded, err := hex.DecodeString(item.SHA256)
			if item.Source == key && validWatchItemID(item.ID) && err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == item.SHA256 {
				priorHashes[item.ID] = item.SHA256
			}
		}
		for _, cursor := range previous.WatchCursors {
			if cursor.Source == key && cursor.ScopeSHA256 == scope {
				priorCursor = cursor
			}
		}
	}
	if priorCursor.HistoryID == "" {
		priorHashes = nil
	}
	fullScan := true
	changed := map[string]bool{}
	cursor := jobs.WatchCursor{Source: key, ScopeSHA256: scope}
	if validWatchHistoryID(priorCursor.HistoryID) && !priorCursor.FullSyncAt.IsZero() && !now.Before(priorCursor.FullSyncAt) && now.Sub(priorCursor.FullSyncAt) < 24*time.Hour {
		var err error
		changed, cursor.HistoryID, fullScan, err = gmailHistoryChanges(ctx, runtime, source.ConnectorID, priorCursor.HistoryID)
		if err != nil {
			return nil, jobs.WatchCursor{}, err
		}
		if !fullScan {
			cursor.FullSyncAt = priorCursor.FullSyncAt
		}
	}
	if fullScan {
		profile, err := runtime.GmailWatchProfile(ctx, source.ConnectorID)
		if err != nil {
			return nil, jobs.WatchCursor{}, fmt.Errorf("read Gmail watch cursor: %w", err)
		}
		var value struct {
			HistoryID string `json:"historyId"`
		}
		if json.Unmarshal(profile.Data, &value) != nil || !validWatchHistoryID(value.HistoryID) {
			return nil, jobs.WatchCursor{}, errors.New("Gmail watch profile returned no valid history cursor")
		}
		cursor.HistoryID = value.HistoryID
		cursor.FullSyncAt = now.UTC()
	}
	ids, err := listGmailWatchIDs(ctx, runtime, source)
	if err != nil {
		return nil, jobs.WatchCursor{}, err
	}
	items := make([]jobs.WatchItem, 0, len(ids))
	for _, id := range ids {
		if old, exists := priorHashes[id]; !fullScan && exists && !changed[id] {
			items = append(items, jobs.WatchItem{Source: key, ID: id, SHA256: old})
			continue
		}
		input, _ := json.Marshal(map[string]string{"resource_id": id})
		message, err := runtime.Invoke(ctx, action.Inspect, source.ConnectorID, "gmail_get", input)
		if err != nil {
			return nil, jobs.WatchCursor{}, fmt.Errorf("read watched Gmail message: %w", err)
		}
		var detail struct {
			ID           string   `json:"id"`
			Snippet      string   `json:"snippet"`
			InternalDate string   `json:"internalDate"`
			LabelIDs     []string `json:"labelIds"`
			Payload      any      `json:"payload"`
		}
		if err := json.Unmarshal(message.Data, &detail); err != nil || detail.ID != id {
			return nil, jobs.WatchCursor{}, errors.New("watched Gmail message was invalid or changed identity")
		}
		sort.Strings(detail.LabelIDs)
		fingerprint, err := watchItemHash(detail)
		if err != nil {
			return nil, jobs.WatchCursor{}, err
		}
		items = append(items, jobs.WatchItem{Source: key, ID: id, SHA256: fingerprint})
	}
	return items, cursor, nil
}

func listGmailWatchIDs(ctx context.Context, runtime connector.Runtime, source jobs.WatchSource) ([]string, error) {
	seenIDs := map[string]bool{}
	seenPages := map[string]bool{}
	var ids []string
	page := ""
	for pages := 0; pages < 10; pages++ {
		input, _ := json.Marshal(map[string]any{"query": source.Query, "limit": 100, "cursor": page})
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
		var envelope map[string]json.RawMessage
		if json.Unmarshal(result.Data, &listing) != nil || json.Unmarshal(result.Data, &envelope) != nil || envelope == nil || envelope["error"] != nil || len(listing.Messages) > 100 {
			return nil, errors.New("Gmail watch returned an invalid message list")
		}
		for _, message := range listing.Messages {
			if !validWatchItemID(message.ID) || seenIDs[message.ID] {
				return nil, errors.New("Gmail watch received an invalid or duplicate message ID")
			}
			seenIDs[message.ID] = true
			ids = append(ids, message.ID)
		}
		if listing.NextPageToken == "" {
			break
		}
		if len(ids) >= maxGmailWatchMessages || pages == 9 || len(listing.NextPageToken) > 2048 || seenPages[listing.NextPageToken] {
			return nil, errors.New("Gmail watch has more than 500 matches or invalid pagination; narrow the query")
		}
		seenPages[listing.NextPageToken] = true
		page = listing.NextPageToken
	}
	sort.Strings(ids)
	return ids, nil
}

func gmailHistoryChanges(ctx context.Context, runtime connector.Runtime, connectorID, startID string) (map[string]bool, string, bool, error) {
	changed := map[string]bool{}
	seenPages := map[string]bool{}
	page := ""
	for pages := 0; pages < 10; pages++ {
		result, err := runtime.GmailWatchHistory(ctx, connectorID, startID, page)
		var status connector.HTTPStatusError
		if errors.As(err, &status) && status.StatusCode == 404 {
			return nil, "", true, nil // expired cursor requires a full scan
		}
		if err != nil {
			return nil, "", false, fmt.Errorf("read Gmail watch history: %w", err)
		}
		var listing struct {
			HistoryID     string            `json:"historyId"`
			History       []json.RawMessage `json:"history"`
			NextPageToken string            `json:"nextPageToken"`
		}
		if json.Unmarshal(result.Data, &listing) != nil || !validWatchHistoryID(listing.HistoryID) {
			return nil, "", false, errors.New("Gmail watch history returned an invalid cursor")
		}
		for _, raw := range listing.History {
			ids, valid := gmailHistoryMessageIDs(raw)
			if !valid {
				return nil, "", true, nil // cannot prove which cached details are unchanged
			}
			for _, id := range ids {
				changed[id] = true
			}
		}
		if listing.NextPageToken == "" {
			return changed, listing.HistoryID, false, nil
		}
		if len(listing.NextPageToken) > 2048 || seenPages[listing.NextPageToken] {
			return nil, "", true, nil
		}
		seenPages[listing.NextPageToken] = true
		page = listing.NextPageToken
	}
	return nil, "", true, nil // bounded history; full scan if it overflows
}

func gmailHistoryMessageIDs(raw json.RawMessage) ([]string, bool) {
	var event struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		MessagesAdded []struct {
			Message struct {
				ID string `json:"id"`
			} `json:"message"`
		} `json:"messagesAdded"`
		MessagesDeleted []struct {
			Message struct {
				ID string `json:"id"`
			} `json:"message"`
		} `json:"messagesDeleted"`
		LabelsAdded []struct {
			Message struct {
				ID string `json:"id"`
			} `json:"message"`
		} `json:"labelsAdded"`
		LabelsRemoved []struct {
			Message struct {
				ID string `json:"id"`
			} `json:"message"`
		} `json:"labelsRemoved"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return nil, false
	}
	ids := make([]string, 0, len(event.Messages)+len(event.MessagesAdded)+len(event.MessagesDeleted)+len(event.LabelsAdded)+len(event.LabelsRemoved))
	for _, value := range event.Messages {
		ids = append(ids, value.ID)
	}
	for _, value := range event.MessagesAdded {
		ids = append(ids, value.Message.ID)
	}
	for _, value := range event.MessagesDeleted {
		ids = append(ids, value.Message.ID)
	}
	for _, value := range event.LabelsAdded {
		ids = append(ids, value.Message.ID)
	}
	for _, value := range event.LabelsRemoved {
		ids = append(ids, value.Message.ID)
	}
	if len(ids) == 0 {
		return nil, false
	}
	for _, id := range ids {
		if !validWatchItemID(id) {
			return nil, false
		}
	}
	return ids, true
}

func validWatchHistoryID(value string) bool {
	if len(value) == 0 || len(value) > 32 {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

const maxCalendarWatchEvents = 500

func hashCalendarWatch(ctx context.Context, digest hash.Hash, runtime connector.Runtime, source jobs.WatchSource, now time.Time) ([]jobs.WatchItem, error) {
	day := now.UTC().Truncate(24 * time.Hour)
	windowStart := day.Add(-24 * time.Hour).Format(time.RFC3339)
	windowEnd := day.Add(time.Duration(source.Days+1) * 24 * time.Hour).Format(time.RFC3339)
	items := make([]string, 0)
	watchItems := make([]jobs.WatchItem, 0)
	seen := make(map[string]bool)
	seenPages := make(map[string]bool)
	page := ""
	for pages := 0; pages < 5; pages++ {
		input, _ := json.Marshal(map[string]any{"time_min": windowStart, "time_max": windowEnd, "limit": 100, "cursor": page})
		result, err := runtime.Invoke(ctx, action.Inspect, source.ConnectorID, "calendar_events", input)
		if err != nil {
			return nil, fmt.Errorf("poll Calendar %s: %w", source.ConnectorID, err)
		}
		var listing struct {
			Items         []json.RawMessage `json:"items"`
			NextPageToken string            `json:"nextPageToken"`
		}
		var envelope map[string]json.RawMessage
		if json.Unmarshal(result.Data, &listing) != nil || json.Unmarshal(result.Data, &envelope) != nil || envelope == nil || envelope["error"] != nil || envelope["items"] == nil || listing.Items == nil || len(listing.Items) > 100 {
			return nil, errors.New("Calendar watch returned an invalid events response")
		}
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
		if listing.NextPageToken == "" {
			break
		}
		if len(items) >= maxCalendarWatchEvents || pages == 4 || len(listing.NextPageToken) > 2048 || seenPages[listing.NextPageToken] {
			return nil, errors.New("Calendar watch has more than 500 events or invalid pagination; narrow its window")
		}
		seenPages[listing.NextPageToken] = true
		page = listing.NextPageToken
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
