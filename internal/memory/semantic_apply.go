package memory

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ApplySemanticChat applies a host-held plan, not model-supplied tool arguments.
// Equivalence can refresh a user's statement. Contradictions and consolidation
// require the separate inspected revision guard or an explicit host review.
func (s Store) ApplySemanticChat(ctx context.Context, input ChatInput, plan SemanticPlan, reviewed bool) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if err := validateChatInput(input); err != nil {
		return Record{}, err
	}
	if plan.StatementSHA256 != textDigest(input.Quote) || plan.Snapshot.Scope != input.GoalID || plan.Snapshot.Token == "" || !plan.Snapshot.Complete {
		return Record{}, errors.New("semantic plan does not describe this current statement and scope")
	}
	release, err := s.lock()
	if err != nil {
		return Record{}, err
	}
	defer release()
	current, err := s.semanticSnapshotUnlocked(input.GoalID, input.ExcludedIDs)
	if err != nil {
		return Record{}, err
	}
	if !current.Complete || current.Token != plan.Snapshot.Token {
		return Record{}, errors.New("saved memory changed during semantic review; analyze the current state again")
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if plan.Relation == Withdrawn {
		return s.restoreSemanticChatUnlocked(input, plan, current)
	}
	if plan.Relation == Distinct {
		if len(plan.MatchIDs) != 0 || input.ReplaceID != "" {
			return Record{}, errors.New("a distinct memory cannot replace an existing record")
		}
		if !subjectPattern.MatchString(plan.SuggestedSubject) {
			return Record{}, errors.New("invalid proposed semantic category")
		}
		input.Subject = CanonicalSubject(plan.SuggestedSubject)
		return s.rememberChatUnlocked(input)
	}
	if plan.Relation != Equivalent && plan.Relation != Contradictory {
		return Record{}, errors.New("semantic meaning remains uncertain; inspect and reconcile the proposed memory")
	}
	if len(plan.MatchIDs) == 0 || len(plan.MatchIDs) > 16 {
		return Record{}, errors.New("semantic plan has no bounded matching record set")
	}
	versions := map[string]string{}
	var target Record
	for _, id := range plan.MatchIDs {
		if _, duplicate := versions[id]; duplicate {
			return Record{}, errors.New("repeated semantic match")
		}
		if slices.Contains(input.ExcludedIDs, id) {
			return Record{}, errors.New("semantic match is excluded from this run")
		}
		record, err := s.loadUnlocked(id)
		if err != nil {
			return Record{}, err
		}
		found := false
		for _, candidate := range current.Candidates {
			found = found || candidate.ID == id
		}
		if !found || record.GoalID != input.GoalID || record.Kind != input.Kind || record.Subject == "" {
			return Record{}, errors.New("semantic match requires manual scope or kind reconciliation")
		}
		versions[id] = record.UpdatedAt.Format(time.RFC3339Nano)
		if target.ID == "" || input.ReplaceID == id {
			target = record
		}
	}
	if input.ReplaceID != "" && (versions[input.ReplaceID] == "" || versions[input.ReplaceID] != input.ExpectedUpdated) {
		return Record{}, errors.New("inspected semantic replacement changed; inspect it again")
	}
	if len(versions) > 1 && !reviewed {
		return Record{}, errors.New("multiple semantic matches need an exact consolidation review")
	}
	if plan.Relation == Contradictory && !reviewed && (input.ReplaceID != target.ID || input.ExpectedUpdated != versions[target.ID]) {
		return Record{}, fmt.Errorf("semantic conflict: inspect %s (updated %s) before replacing it", target.ID, versions[target.ID])
	}
	if target.SourceStatus == "changed" || target.SourceStatus == "unavailable" || !target.ExpiresAt.IsZero() && !target.ExpiresAt.After(s.clock()) {
		return Record{}, errors.New("semantic match has an expired or changed source; review its owning memory controls")
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	// Bind the proposed label only after every applicable guard has passed.
	// The original saved label remains stable; model-proposed labels cannot
	// redirect the update into an unrelated existing category.
	bindings, err := s.bindings()
	if err != nil {
		return Record{}, err
	}
	if len(versions) > 1 || bindings.resolve(input.GoalID, input.Subject) != bindings.resolve(target.GoalID, target.Subject) {
		target, err = s.reconcileUnlocked(Reconciliation{KeepID: target.ID, Revisions: versions, Aliases: []string{input.Subject}})
		if err != nil {
			return Record{}, err
		}
	}
	input.Subject, input.ReplaceID, input.ExpectedUpdated = target.Subject, target.ID, target.UpdatedAt.Format(time.RFC3339Nano)
	return s.rememberChatUnlocked(input)
}

func (s Store) restoreSemanticChatUnlocked(input ChatInput, plan SemanticPlan, current SemanticSnapshot) (Record, error) {
	if !input.Explicit {
		return Record{}, errors.New("this semantic topic was forgotten; only an explicit new request to remember it may restore it")
	}
	if len(plan.MatchIDs) != 1 || input.ReplaceID != "" {
		return Record{}, errors.New("restore one withdrawn concept at a time after reviewing any remaining records")
	}
	id := strings.TrimPrefix(plan.MatchIDs[0], "withdrawal-")
	if !validConceptKey(id) || id == plan.MatchIDs[0] {
		return Record{}, errors.New("invalid semantic withdrawal reference")
	}
	rules, err := s.withdrawalRulesUnlocked(input.GoalID)
	if err != nil {
		return Record{}, err
	}
	var rule WithdrawalRule
	for _, candidate := range rules {
		if candidate.ID == id {
			rule = candidate
		}
	}
	if rule.ID == "" || rule.Scope != input.GoalID || rule.ScopeUnknown || rule.Topic == "" || !rule.RestoredAt.IsZero() {
		return Record{}, errors.New("withdrawal needs current scope/topic review before restoration")
	}
	if !subjectPattern.MatchString(plan.SuggestedSubject) {
		return Record{}, errors.New("invalid restored category")
	}
	bindings, err := s.bindings()
	if err != nil {
		return Record{}, err
	}
	remaining, err := s.listUnlocked(input.GoalID, true)
	if err != nil {
		return Record{}, err
	}
	for _, record := range remaining {
		if record.GoalID == input.GoalID && record.Subject != "" && bindings.resolve(record.GoalID, record.Subject) == rule.ID {
			return Record{}, errors.New("review records left by an interrupted withdrawal before restoring this topic")
		}
	}
	aliases := []string{input.Subject, CanonicalSubject(plan.SuggestedSubject)}
	for _, alias := range aliases {
		root := bindings.resolve(input.GoalID, alias)
		blocked, err := s.blockedConcept(bindings, input.GoalID, alias)
		if err != nil {
			return Record{}, err
		}
		if blocked && root != rule.ID {
			return Record{}, errors.New("restoration would lift another withdrawn concept")
		}
		for _, candidate := range current.Candidates {
			if candidate.Kind != "withdrawn" && (bindings.resolve(input.GoalID, candidate.Subject) == root || bindings.resolve(input.GoalID, candidate.Subject) == rule.ID) {
				return Record{}, errors.New("inspect remaining saved records before restoring this semantic topic")
			}
		}
		bindings[conceptKey(input.GoalID, alias)] = rule.ID
	}
	bindings[rule.ID] = rule.ID
	if len(bindings) > maxConceptBindings {
		return Record{}, errors.New("concept registry reached its bound")
	}
	if err := s.writeWithdrawal(rule); err != nil {
		return Record{}, err
	}
	if err := s.publish(".concepts", bindings, 1<<20, false); err != nil {
		return Record{}, err
	}
	input.Subject = CanonicalSubject(plan.SuggestedSubject)
	return s.rememberChatUnlocked(input)
}
