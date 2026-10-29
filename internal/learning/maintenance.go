package learning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maintenanceScanLimit = 512
const maintenanceProposalLimit = 3

type MaintenanceReport struct {
	NextOffset   int       `json:"next_offset"`
	ScanComplete bool      `json:"scan_complete"`
	Scanned      int       `json:"scanned"`
	Invalid      int       `json:"invalid"`
	Truncated    bool      `json:"truncated"`
	Candidates   []string  `json:"candidates"`
	Existing     int       `json:"existing"`
	FinishedAt   time.Time `json:"finished_at"`
	Error        string    `json:"error,omitempty"`
}

// Maintain proposes reviewable process guidance from repeated observed outcomes,
// never a causal diagnosis. No model, network request or external action runs.
// A deterministic identity prevents duplicate proposals across restarts and
// preserves an existing rejection, disable or approval without modifying it.
// Each pass resumes its bounded scan and retains source IDs across pages. A
// completed sweep resets the cursor to catch directory changes on the next pass.
func (s Store) Maintain(ctx context.Context) (report MaintenanceReport, finalErr error) {
	report.Candidates = []string{}
	release, err := s.maintenanceLock(ctx)
	if err != nil {
		return report, err
	}
	defer release()
	defer func() {
		report.FinishedAt = s.clock()().UTC()
		if finalErr != nil {
			report.Error = finalErr.Error()
		}
		if err := writeAtomic(filepath.Join(s.root, ".maintenance"), report); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
	}()
	previous, err := s.MaintenanceStatus()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return report, err
	}
	if previous.NextOffset < 0 {
		return report, errors.New("invalid memory maintenance cursor")
	}
	report.NextOffset = previous.NextOffset
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
	entries, err := maintenanceWindow(ctx, directory, report.NextOffset)
	if err != nil {
		return report, err
	}
	hasMore := len(entries) > maintenanceScanLimit
	if hasMore {
		entries = entries[:maintenanceScanLimit]
		report.Truncated = true
	}
	existing := map[string]bool{}
	now := s.clock()().UTC()
	for index, entry := range entries {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		// Only advance past an entry after its processing has completed. A
		// failed/cancelled pass can safely retry its unfinished observation.
		report.NextOffset = previous.NextOffset + index
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		report.Scanned++
		observation, err := s.LoadObservation(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			report.Invalid++
			continue
		}
		if !eligibleObservation(observation, now) {
			continue
		}
		key, scope := observationGroup(observation)
		observations, err := s.accumulateMaintenance(observation, now)
		if err != nil {
			return report, err
		}
		if len(observations) < 3 {
			continue
		}
		digest := sha256.Sum256([]byte("failure-review-v1\x00" + key))
		id := "learning-" + hex.EncodeToString(digest[:12])
		if _, err := s.Load(id); err == nil {
			if !existing[id] {
				report.Existing++
				existing[id] = true
			}
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return report, err
		}
		if len(report.Candidates) == maintenanceProposalLimit {
			report.Truncated = true
			continue
		}
		workIDs, refs := []string{}, []string{}
		for _, current := range observations {
			workIDs = append(workIDs, current.WorkID)
			refs = append(refs, s.observationReference(current.ID))
		}
		_, err = s.Create(Create{ID: id, Type: FailurePrevention, Key: "review-" + string(observation.Signal), Content: failureGuidance(observation.Signal), Scope: scope, Origin: Inferred, Provenance: Provenance{WorkIDs: workIDs, EvidenceRefs: refs}})
		if errors.Is(err, os.ErrExist) {
			if !existing[id] {
				report.Existing++
				existing[id] = true
			}
			continue
		}
		if err != nil {
			return report, err
		}
		report.Candidates = append(report.Candidates, id)
	}
	report.NextOffset = previous.NextOffset + len(entries)
	if !hasMore {
		report.NextOffset = 0
		report.ScanComplete = true
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
