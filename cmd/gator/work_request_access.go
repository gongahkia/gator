package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/workapp"
	"github.com/gongahkia/gator/internal/worksession"
)

func reviewRequestAccess(r worksession.Request, selected []string) (workapp.RequestAccessReview, error) {
	review := workapp.RequestAccessReview{Ready: true, Requirements: []workapp.RequestAccessRequirement{}}
	if r.Recovery == nil || len(r.Recovery.Needs) == 0 {
		return review, errors.New("this task has no retained access requirement")
	}
	if len(selected) != 0 && len(selected) != len(r.Recovery.Needs) {
		return review, errors.New("choose one account for each retained requirement")
	}
	var options workapp.RunOptions
	if json.Unmarshal(r.Options, &options) != nil {
		return review, errors.New("original task settings are unavailable")
	}
	settings, err := loadSettings()
	if err != nil {
		return review, err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return review, err
	}
	ids := append([]string{}, options.ConnectorIDs...)
	for index, need := range r.Recovery.Needs {
		id := need.ConnectorID
		if len(selected) > 0 {
			id = selected[index]
		}
		row := workapp.RequestAccessRequirement{Need: need, Selected: id, Choices: []workapp.RequestAccessChoice{}}
		available := false
		for _, d := range settings.Connectors {
			if d.Kind != need.Kind {
				continue
			}
			// Inventory all supported operations so an OAuth scope missing from the
			// configured grant can direct the user to canonical account setup.
			inventory := d
			inventory.Authentication = connector.AuthNone
			supported := false
			var capability action.Capability
			for _, op := range inventory.Operations() {
				if op.ID == need.Operation {
					supported = true
					capability = op.Capability
				}
			}
			if !supported {
				continue
			}
			permission := connector.PermissionSet(settings.ConnectorPermissions).Resolve(d.ID, need.Operation, capability)
			choice := workapp.RequestAccessChoice{ID: d.ID, Label: d.Name, Kind: d.Kind, Destination: d.Resource, Operation: need.Operation, Permission: string(permission)}
			configured := false
			for _, op := range d.Operations() {
				configured = configured || op.ID == need.Operation
			}
			credential, err := connectionCredential(credentials, d)
			if err != nil {
				return review, err
			}
			switch {
			case !configured:
				choice.Issue = "Enable this operation's provider permission in Connections, then review access again."
			case d.Authentication != connector.AuthNone && credential == nil:
				choice.Issue = "Sign in or add this account's token in Connections."
			case credential != nil && credential.Expired(time.Now()) && credential.Refresh == "":
				choice.Issue = "Reconnect this expired account in Connections."
			case permission == connector.PermissionDeny:
				choice.Issue = "This operation is denied by Gator's saved permissions. Review Permissions before continuing."
			case options.Mode == "inspect" && action.RequiresFreshApproval(capability):
				choice.Issue = "The original request is read-only. A separately reviewed action task is required."
			}
			row.Choices = append(row.Choices, choice)
			if d.ID == id {
				available = choice.Issue == ""
			}
		}
		if !available {
			review.Ready = false
		}
		if available && need.ConnectorID != "" && need.ConnectorID != id {
			ids = slices.DeleteFunc(ids, func(value string) bool { return value == need.ConnectorID })
		}
		if available && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
		review.Requirements = append(review.Requirements, row)
	}
	if len(ids) > 16 {
		return review, errors.New("the original task and selected accounts exceed the connection limit")
	}
	options.ConnectorIDs = ids
	if !review.Ready {
		options.ConnectorIDs = slices.DeleteFunc(append([]string{}, ids...), func(id string) bool {
			for _, d := range settings.Connectors {
				if d.ID == id {
					return false
				}
			}
			return true
		})
	}
	captured, err := captureConversationInput(workapp.ConversationInput{Version: 1, RunOptions: options})
	if err != nil {
		return review, err
	}
	if r.Recovery.ProcessingDigest == "" || captured.ProcessingDigest != r.Recovery.ProcessingDigest {
		return review, errors.New("the original processing or execution settings changed; inspect this task before reviewing account access")
	}
	review.Connections = ids
	review.Summary = "Applies only to this retained task. Original inputs, work mode, model and remaining budget stay fixed. Readiness reflects local configuration; provider access is checked during execution. Selected accounts expose their currently permitted operations; each consequential action still requires exact review. Saved source selection for future messages and background checks is unchanged."
	// Report the actual permitted operation surface, not just the requested one.
	for _, d := range settings.Connectors {
		if !slices.Contains(ids, d.ID) {
			continue
		}
		operations := []string{}
		for _, op := range d.Operations() {
			operations = append(operations, op.ID+":"+string(connector.PermissionSet(settings.ConnectorPermissions).Resolve(d.ID, op.ID, op.Capability)))
		}
		review.Summary += "\n" + d.Name + " (" + d.ID + "): " + strings.Join(operations, ", ")
	}
	identity, _ := json.Marshal([]any{r.ID, r.State, r.WorkID, r.RevisionID, r.Options, r.Recovery, review, captured.ScopeDigest, captured.OAuthRefreshPins, captured.CredentialPins})
	review.Token = fmt.Sprintf("%x", sha256.Sum256(identity))
	review.Change = &worksession.PlanAccessChange{ConnectorIDs: ids, BeforeScope: r.Recovery.ScopeDigest, AfterScope: captured.ScopeDigest, ProcessingDigest: captured.ProcessingDigest, ReviewID: review.Token, OAuthPins: captured.OAuthRefreshPins, CredentialPins: captured.CredentialPins}
	return review, nil
}
