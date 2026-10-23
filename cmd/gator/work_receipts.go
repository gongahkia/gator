package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/workapp"
	"github.com/gongahkia/gator/internal/workspace"
)

// The desktop supplies an owned bundle reference, never connector credentials,
// a mailbox query or a send payload. Checking Sent invokes only existing reads.
func workReceiptChecker(stateDir string) func(context.Context, workapp.ReceiptRequest) (workapp.ReceiptCheck, error) {
	return func(ctx context.Context, request workapp.ReceiptRequest) (workapp.ReceiptCheck, error) {
		var empty workapp.ReceiptCheck
		if !workReferencePattern.MatchString(request.RunID) || !validGmailCorrelationID(request.CorrelationID) || request.Action != "inspect" && request.Action != "check_sent" {
			return empty, errors.New("invalid retained request identity")
		}
		release, err := state.Guard(stateDir, false)
		if err != nil {
			return empty, err
		}
		defer release()
		bundle, err := artifact.OpenBundle(request.BundlePath)
		if err != nil {
			return empty, err
		}
		if err := artifact.VerifyBundle(bundle); err != nil {
			return empty, err
		}
		if bundle.Manifest.RunID != request.RunID || bundle.ManifestSHA256 != request.ManifestSHA256 {
			return empty, errors.New("the retained request changed; reopen its receipt")
		}
		matches := 0
		for _, record := range bundle.Manifest.Actions {
			if record.Proposal.CorrelationID == request.CorrelationID && record.Proposal.Operation == "gmail_send" && record.Status == action.Unknown {
				matches++
			}
		}
		if matches != 1 {
			return empty, errors.New("select one unconfirmed Gmail request from this result")
		}
		if request.Action == "check_sent" {
			if err := reconcileGmailBundle(ctx, stateDir, bundle, request.ManifestSHA256, request.CorrelationID, io.Discard); err != nil {
				return empty, err
			}
		}
		root, err := workspace.Open(filepath.Join(stateDir, "gator"))
		if err != nil {
			return empty, err
		}
		data, err := root.ReadRegularFile(filepath.Join("reconciliations", request.RunID, request.CorrelationID+".json"), 16*1024)
		if errors.Is(err, os.ErrNotExist) {
			return workapp.ReceiptCheck{Status: "not_checked"}, nil
		}
		if err != nil {
			return empty, err
		}
		var check gmailReconciliation
		if json.Unmarshal(data, &check) != nil || check.Version != 1 || check.WorkID != request.RunID || check.CorrelationID != request.CorrelationID || check.ManifestSHA256 != request.ManifestSHA256 || check.CheckedAt.IsZero() || check.Status == "found" && !validGmailResultID(check.GmailID) || check.Status != "found" && check.GmailID != "" || (check.Status != "found" && check.Status != "ambiguous" && check.Status != "unconfirmed") {
			return empty, errors.New("the saved Sent check does not match this retained request")
		}
		return workapp.ReceiptCheck{Status: check.Status, ProviderID: check.GmailID, CheckedAt: check.CheckedAt.Format(time.RFC3339)}, nil
	}
}
