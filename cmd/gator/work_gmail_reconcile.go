package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/connector"
)

type gmailReconciliation struct {
	Version        int       `json:"version"`
	WorkID         string    `json:"work_id"`
	ManifestSHA256 string    `json:"manifest_sha256"`
	CorrelationID  string    `json:"correlation_id"`
	Status         string    `json:"status"`
	GmailID        string    `json:"gmail_id,omitempty"`
	CheckedAt      time.Time `json:"checked_at"`
}

// reconcileGmailSend checks the Sent mailbox for one uncertain, approved send.
// The sealed action record remains unchanged, and this path never sends mail.
func reconcileGmailSend(ctx context.Context, stateDir, workID, correlationID string, out io.Writer) error {
	if !workReferencePattern.MatchString(workID) || !validGmailCorrelationID(correlationID) {
		return errors.New("valid Work and Gmail correlation IDs are required")
	}
	bundle, found, err := resolveWorkBundle(workID, stateDir)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("retained Work run not found")
	}
	return reconcileGmailBundle(ctx, stateDir, bundle, "", correlationID, out)
}

func reconcileGmailBundle(ctx context.Context, stateDir string, bundle artifact.Bundle, expectedManifest, correlationID string, out io.Writer) error {
	if expectedManifest != "" && bundle.ManifestSHA256 != expectedManifest {
		return errors.New("the retained request changed; reopen its receipt")
	}
	if err := artifact.VerifyBundle(bundle); err != nil {
		return fmt.Errorf("verify Work run before reconciliation: %w", err)
	}
	before, err := bundle.Root.ReadRegularFile("manifest.json", 8*1024*1024)
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(before)) != bundle.ManifestSHA256 {
		return errors.New("the retained request changed before the Sent lookup; reopen its receipt")
	}
	var selected *action.Record
	for index := range bundle.Manifest.Actions {
		record := &bundle.Manifest.Actions[index]
		if record.Proposal.CorrelationID == correlationID && record.Proposal.Operation == "gmail_send" {
			if selected != nil {
				return errors.New("Gmail correlation ID is duplicated in this Work run")
			}
			selected = record
		}
	}
	if selected == nil {
		return errors.New("Gmail correlation ID was not found in this Work run")
	}
	if selected.Status != action.Unknown {
		return fmt.Errorf("Gmail action is %s; only an unknown send needs reconciliation", selected.Status)
	}
	settings, err := loadSettings()
	if err != nil {
		return err
	}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		return err
	}
	descriptor, found := registry.Get(selected.Proposal.ConnectorID)
	if !found {
		return errors.New("the original Google connector is no longer configured")
	}
	target, err := connector.GmailSendTarget(descriptor)
	if err != nil || target != selected.Proposal.Target {
		return errors.New("the original Gmail send target no longer matches this connector")
	}
	permissions := connector.PermissionSet(settings.ConnectorPermissions)
	for _, operation := range []string{"gmail_list", "gmail_get"} {
		if err := checkWatchRead(registry, permissions, descriptor.ID, operation); err != nil {
			return errors.New("Gmail reconciliation requires this connector's gmail.readonly scope and allowed list/detail reads; configure access before checking Sent")
		}
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return err
	}
	runtime := connector.Runtime{Registry: registry, Credentials: credentials}
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	status, gmailID, err := lookupGmailSent(probeCtx, runtime, descriptor.ID, correlationID)
	if err != nil {
		return fmt.Errorf("Gmail Sent lookup: %s", goalAccessFailure(err, descriptor.ID))
	}
	manifest, err := bundle.Root.ReadRegularFile("manifest.json", 8*1024*1024)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(manifest)
	if !bytes.Equal(before, manifest) {
		return errors.New("the retained request changed during the Sent lookup; reopen its receipt")
	}
	check := gmailReconciliation{
		Version: 1, WorkID: bundle.Manifest.RunID, ManifestSHA256: hex.EncodeToString(digest[:]),
		CorrelationID: correlationID, Status: status, GmailID: gmailID, CheckedAt: time.Now().UTC(),
	}
	if err := writeGmailReconciliation(stateDir, check); err != nil {
		return err
	}
	switch status {
	case "found":
		_, err = fmt.Fprintf(out, "Gmail message %s is in Sent for reconciliation ID %s. This confirms a Sent mailbox copy, not recipient delivery. Original Work action remains unknown; no send was retried.\n", gmailID, correlationID)
	case "ambiguous":
		_, err = fmt.Fprintf(out, "Multiple Sent candidates match reconciliation ID %s. Outcome remains unknown; inspect Sent manually. No send was retried.\n", correlationID)
	default:
		_, err = fmt.Fprintf(out, "No verified Sent copy was found for reconciliation ID %s. Outcome remains unknown; search indexing can lag. No send was retried.\n", correlationID)
	}
	return err
}

func validGmailCorrelationID(value string) bool {
	if len(value) != 32 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func lookupGmailSent(ctx context.Context, runtime connector.Runtime, connectorID, correlationID string) (string, string, error) {
	messageID := "<gator-" + correlationID + "@gator.invalid>"
	query, _ := json.Marshal(map[string]any{"query": "in:sent rfc822msgid:" + messageID, "limit": 10})
	result, err := runtime.Invoke(ctx, action.Inspect, connectorID, "gmail_list", query)
	if err != nil {
		return "", "", err
	}
	var listing struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		NextPageToken string          `json:"nextPageToken"`
		Error         json.RawMessage `json:"error"`
	}
	if json.Unmarshal(result.Data, &listing) != nil || listing.Error != nil {
		return "", "", errors.New("invalid Gmail list response")
	}
	if len(listing.Messages) == 0 && listing.NextPageToken == "" {
		return "unconfirmed", "", nil
	}
	if len(listing.Messages) != 1 || listing.NextPageToken != "" {
		return "ambiguous", "", nil
	}
	id := listing.Messages[0].ID
	if !validGmailResultID(id) {
		return "ambiguous", "", nil
	}
	input, _ := json.Marshal(map[string]string{"resource_id": id})
	detail, err := runtime.Invoke(ctx, action.Inspect, connectorID, "gmail_get", input)
	if err != nil {
		return "", "", err
	}
	var message struct {
		ID       string   `json:"id"`
		LabelIDs []string `json:"labelIds"`
		Payload  struct {
			Headers []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
		} `json:"payload"`
	}
	if json.Unmarshal(detail.Data, &message) != nil || message.ID != id {
		return "unconfirmed", "", nil
	}
	sent, header := false, false
	for _, label := range message.LabelIDs {
		sent = sent || label == "SENT"
	}
	for _, field := range message.Payload.Headers {
		header = header || strings.EqualFold(field.Name, "Message-ID") && strings.TrimSpace(field.Value) == messageID
	}
	if !sent || !header {
		return "unconfirmed", "", nil
	}
	return "found", id, nil
}

func validGmailResultID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

func writeGmailReconciliation(stateDir string, check gmailReconciliation) error {
	if !workReferencePattern.MatchString(check.WorkID) || !validGmailCorrelationID(check.CorrelationID) {
		return errors.New("invalid Gmail reconciliation identity")
	}
	directory := filepath.Join(stateDir, "gator", "reconciliations", check.WorkID)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".check-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if err := json.NewEncoder(file).Encode(check); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(directory, check.CorrelationID+".json"))
}
