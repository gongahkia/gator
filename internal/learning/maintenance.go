package learning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maintenanceScanLimit = 512
const maintenanceProposalLimit = 3

type MaintenanceReport struct {
	Scanned    int       `json:"scanned"`
	Invalid    int       `json:"invalid"`
	Truncated  bool      `json:"truncated"`
	Candidates []string  `json:"candidates"`
	Existing   int       `json:"existing"`
	FinishedAt time.Time `json:"finished_at"`
	Error      string    `json:"error,omitempty"`
}

// Maintain proposes reviewable process guidance from repeated observed outcomes,
// never a causal diagnosis. No model, network request or external action runs.
// A deterministic identity prevents duplicate proposals across restarts and
// preserves an existing rejection, disable or approval without modifying it.
// The bounded scan reports incomplete coverage instead of implying exhaustive
// review. A later indexed scan can extend coverage without changing authority.
func (s Store) Maintain(ctx context.Context) (report MaintenanceReport, finalErr error) {
	report.Candidates = []string{}
	defer func() {
		report.FinishedAt = s.clock()().UTC()
		if finalErr != nil {
			report.Error = finalErr.Error()
		}
		if err := writeAtomic(filepath.Join(s.root, ".maintenance"), report); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
	}()
	if err := ctx.Err(); err != nil {
		return report, err
	}
	root, err := s.observationsRoot()
	if err != nil {
		return report, err
	}
	directory, err := os.Open(root)
	if err != nil {
		return report, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(maintenanceScanLimit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return report, err
	}
	if len(entries) > maintenanceScanLimit {
		report.Truncated = true
		entries = entries[:maintenanceScanLimit]
	}
	type group struct {
		scope        Scope
		signal       Signal
		observations map[string]Observation
	}
	groups := map[string]*group{}
	now := s.clock()().UTC()
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		report.Scanned++
		observation, err := s.LoadObservation(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			report.Invalid++
			continue
		}
		if observation.CreatedAt.Before(now.AddDate(0, 0, -30)) || observation.CreatedAt.After(now) || len(observation.EvidenceRefs) == 0 {
			continue
		}
		if failureGuidance(observation.Signal) == "" {
			continue
		}
		scope := Scope{Kind: Global}
		if observation.Scope != nil {
			scope = *observation.Scope
		}
		key := string(observation.Signal) + "\x00" + string(scope.Kind) + "\x00" + scope.Value
		if groups[key] == nil {
			groups[key] = &group{scope: scope, signal: observation.Signal, observations: map[string]Observation{}}
		}
		// Multiple retries/events from one Work are one piece of evidence.
		prior, exists := groups[key].observations[observation.WorkID]
		if !exists || observation.ID < prior.ID {
			groups[key].observations[observation.WorkID] = observation
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		g := groups[key]
		if len(g.observations) < 3 {
			continue
		}
		digest := sha256.Sum256([]byte("failure-review-v1\x00" + key))
		id := "learning-" + hex.EncodeToString(digest[:12])
		if _, err := s.Load(id); err == nil {
			report.Existing++
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return report, err
		}
		if len(report.Candidates) == maintenanceProposalLimit {
			report.Truncated = true
			break
		}
		workIDs := make([]string, 0, len(g.observations))
		for workID := range g.observations {
			workIDs = append(workIDs, workID)
		}
		sort.Strings(workIDs)
		if len(workIDs) > 16 {
			workIDs = workIDs[:16]
		}
		refs := make([]string, 0, len(workIDs))
		for _, workID := range workIDs {
			refs = append(refs, s.observationReference(g.observations[workID].ID))
		}
		_, err := s.Create(Create{ID: id, Type: FailurePrevention, Key: "review-" + string(g.signal), Content: failureGuidance(g.signal), Scope: g.scope, Origin: Inferred, Provenance: Provenance{WorkIDs: workIDs, EvidenceRefs: refs}})
		if errors.Is(err, os.ErrExist) {
			report.Existing++
			continue
		}
		if err != nil {
			return report, err
		}
		report.Candidates = append(report.Candidates, id)
	}
	return report, nil
}

func failureGuidance(signal Signal) string {
	switch signal {
	case WorkFailed:
		return "Before retrying incomplete work, inspect the retained failure evidence and agree on a bounded verification step. Several independent runs failed; a shared root cause has not been established."
	case VerificationFailed:
		return "Review the required verification checks before starting similar work, and report failed checks explicitly before claiming completion. Several independent runs failed required verification; inspect the linked evidence before adopting this guidance."
	case DeliveryUnknown, ExternalOutcomeUnknown:
		return "When an external outcome is unknown, reconcile the destination and retained attempt evidence before retrying. Several independent runs had unknown outcomes; this proposal does not authorize a retry or external action."
	case DeliveryFailed:
		return "Check the selected delivery destination and connection before repeating delivery, using retained attempt evidence. Several independent deliveries failed; a shared root cause has not been established."
	}
	return ""
}

func (s Store) MaintenanceStatus() (MaintenanceReport, error) {
	var report MaintenanceReport
	err := readJSONRecord(filepath.Join(s.root, ".maintenance"), &report)
	return report, err
}

func ConfidenceDescription(value int) string {
	if value == 0 {
		return "not estimated"
	}
	return "uncalibrated score " + fmt.Sprint(value)
}
