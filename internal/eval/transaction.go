package eval

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/delivery"
	"github.com/gongahkia/gator/internal/snapshot"
	"github.com/gongahkia/gator/internal/workhistory"
)

// TransactionDimension is one independently inspectable property of Work.
// State is intentionally descriptive rather than an aggregate score: a
// verified result that has not been delivered is useful and distinct from a
// failed Work execution.
type TransactionDimension struct {
	State    string `json:"state"`
	Evidence string `json:"evidence,omitempty"`
	Error    string `json:"error,omitempty"`
}

// TransactionFidelity is the deterministic projection of canonical Work
// records. It contains only grading results and paths/hashes already retained
// by Work; it never copies prompts, artifact contents, or action payloads.
type TransactionFidelity struct {
	Work            TransactionDimension `json:"work"`
	History         TransactionDimension `json:"history"`
	Verification    TransactionDimension `json:"verification"`
	Delivery        TransactionDimension `json:"delivery"`
	Recovery        TransactionDimension `json:"recovery"`
	ExternalActions TransactionDimension `json:"external_actions"`
}

const (
	transactionPassed      = "passed"
	transactionFailed      = "failed"
	transactionUnavailable = "unavailable"

	workCompleted = "completed"
	workFailed    = "failed"
	workRunning   = "running"

	deliveryNotAttempted = "not_attempted"
	deliveryFully        = "fully_delivered"
	deliveryPartial      = "partially_delivered"
	deliveryFailed       = "failed"
	deliveryPending      = "pending"
	deliveryUnknown      = "unknown"

	recoveryNotRun          = "not_run"
	recoveryNotRequired     = "not_required"
	recoveryRetryEligible   = "retry_eligible"
	recoveryBlockedConflict = "blocked_precondition"
	recoveryBlockedUnknown  = "blocked_unknown"
	recoveryBlockedExternal = "blocked_external"

	externalNone    = "none"
	externalPending = "pending"
	externalDone    = "recorded"
	externalFailed  = "failed"
	externalUnknown = "unknown"
)

// InspectWorkTransaction reads existing Work evidence after one execution.
// expectedMode is part of the evaluation fixture's contract and lets the
// evaluator catch a history record with mismatched authority. A missing or
// malformed record is reported as a failed History dimension rather than
// being mistaken for a model-quality failure.
func InspectWorkTransaction(stateDir, workID string, expectedMode action.Mode) (TransactionFidelity, error) {
	result := TransactionFidelity{
		Work:            TransactionDimension{State: transactionUnavailable},
		History:         TransactionDimension{State: transactionUnavailable},
		Verification:    TransactionDimension{State: transactionUnavailable},
		Delivery:        TransactionDimension{State: deliveryNotAttempted, Evidence: "no local delivery record"},
		Recovery:        TransactionDimension{State: recoveryNotRun, Evidence: "no delivery attempt"},
		ExternalActions: TransactionDimension{State: externalNone, Evidence: "no external action record"},
	}
	if strings.TrimSpace(stateDir) == "" || strings.TrimSpace(workID) == "" {
		return result, errors.New("transaction inspection requires Work state and ID")
	}
	if err := expectedMode.Validate(); err != nil {
		return result, fmt.Errorf("expected Work mode: %w", err)
	}
	stateRoot, err := filepath.Abs(stateDir)
	if err != nil {
		return result, err
	}
	historyStore, err := workhistory.Open(stateRoot)
	if err != nil {
		return result, err
	}
	record, err := historyStore.Load(workID)
	if errors.Is(err, os.ErrNotExist) {
		result.History = TransactionDimension{State: transactionFailed, Error: "canonical Work history is missing"}
		return result, nil
	}
	if err != nil {
		result.History = TransactionDimension{State: transactionFailed, Error: "canonical Work history is unreadable: " + err.Error()}
		return result, nil
	}
	result.Work = TransactionDimension{State: string(record.Status), Evidence: "canonical Work history"}
	result.History = historyDimension(stateRoot, record, expectedMode)

	bundle, bundleErr := openHistoryBundle(record)
	if bundleErr != nil {
		result.Verification = TransactionDimension{State: transactionUnavailable, Error: bundleErr.Error()}
		if result.History.State == transactionPassed {
			result.History = TransactionDimension{State: transactionFailed, Error: bundleErr.Error()}
		}
		return result, nil
	}
	result.Verification = verificationDimension(stateRoot, record, bundle)
	result.ExternalActions = externalActionDimension(bundle.Manifest.Actions)

	deliveryStore, err := delivery.Open(stateRoot)
	if err != nil {
		return result, err
	}
	records, err := deliveryStore.ListWork(workID)
	if err != nil {
		result.Delivery = TransactionDimension{State: transactionUnavailable, Error: err.Error()}
		result.Recovery = TransactionDimension{State: transactionUnavailable, Error: err.Error()}
		return result, nil
	}
	result.Delivery = deliveryDimension(records)
	result.Recovery = recoveryDimension(records, result.ExternalActions)
	return result, nil
}

func historyDimension(stateRoot string, record workhistory.Record, expectedMode action.Mode) TransactionDimension {
	var problems []string
	if record.Mode != expectedMode {
		problems = append(problems, fmt.Sprintf("mode is %q, expected %q", record.Mode, expectedMode))
	}
	if record.SnapshotID == "" {
		problems = append(problems, "snapshot reference is missing")
	} else if _, err := snapshot.Open(stateRoot, record.SnapshotID); err != nil {
		problems = append(problems, "snapshot does not resolve")
	}
	if record.RevisionID == "" {
		problems = append(problems, "revision reference is missing")
	}
	if record.Evidence.ArtifactManifestPath == "" {
		problems = append(problems, "artifact manifest reference is missing")
	}
	wantDeliveries := filepath.Join(stateRoot, "gator", "delivery", record.ID)
	if record.Evidence.DeliveriesPath != wantDeliveries {
		problems = append(problems, "delivery evidence reference does not match Work ID")
	}
	if len(problems) > 0 {
		return TransactionDimension{State: transactionFailed, Error: strings.Join(problems, "; ")}
	}
	return TransactionDimension{State: transactionPassed, Evidence: "history, snapshot, revision, and delivery references resolve"}
}

func openHistoryBundle(record workhistory.Record) (artifact.Bundle, error) {
	if record.Evidence.ArtifactManifestPath == "" {
		return artifact.Bundle{}, errors.New("canonical Work history has no artifact manifest reference")
	}
	bundle, err := artifact.OpenBundle(filepath.Dir(record.Evidence.ArtifactManifestPath))
	if err != nil {
		return artifact.Bundle{}, fmt.Errorf("open retained Work result: %w", err)
	}
	if err := artifact.VerifyBundle(bundle); err != nil {
		return artifact.Bundle{}, fmt.Errorf("verify retained Work result: %w", err)
	}
	if bundle.Manifest.RunID != record.ID {
		return artifact.Bundle{}, errors.New("retained Work result does not match canonical history")
	}
	return bundle, nil
}

func verificationDimension(stateRoot string, record workhistory.Record, bundle artifact.Bundle) TransactionDimension {
	source, err := snapshot.Open(stateRoot, record.SnapshotID)
	if err != nil {
		return TransactionDimension{State: transactionUnavailable, Error: "open recorded source snapshot: " + err.Error()}
	}
	if bundle.Manifest.Source.SnapshotSHA256 != source.SHA256 {
		return TransactionDimension{State: transactionFailed, Error: "retained manifest source snapshot does not match canonical history"}
	}
	verification := workhistory.VerificationPassed
	for _, check := range bundle.Manifest.Validations {
		if !check.Passed {
			verification = workhistory.VerificationFailed
			break
		}
	}
	if bundle.Manifest.Status == artifact.Failed {
		verification = workhistory.VerificationFailed
	}
	if record.ArtifactStatus != string(bundle.Manifest.Status) || record.VerificationStatus != verification {
		return TransactionDimension{State: transactionFailed, Error: "history verification projection does not match retained manifest"}
	}
	if verification == workhistory.VerificationPassed {
		return TransactionDimension{State: transactionPassed, Evidence: "sealed artifact validations passed"}
	}
	return TransactionDimension{State: transactionFailed, Evidence: "sealed artifact validation or Work execution failed"}
}

func deliveryDimension(records []delivery.Record) TransactionDimension {
	if len(records) == 0 {
		return TransactionDimension{State: deliveryNotAttempted, Evidence: "verified Work result has no local delivery attempt"}
	}
	var applied, failed, pending, unknown, total int
	for _, record := range records {
		for _, effect := range record.Effects {
			total++
			switch effect.Status {
			case delivery.Applied, delivery.Superseded:
				applied++
			case delivery.Failed:
				failed++
			case delivery.Pending:
				pending++
			case delivery.Unknown:
				unknown++
			}
		}
	}
	evidence := fmt.Sprintf("%d local effects across %d delivery record(s)", total, len(records))
	switch {
	case unknown > 0:
		return TransactionDimension{State: deliveryUnknown, Evidence: evidence}
	case total > 0 && applied == total:
		return TransactionDimension{State: deliveryFully, Evidence: evidence}
	case applied > 0 && (failed > 0 || pending > 0):
		return TransactionDimension{State: deliveryPartial, Evidence: evidence}
	case failed > 0:
		return TransactionDimension{State: deliveryFailed, Evidence: evidence}
	default:
		return TransactionDimension{State: deliveryPending, Evidence: evidence}
	}
}

func recoveryDimension(records []delivery.Record, external TransactionDimension) TransactionDimension {
	if external.State == externalUnknown {
		return TransactionDimension{State: recoveryBlockedUnknown, Evidence: "unknown external action is never replayed automatically"}
	}
	if external.State == externalPending || external.State == externalFailed {
		return TransactionDimension{State: recoveryBlockedExternal, Evidence: "external actions require their own approval or reconciliation"}
	}
	if len(records) == 0 {
		return TransactionDimension{State: recoveryNotRun, Evidence: "no local delivery attempt"}
	}
	var retryable, blocked, unknown int
	for _, record := range records {
		for _, effect := range record.Effects {
			switch effect.Status {
			case delivery.Unknown:
				unknown++
			case delivery.Failed, delivery.Pending:
				if effect.Retryable {
					retryable++
				} else {
					blocked++
				}
			}
		}
	}
	switch {
	case unknown > 0:
		return TransactionDimension{State: recoveryBlockedUnknown, Evidence: "a local effect has uncertain outcome"}
	case retryable > 0:
		return TransactionDimension{State: recoveryRetryEligible, Evidence: "failed or pending local effects retain replay-safe preconditions"}
	case blocked > 0:
		return TransactionDimension{State: recoveryBlockedConflict, Evidence: "failed local effects require a new reviewed delivery plan"}
	default:
		return TransactionDimension{State: recoveryNotRequired, Evidence: "no remaining local delivery effect is eligible for retry"}
	}
}

func externalActionDimension(records []action.Record) TransactionDimension {
	if len(records) == 0 {
		return TransactionDimension{State: externalNone, Evidence: "no external action record"}
	}
	var pending, failed, unknown int
	for _, record := range records {
		switch record.Status {
		case action.Unknown:
			unknown++
		case action.Pending:
			pending++
		case action.Failed:
			failed++
		}
	}
	switch {
	case unknown > 0:
		return TransactionDimension{State: externalUnknown, Evidence: "external action outcome is uncertain"}
	case pending > 0:
		return TransactionDimension{State: externalPending, Evidence: "external action is proposed but not executed"}
	case failed > 0:
		return TransactionDimension{State: externalFailed, Evidence: "external action failed without a replayable payload"}
	default:
		return TransactionDimension{State: externalDone, Evidence: "external action records are terminal"}
	}
}

func transactionHistoryGrade(fidelity TransactionFidelity) Grade {
	return Grade{
		Kind:     "transaction_history",
		Passed:   fidelity.History.State == transactionPassed,
		Evidence: fidelity.History.Evidence,
		Error:    fidelity.History.Error,
	}
}
