package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/workapp"
)

var retainedKnowledgeID = regexp.MustCompile(`^(mem|note)-[a-f0-9]{24}$`)
var retainedKnowledgeLocator = regexp.MustCompile(`^knowledge:([a-zA-Z0-9][a-zA-Z0-9_-]{0,95})/(knowledge_read|knowledge_search|knowledge_refresh)/[a-f0-9]{24}$`)

func retainedKnowledgeReplay(messages []agent.Message) map[string]bool {
	result := map[string]bool{}
	for _, message := range messages {
		if message.Role == agent.RoleTool && (message.ToolName == "knowledge_read" || message.ToolName == "knowledge_search") {
			digest := sha256.Sum256([]byte(message.Content))
			result[message.ToolName+":"+hex.EncodeToString(digest[:])] = true
		}
	}
	return result
}

type suppliedKnowledge struct {
	ID, Kind, Title, Text, Body, Source, Updated string
	Status                                       string
	Truncated                                    bool
	GoalID                                       *string  `json:"goal_id"`
	SourceRefs                                   []string `json:"source_refs"`
	Origin                                       *struct {
		ConversationID string `json:"conversation_id"`
		Basis          string `json:"basis"`
	}
}

// Read only hash-bound tool responses from this answer's sealed bundle. Current
// saved text/revision remains separate and supplies the usual mutation guard.
func appendRetainedKnowledge(result *workapp.ContextState, bundle artifact.Bundle, load func(string) workapp.KnowledgeItem, replayed map[string]bool) error {
	if bundle.Manifest.RunID != result.WorkID {
		return errors.New("retained context belongs to a different run")
	}
	for _, evidence := range bundle.Manifest.Evidence {
		match := retainedKnowledgeLocator.FindStringSubmatch(evidence.Locator)
		if match == nil {
			continue
		}
		read, refresh, inherited := match[2] == "knowledge_read", match[2] == "knowledge_refresh", match[1] != result.WorkID
		if inherited && !replayed[match[2]+":"+evidence.SHA256] {
			continue
		}
		if evidence.SnapshotPath != "evidence/"+evidence.SHA256+".txt" {
			return errors.New("retained knowledge response is unavailable")
		}
		data, err := bundle.Root.ReadRegularFile(evidence.SnapshotPath, 512*1024)
		if err != nil {
			return fmt.Errorf("read supplied knowledge: %w", err)
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != evidence.SHA256 {
			return errors.New("retained knowledge response changed")
		}
		var response struct {
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(data, &response); err != nil || !response.OK {
			return errors.New("retained knowledge response is invalid")
		}
		var snapshots []suppliedKnowledge
		if refresh {
			var status struct {
				Items []suppliedKnowledge `json:"items"`
			}
			if err := json.Unmarshal(response.Result, &status); err != nil {
				return err
			}
			snapshots = status.Items
		} else if read {
			var item suppliedKnowledge
			if err := json.Unmarshal(response.Result, &item); err != nil {
				return err
			}
			snapshots = []suppliedKnowledge{item}
		} else {
			// A partial-store warning wraps the same successful search items.
			if err := json.Unmarshal(response.Result, &snapshots); err != nil {
				var partial struct {
					Items   []suppliedKnowledge `json:"items"`
					Warning string              `json:"warning"`
				}
				if err := json.Unmarshal(response.Result, &partial); err != nil {
					return err
				}
				snapshots = partial.Items
				if partial.Warning != "" {
					result.Recall = strings.TrimSpace(result.Recall + " " + partial.Warning)
				}
			}
		}
		for _, snapshot := range snapshots {
			foreign := snapshot.GoalID != nil && *snapshot.GoalID != "" && *snapshot.GoalID != result.GoalID
			if !retainedKnowledgeID.MatchString(snapshot.ID) || foreign && !inherited {
				return errors.New("retained knowledge has an invalid identity or goal scope")
			}
			var item workapp.KnowledgeItem
			if foreign || refresh && snapshot.Status == "outside_scope" {
				item.ID = snapshot.ID
				item.UnavailableReason = "This excerpt was carried from another goal. Open its original conversation to inspect or correct its current saved record."
				if refresh {
					item.UnavailableReason = "This saved reference is outside the current goal. Open its original conversation to inspect or correct it."
				}
			} else {
				item = load(snapshot.ID)
			}
			item.Kind, item.Title, item.Text = "memory", snapshot.Kind, snapshot.Text
			if strings.HasPrefix(snapshot.ID, "note-") {
				item.Kind, item.Title, item.Text = "note", snapshot.Title, snapshot.Body
			}
			method := "read during run"
			if refresh {
				method = "refreshed for this run"
				item.Title = snapshot.Title
				if item.Title == "" {
					item.Title = "Saved knowledge"
				}
				if snapshot.Status != "current" {
					labels := map[string]string{"omitted": "Omitted for this request", "forgotten_or_missing": "No longer saved when this request started", "outside_scope": "Outside this request's goal", "expired": "Expired when this request started", "needs_review": "Saved source or memory needs review", "unavailable": "Current saved state could not be verified", "requires_read": "The complete current record needs an explicit read"}
					label, known := labels[snapshot.Status]
					if !known {
						return errors.New("retained knowledge refresh has an invalid status")
					}
					item.Text = label + ". No current value was supplied by this refresh; older excerpts remain historical."
				}
				if snapshot.Truncated {
					item.Text += "\n[Supplied preview shortened; inspect the complete current record before relying on omitted detail.]"
				}
			} else if !read {
				method, item.Text = "search preview during run", snapshot.Title
				if item.Kind == "memory" {
					item.Title = snapshot.Kind
				}
			}
			if inherited {
				method = strings.ReplaceAll(method, "during run", "carried from "+match[1])
			}
			item.Title += " · " + method
			item.Scope = "not recorded"
			if snapshot.GoalID != nil {
				item.Scope = contextScope(*snapshot.GoalID)
			}
			if refresh && snapshot.Status == "outside_scope" {
				item.Scope = "outside this goal"
			}
			item.Source = strings.Join(append([]string{snapshot.Source}, snapshot.SourceRefs...), ", ")
			item.Source = strings.Trim(item.Source, ", ")
			if snapshot.Origin != nil {
				item.Source = strings.TrimSpace(item.Source + " · Chat: " + snapshot.Origin.ConversationID + " · " + snapshot.Origin.Basis)
			}
			item.Updated = method
			if snapshot.Updated != "" {
				item.Updated += " · saved version " + snapshot.Updated
			}
			item.Updated += " · captured " + evidence.RetrievedAt.Local().Format("Jan 2 15:04:05")
			item.EvidenceID = evidence.ID
			result.Items = append(result.Items, item)
		}
	}
	return nil
}
