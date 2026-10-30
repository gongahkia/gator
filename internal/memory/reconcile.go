package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gongahkia/gator/internal/retrieval"
)

// Reconciliation chooses an inspected record as the surviving representation.
// Every affected saved record requires its exact revision. Subject aliases can
// bind novel wording to that concept without changing the user's saved words.
// Relation inference and permission/review are the caller's separate duties.
type Reconciliation struct {
	KeepID    string            `json:"keep_id"`
	Revisions map[string]string `json:"revisions"`
	Aliases   []string          `json:"aliases,omitempty"`
}

type ReconciliationReceipt struct {
	ID         string            `json:"id"`
	Status     string            `json:"status"`
	KeepID     string            `json:"keep_id"`
	Revisions  map[string]string `json:"revisions"`
	Bindings   []string          `json:"bindings"`
	PreparedAt time.Time         `json:"prepared_at"`
	AppliedAt  time.Time         `json:"applied_at,omitzero"`
}

func (s Store) Reconcile(input Reconciliation) (Record, error) {
	if len(input.Revisions) < 1 || len(input.Revisions) > 16 || len(input.Aliases) > 16 || input.Revisions[input.KeepID] == "" || len(input.Revisions) == 1 && len(input.Aliases) == 0 {
		return Record{}, errors.New("reconciliation needs a kept record and 1–16 inspected revisions, plus another record or alias")
	}
	release, err := s.lock()
	if err != nil {
		return Record{}, err
	}
	defer release()
	bindings, err := s.bindings()
	if err != nil {
		return Record{}, err
	}
	kept, err := s.loadUnlocked(input.KeepID)
	if err != nil {
		return Record{}, err
	}
	if kept.Subject == "" {
		return Record{}, errors.New("reconciliation requires an identified memory subject")
	}
	selected := map[string]Record{}
	roots := map[string]bool{bindings.resolve(kept.GoalID, kept.Subject): true}
	for id, revision := range input.Revisions {
		record, err := s.loadUnlocked(id)
		if err != nil {
			return Record{}, err
		}
		if record.GoalID != kept.GoalID || record.Subject == "" {
			return Record{}, errors.New("reconciliation cannot cross memory scopes or consume unidentified subjects")
		}
		if record.UpdatedAt.Format(time.RFC3339Nano) != revision {
			return Record{}, errors.New("memory changed after review; inspect all reconciliation inputs again")
		}
		blocked, err := s.blockedConcept(bindings, record.GoalID, record.Subject)
		if err != nil {
			return Record{}, err
		}
		if blocked {
			return Record{}, errors.New("reconciliation cannot restore forgotten information")
		}
		selected[id] = record
		roots[bindings.resolve(record.GoalID, record.Subject)] = true
	}
	for _, alias := range input.Aliases {
		if !subjectPattern.MatchString(alias) {
			return Record{}, errors.New("invalid reconciliation alias")
		}
		blocked, err := s.blockedConcept(bindings, kept.GoalID, alias)
		if err != nil {
			return Record{}, err
		}
		if blocked {
			return Record{}, errors.New("reconciliation alias belongs to a forgotten concept")
		}
		roots[bindings.resolve(kept.GoalID, alias)] = true
	}
	records, err := s.listUnlocked(kept.GoalID, true)
	if err != nil {
		return Record{}, err
	}
	for _, record := range records {
		if record.GoalID == kept.GoalID && roots[bindings.resolve(record.GoalID, record.Subject)] {
			if _, reviewed := selected[record.ID]; !reviewed {
				return Record{}, fmt.Errorf("inspect related memory %s before reconciling this concept", record.ID)
			}
		}
	}
	target := bindings.resolve(kept.GoalID, kept.Subject)
	for alias, root := range bindings {
		if roots[root] {
			bindings[alias] = target
		}
	}
	bindings[target] = target
	for _, record := range selected {
		bindings[conceptKey(record.GoalID, record.Subject)] = target
	}
	for _, alias := range input.Aliases {
		bindings[conceptKey(kept.GoalID, alias)] = target
	}
	if len(bindings) > maxConceptBindings {
		return Record{}, errors.New("concept registry reached its bound; no reconciliation was published")
	}
	keys := []string{}
	for alias, root := range bindings {
		if root == target {
			keys = append(keys, alias)
		}
	}
	sort.Strings(keys)
	intent, _ := json.Marshal(struct {
		KeepID    string
		Revisions map[string]string
		Bindings  []string
	}{kept.ID, input.Revisions, keys})
	digest := sha256.Sum256(intent)
	receipt := ReconciliationReceipt{ID: hex.EncodeToString(digest[:]), Status: "prepared", KeepID: kept.ID, Revisions: input.Revisions, Bindings: keys, PreparedAt: s.clock()}
	if err := privateDirectory(filepath.Join(s.root, ".reconciliations")); err != nil {
		return Record{}, err
	}
	receiptPath := ".reconciliations/" + receipt.ID + ".json"
	if err := s.publish(receiptPath, receipt, 32*1024, false); err != nil {
		return Record{}, err
	}
	// Copy source associations before binding/deletion. An interruption can
	// conservatively retain extra references, but cannot lose the forget path.
	for _, record := range selected {
		hashes, err := s.historyOrigins(record)
		if err != nil {
			return Record{}, err
		}
		if len(hashes) != 0 {
			if err := privateDirectory(filepath.Join(s.root, ".origins")); err != nil {
				return Record{}, err
			}
			for hash := range hashes {
				if err := blockMarker(filepath.Join(s.originDirectory(kept), hash)); err != nil {
					return Record{}, err
				}
			}
		}
	}
	// Publish common identity first. Any interrupted remaining duplicates are
	// quarantined by normal recall rather than independently authoritative.
	if err := s.publish(".concepts", bindings, 1<<20, false); err != nil {
		return Record{}, err
	}
	kept.UpdatedAt = s.nextUpdate(kept.UpdatedAt)
	// Alias reconciliation is not new factual confirmation; preserve evidence
	// dates, expiry, source fingerprint and the kept text verbatim.
	if err := s.write(kept, false); err != nil {
		return Record{}, err
	}
	for id := range selected {
		if id == kept.ID {
			continue
		}
		if err := os.Remove(s.path(id)); err != nil {
			return Record{}, err
		}
		if err := retrieval.DropID(filepath.Dir(filepath.Dir(s.root)), id); err != nil {
			return Record{}, err
		}
	}
	receipt.Status, receipt.AppliedAt = "applied", s.clock()
	if err := s.publish(receiptPath, receipt, 32*1024, false); err != nil {
		return Record{}, err
	}
	kept.ConceptID = target
	return kept, nil
}
