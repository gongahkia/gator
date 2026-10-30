package memory

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// WithdrawalRule retains an abstract topic, never the forgotten fact value.
// Custom topics must come from an explicit owning-control review. Empty topics
// are visible migration gaps rather than evidence that old history is safe.
type WithdrawalRule struct {
	ScopeUnknown bool      `json:"scope_unknown,omitempty"`
	Version      int       `json:"version"`
	ID           string    `json:"id"`
	Scope        string    `json:"scope,omitempty"`
	Topic        string    `json:"topic,omitempty"`
	ForgottenAt  time.Time `json:"forgotten_at"`
	RestoredAt   time.Time `json:"restored_at,omitzero"`
	Derived      bool      `json:"-"`
}

var subjectTopics = map[string]string{
	"profile.name":               "the user's name or identity name",
	"profile.pronouns":           "the user's pronouns",
	"profile.city":               "the user's home city or place of residence",
	"profile.timezone":           "the user's time zone",
	"profile.language":           "languages the user speaks",
	"profile.job":                "the user's job or occupation",
	"profile.role":               "the user's work role",
	"profile.employer":           "the user's employer",
	"profile.workplace":          "the user's workplace",
	"preference.response_length": "the user's preferred response length or level of detail",
	"preference.language":        "the user's preferred response language",
	"preference.format":          "the user's preferred response formatting",
	"preference.tone":            "the user's preferred response tone",
	"preference.units":           "the user's preferred measurement units",
	"preference.accessibility":   "the user's accessibility preferences",
}

func withdrawalKey(bindings conceptBindings, record Record) string {
	if record.Subject == "" {
		return historyHash(record.GoalID, "record:"+record.ID)
	}
	return bindings.resolve(record.GoalID, record.Subject)
}

func topicForConcept(bindings conceptBindings, scope, id string) string {
	var topics []string
	for subject, topic := range subjectTopics {
		if bindings.resolve(scope, subject) == id {
			topics = append(topics, topic)
		}
	}
	sort.Strings(topics)
	return strings.Join(topics, "; ")
}

func (s Store) loadWithdrawal(id string) (WithdrawalRule, error) {
	if !validConceptKey(id) {
		return WithdrawalRule{}, errors.New("invalid withdrawal identity")
	}
	path := filepath.Join(s.root, ".withdrawals", id+".json")
	info, err := os.Lstat(path)
	if err != nil {
		return WithdrawalRule{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4096 {
		return WithdrawalRule{}, errors.New("withdrawal must be a bounded ordinary file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return WithdrawalRule{}, err
	}
	var rule WithdrawalRule
	if err := json.Unmarshal(data, &rule); err != nil {
		return rule, err
	}
	if rule.Version != 1 || rule.ID != id || rule.ForgottenAt.IsZero() || len(rule.Topic) > 1000 || strings.ContainsRune(rule.Topic, 0) || !rule.RestoredAt.IsZero() && rule.RestoredAt.Before(rule.ForgottenAt) {
		return rule, errors.New("invalid withdrawal rule")
	}
	return rule, nil
}

func (s Store) writeWithdrawal(rule WithdrawalRule) error {
	if err := privateDirectory(filepath.Join(s.root, ".withdrawals")); err != nil {
		return err
	}
	return s.publish(".withdrawals/"+rule.ID+".json", rule, 4096, false)
}

func (s Store) recordWithdrawal(record Record, topic string) error {
	bindings, err := s.bindings()
	if err != nil {
		return err
	}
	id := withdrawalKey(bindings, record)
	old, err := s.loadWithdrawal(id)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if topic == "" {
		topic = old.Topic
	}
	if topic == "" {
		topic = topicForConcept(bindings, record.GoalID, id)
	}
	if len(topic) > 1000 || strings.ContainsRune(topic, 0) || topic != "" && strings.Contains(strings.ToLower(topic), strings.ToLower(record.Text)) {
		return errors.New("withdrawal topic must describe the attribute without repeating the forgotten fact")
	}
	return s.writeWithdrawal(WithdrawalRule{Version: 1, ID: id, Scope: record.GoalID, Topic: topic, ForgottenAt: s.clock()})
}

// WithdrawalRules includes persisted rules and legacy subject markers. Read
// migration is derived in memory; no facts are reconstructed or invented.
func (s Store) WithdrawalRules(scope string) ([]WithdrawalRule, error) {
	release, err := s.readLock()
	if err != nil {
		return nil, err
	}
	defer release()
	return s.withdrawalRulesUnlocked(scope)
}

func (s Store) withdrawalRulesUnlocked(scope string) ([]WithdrawalRule, error) {
	byID := map[string]WithdrawalRule{}
	knownIDs := map[string]bool{}
	entries, err := os.ReadDir(filepath.Join(s.root, ".withdrawals"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil, errors.New("invalid withdrawal directory entry")
		}
		rule, err := s.loadWithdrawal(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		knownIDs[rule.ID] = true
		if rule.Scope == "" || rule.Scope == scope {
			byID[rule.ID] = rule
		}
	}
	bindings, err := s.bindings()
	if err != nil {
		return nil, err
	}
	entries, err = os.ReadDir(filepath.Join(s.root, ".forgotten"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !validConceptKey(entry.Name()) {
			return nil, errors.New("invalid legacy withdrawal marker")
		}
		id := entry.Name()
		if target, ok := bindings[id]; ok {
			id = target
		}
		knownScope, topic, scopeKnown := "", "", false
		for _, candidateScope := range []string{"", scope} {
			for subject := range subjectTopics {
				if conceptKey(candidateScope, subject) == entry.Name() || bindings.resolve(candidateScope, subject) == id {
					knownScope, scopeKnown = candidateScope, true
					id = bindings.resolve(candidateScope, subject)
					topic = topicForConcept(bindings, candidateScope, id)
				}
			}
			for alias, target := range conceptAliases {
				if historyHash(candidateScope, alias) == entry.Name() {
					knownScope, scopeKnown = candidateScope, true
					id = bindings.resolve(candidateScope, target)
					topic = topicForConcept(bindings, candidateScope, id)
				}
			}
		}
		if knownIDs[id] {
			continue
		}
		if _, ok := byID[id]; ok {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		byID[id] = WithdrawalRule{Version: 1, ID: id, Scope: knownScope, Topic: topic, ForgottenAt: info.ModTime().UTC(), Derived: true, ScopeUnknown: !scopeKnown}
	}
	var result []WithdrawalRule
	for _, rule := range byID {
		result = append(result, rule)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (s Store) restoreWithdrawal(scope, subject string) error {
	bindings, err := s.bindings()
	if err != nil {
		return err
	}
	id := bindings.resolve(scope, subject)
	rules, err := s.withdrawalRulesUnlocked(scope)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if rule.ID == id && rule.Scope == scope && rule.RestoredAt.IsZero() {
			rule.RestoredAt = s.clock()
			if rule.RestoredAt.Before(rule.ForgottenAt) {
				rule.RestoredAt = rule.ForgottenAt
			}
			return s.writeWithdrawal(rule)
		}
	}
	return nil
}

func (s Store) ForgetWithTopic(id, updated, topic string) error {
	if updated == "" || strings.TrimSpace(topic) == "" {
		return errors.New("an inspected revision and approved abstract topic are required")
	}
	release, err := s.lock()
	if err != nil {
		return err
	}
	defer release()
	record, err := s.loadUnlocked(id)
	if err != nil {
		return err
	}
	if record.UpdatedAt.Format(time.RFC3339Nano) != updated {
		return errors.New("memory changed during withdrawal review; inspect it again")
	}
	if err := s.recordWithdrawal(record, strings.TrimSpace(topic)); err != nil {
		return err
	}
	return s.forgetRecord(record)
}
