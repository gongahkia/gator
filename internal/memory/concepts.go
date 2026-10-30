package memory

import (
	"errors"
	"os"
	"path/filepath"
)

// These are deliberately narrow, documented label aliases. They do not claim
// to resolve arbitrary paraphrases, infer ownership or merge unrelated facts.
var conceptAliases = map[string]string{
	"preference.length":       "preference.response_length",
	"preference.reply_length": "preference.response_length",
	"profile.home_city":       "profile.city",
	"profile.residence_city":  "profile.city",
}

func CanonicalSubject(subject string) string {
	if canonical, ok := conceptAliases[subject]; ok {
		return canonical
	}
	return subject
}

// Old tombstones used the model's literal label. Check those as well as the
// canonical label so upgrading cannot resurrect previously forgotten memory.
func (s Store) blockedSubject(goalID, subject string) (bool, error) {
	bindings, err := s.bindings()
	if err != nil {
		return false, err
	}
	return s.blockedConcept(bindings, goalID, subject)
}

func (s Store) unblockSubject(goalID, subject string) error {
	if err := s.restoreWithdrawal(goalID, subject); err != nil {
		return err
	}
	bindings, err := s.bindings()
	if err != nil {
		return err
	}
	root := bindings.resolve(goalID, subject)
	keys := map[string]bool{root: true, conceptKey(goalID, subject): true}
	for alias, target := range conceptAliases {
		if bindings.resolve(goalID, target) == root {
			keys[historyHash(goalID, alias)] = true
		}
	}
	for alias, target := range bindings {
		if target == root {
			keys[alias] = true
		}
	}
	for key := range keys {
		if err := os.Remove(filepath.Join(s.root, ".forgotten", key)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (s Store) annotateConcepts(records []Record) error {
	bindings, err := s.bindings()
	if err != nil {
		return err
	}
	groups := map[string][]int{}
	for i, record := range records {
		if record.Subject == "" {
			continue
		}
		blocked, err := s.blockedConcept(bindings, record.GoalID, record.Subject)
		if err != nil {
			return err
		}
		if blocked {
			records[i].RecallIssue = "This concept was forgotten; excluded from automatic recall."
		}
		if !record.ExpiresAt.IsZero() && !record.ExpiresAt.After(s.clock()) {
			continue
		}
		key := bindings.resolve(record.GoalID, record.Subject)
		records[i].ConceptID = key
		groups[key] = append(groups[key], i)
	}
	for _, indices := range groups {
		if len(indices) < 2 {
			continue
		}
		for _, i := range indices {
			if records[i].RecallIssue == "" {
				records[i].RecallIssue = "Multiple records share this concept; reconcile them before automatic recall."
			}
		}
	}
	return nil
}

func (r Record) Recallable() bool {
	return r.RecallIssue == "" && r.SourceStatus != "changed" && r.SourceStatus != "unavailable"
}

// Inspect includes current concept annotations for a direct ID lookup. Load is
// the storage primitive used inside mutations; inspecting never rewrites files.
func (s Store) Inspect(id string) (Record, error) {
	record, err := s.Load(id)
	if err != nil {
		return Record{}, err
	}
	records, err := s.List(record.GoalID, true)
	if err != nil && ReadWarning(err) == "" {
		return Record{}, err
	}
	for _, current := range records {
		if current.ID == id {
			return current, nil
		}
	}
	return Record{}, os.ErrNotExist
}
