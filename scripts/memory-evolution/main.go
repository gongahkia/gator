// Command memory-evolution probes current memory behavior over synthetic changes.
// It does not call a model, inspect the live profile or change production behavior.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/tools"
)

type finding struct {
	Capability string `json:"capability"`
	Outcome    string `json:"outcome"`
	Evidence   any    `json:"evidence"`
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
func require(err error) {
	if err != nil {
		panic(err)
	}
}
func input(subject, quote string) memory.ChatInput {
	return memory.ChatInput{Subject: subject, Kind: "preference", Quote: quote, ConversationID: "work-first", RunID: "run-first"}
}
func contains(records []memory.Record, id string) bool {
	for _, record := range records {
		if record.ID == id {
			return true
		}
	}
	return false
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/memory-evolution NEW_REPORT_JSON")
		os.Exit(2)
	}
	output := must(os.OpenFile(os.Args[1], os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600))
	defer output.Close()
	root := must(os.MkdirTemp("", "gator-memory-evolution-"))
	defer os.RemoveAll(root)
	var findings []finding
	add := func(name string, met bool, evidence any) {
		outcome := "met"
		if !met {
			outcome = "gap"
		}
		findings = append(findings, finding{name, outcome, evidence})
	}
	newStore := func(name string) memory.Store { return must(memory.Open(filepath.Join(root, name))) }
	// Exercise the public host tool: an exact quote alone is not a suitability test.
	for _, item := range []struct {
		name, message, quote string
		automatic, wantSave  bool
	}{
		{"passive exact-quote save", "I prefer brief answers.", "I prefer brief answers.", true, true},
		{"automatic learning opt-out", "I prefer brief answers.", "I prefer brief answers.", false, false},
		{"negative save instruction", "Don't save this: I prefer brief answers.", "I prefer brief answers.", true, false},
		{"secret rejected by host", "My password is SYNTHETIC_NOT_A_REAL_SECRET_123!", "My password is SYNTHETIC_NOT_A_REAL_SECRET_123!", true, false},
		{"third-party statement rejected by host", "My colleague said: I prefer brief answers.", "I prefer brief answers.", true, false},
		{"temporary request rejected by host", "For this reply only, use bullet points.", "For this reply only, use bullet points.", true, false},
	} {
		store := newStore(fmt.Sprint(len(findings)))
		tool := (tools.ChatMemory{Store: store, UserMessage: item.message, ConversationID: "work-probe", RunID: "run-probe", Automatic: item.automatic}).Tools()[0]
		arguments := must(json.Marshal(map[string]any{"subject": "preference.response", "kind": "preference", "quote": item.quote, "scope": "personal"}))
		_, err := tool.Execute(context.Background(), arguments)
		records := must(store.List("", false))
		saved := err == nil && len(records) == 1
		add(item.name, saved == item.wantSave, map[string]any{"saved": saved, "host_rejected": err != nil})
	}
	store := newStore("evolution")
	original := must(store.RememberChat(input("preference.length", "I prefer short replies.")))
	change := input("preference.length", "I now prefer detailed replies.")
	_, err := store.RememberChat(change)
	add("conflicting replacement requires current revision", err != nil, map[string]bool{"rejected": err != nil})
	change.ReplaceID, change.ExpectedUpdated = original.ID, original.UpdatedAt.Format(time.RFC3339Nano)
	updated := must(store.RememberChat(change))
	reopened := must(memory.Open(filepath.Join(root, "evolution")))
	current := must(reopened.Load(original.ID))
	add("reviewed correction survives reopen", current.Text == change.Quote && current.ID == original.ID, map[string]any{"same_id": current.ID == original.ID, "record_count": len(must(reopened.List("", false))), "updated_after_original": current.UpdatedAt.After(original.UpdatedAt)})
	change.Quote = "I prefer medium replies."
	_, err = store.RememberChat(change)
	add("old revision cannot overwrite newer correction", err != nil, map[string]bool{"rejected": err != nil})
	// Age only this disposable fixture's canonical record, then confirm the exact
	// same words in a newer conversation. No wall-clock or live profile is changed.
	old := updated
	old.CreatedAt = time.Now().AddDate(-1, 0, 0)
	old.UpdatedAt = old.CreatedAt
	old.LastObservedAt, old.LastConfirmedAt = old.CreatedAt, old.CreatedAt
	for i := range old.Evidence {
		old.Evidence[i].ObservedAt = old.CreatedAt
	}
	require(os.WriteFile(filepath.Join(root, "evolution/gator/memory", old.ID+".json"), must(json.Marshal(old)), 0600))
	for index := 0; index < 4; index++ {
		must(store.RememberChat(input(fmt.Sprintf("preference.other%d", index), fmt.Sprintf("I prefer option %d for a separate topic.", index))))
	}
	confirmation := input(old.Subject, old.Text)
	confirmation.ConversationID = "work-later"
	confirmation.RunID = "run-later"
	confirmed := must(store.RememberChat(confirmation))
	profile := must(store.Profile(nil))
	add("reconfirmation refreshes evidence and profile priority", confirmed.UpdatedAt.After(old.UpdatedAt) && contains(profile, old.ID), map[string]any{"timestamp_refreshed": confirmed.UpdatedAt.After(old.UpdatedAt), "latest_origin_retained": confirmed.Origin.ConversationID == "work-later", "in_four_item_profile": contains(profile, old.ID), "profile_size": len(profile), "chat_memory_has_expiry": !confirmed.ExpiresAt.IsZero()})
	alias := input("preference.response_length", "I prefer short replies.")
	_, err = store.RememberChat(alias)
	add("contradictory alias requires reconciliation", err != nil, map[string]bool{"alias_saved_without_conflict": err == nil})
	forgetStore := newStore("forget")
	city := input("profile.city", "I live in Harbor City.")
	city.Kind = "fact"
	cityRecord := must(forgetStore.RememberChat(city))
	require(forgetStore.Forget(cityRecord.ID))
	_, err = forgetStore.RememberChat(city)
	add("forgotten exact subject stays suppressed", err != nil, map[string]bool{"rejected": err != nil})
	city.Subject = "profile.home_city"
	_, err = forgetStore.RememberChat(city)
	add("forgotten meaning stays suppressed across aliases", err != nil, map[string]bool{"alias_relearned": err == nil})
	scope := newStore("scope")
	goal := input("preference.project", "Use Project Alpha terminology.")
	goal.GoalID = "goal-0123456789abcdef01234567"
	goalRecord := must(scope.RememberChat(goal))
	add("goal facts stay out of personal and other-goal memory", !contains(must(scope.List("", false)), goalRecord.ID) && !contains(must(scope.List("goal-1123456789abcdef01234567", false)), goalRecord.ID), map[string]bool{"own_goal_visible": contains(must(scope.List(goal.GoalID, false)), goalRecord.ID)})
	expired := must(scope.Add("fact", "The temporary venue is Harbor Hall.", "", "Synthetic fixture", time.Now().Add(-time.Hour)))
	add("explicit expiry excludes stale facts", !contains(must(scope.List("", false)), expired.ID), map[string]bool{"retained_for_inspection": contains(must(scope.List("", true)), expired.ID)})
	file := filepath.Join(root, "source.txt")
	require(os.WriteFile(file, []byte("Capacity: 10"), 0600))
	source := must(scope.AddFromFile("fact", "Harbor capacity is 10.", "", file, time.Time{}))
	require(os.WriteFile(file, []byte("Capacity: 20"), 0600))
	add("changed file source is excluded from recall", !contains(must(scope.Search("Harbor capacity", "", 8)), source.ID), map[string]string{"source_status": must(scope.Load(source.ID)).SourceStatus})
	// A corrupt independent record should not make unrelated healthy memory
	// unavailable; current behavior is recorded as a gap, never silently accepted.
	broken := newStore("corrupt")
	healthy := must(broken.Add("fact", "Harbor is the current project.", "", "fixture", time.Time{}))
	bad := must(broken.Add("fact", "Another fixture fact.", "", "fixture", time.Time{}))
	require(os.WriteFile(filepath.Join(root, "corrupt/gator/memory", bad.ID+".json"), []byte("{"), 0600))
	hits, searchErr := broken.Search("Harbor", "", 8)
	add("one corrupt record preserves unrelated healthy recall", (searchErr == nil || memory.ReadWarning(searchErr) != "") && contains(hits, healthy.ID), map[string]bool{"search_failed": searchErr != nil && memory.ReadWarning(searchErr) == "", "partial_read_warning": memory.ReadWarning(searchErr) != "", "healthy_record_recalled": contains(hits, healthy.ID)})
	learnings := must(learning.Open(filepath.Join(root, "learning")))
	for index := 0; index < 3; index++ {
		reference := fmt.Sprintf("fixture-failure-%d.txt", index)
		require(os.WriteFile(filepath.Join(root, "learning", reference), []byte("Synthetic failed verification evidence."), 0600))
		must(learnings.RecordObservation(learning.ObservationInput{Signal: learning.WorkFailed, WorkID: fmt.Sprintf("work-failure-%d", index), Summary: "Synthetic repeated verification failure.", EvidenceRefs: []string{reference}}))
	}
	must(learnings.Maintain(context.Background()))
	add("repeated failure observations produce a reviewable candidate", len(must(learnings.List())) > 0, map[string]any{"observations_recorded": 3, "candidates_generated": len(must(learnings.List()))})
	observation := must(learnings.RecordObservation(learning.ObservationInput{Signal: learning.UserCorrected, WorkID: "work-correction", Summary: "Use focused checks first."}))
	proposal := learning.Proposal{Observation: observation, Type: learning.FailurePrevention, Key: "focused-checks", Content: "Run focused checks before broad suites.", Scope: learning.Scope{Kind: learning.Global}}
	candidate := must(learnings.Propose(proposal))
	add("inferred lesson awaits confirmation", candidate.Status == learning.Candidate && len(must(learnings.Projection(learning.Context{}))) == 0, map[string]any{"status": candidate.Status, "stored_confidence": candidate.Confidence})
	proposal.Observation = must(learnings.RecordObservation(learning.ObservationInput{Signal: learning.UserCorrected, WorkID: "work-correction-two", Summary: "Use focused checks first."}))
	merged := must(learnings.Propose(proposal))
	add("repeated identical correction consolidates evidence", merged.ID == candidate.ID && len(merged.Provenance.WorkIDs) == 2, map[string]any{"same_id": merged.ID == candidate.ID, "evidence_work_count": len(merged.Provenance.WorkIDs)})
	must(learnings.Enable(candidate.ID))
	add("approved lesson enters future projection", len(must(learnings.Projection(learning.Context{}))) == 1, map[string]int{"active": len(must(learnings.Projection(learning.Context{})))})
	must(learnings.Disable(candidate.ID))
	add("withdrawn lesson stops future projection", len(must(learnings.Projection(learning.Context{}))) == 0, map[string]int{"retained_records": len(must(learnings.List()))})
	met := 0
	for _, finding := range findings {
		if finding.Outcome == "met" {
			met++
		}
	}
	report := map[string]any{"scope": "Deterministic synthetic host/store capability probes, not live-model extraction accuracy or a memory quality score. Gaps are retained as findings; this command does not repair them.", "probes": findings, "met": met, "gaps": len(findings) - met}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	require(encoder.Encode(report))
	fmt.Printf("%d capability checks met; %d gaps observed. Report: %s\n", met, len(findings)-met, os.Args[1])
}
