package worksession

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gongahkia/gator/internal/attachment"
)

// PlanInputChange retains the original immutable options and the exact additions
// requiring the replacement review. It never changes accounts or other grants.
type PlanInputChange struct {
	ParentID         string          `json:"parent_id"`
	TurnID           string          `json:"turn_id"`
	BeforeOptions    json.RawMessage `json:"before_options"`
	AddedUploads     []string        `json:"added_uploads,omitempty"`
	AddedAttachments []string        `json:"added_attachments,omitempty"`
	AfterSHA256      string          `json:"after_sha256"`
}

// OriginalAdmissionOptions identifies a transport repeat, never current task
// authority. Explicitly reviewed input evolution does not rewrite admission.
func (r Request) OriginalAdmissionOptions() json.RawMessage {
	if r.Plan != nil && len(r.Plan.InputChanges) > 0 {
		return append(json.RawMessage(nil), r.Plan.InputChanges[0].BeforeOptions...)
	}
	return append(json.RawMessage(nil), r.Options...)
}

func mergePlanInputs(before json.RawMessage, uploads, attachments []string) (json.RawMessage, []string, []string, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(before, &fields) != nil || fields == nil {
		return nil, nil, nil, errors.New("invalid original task options")
	}
	uploadKey, attachmentKey := "Uploads", "Attachments"
	foundUpload, foundAttachment := false, false
	optionKeys := map[string]bool{}
	for key := range fields {
		folded := strings.ToLower(key)
		if optionKeys[folded] {
			return nil, nil, nil, errors.New("ambiguous original task option names")
		}
		optionKeys[folded] = true
		if strings.EqualFold(key, "Uploads") {
			if foundUpload {
				return nil, nil, nil, errors.New("ambiguous original upload options")
			}
			foundUpload = true
			uploadKey = key
		}
		if strings.EqualFold(key, "Attachments") {
			if foundAttachment {
				return nil, nil, nil, errors.New("ambiguous original attachment options")
			}
			foundAttachment = true
			attachmentKey = key
		}
	}
	var oldUploads, oldAttachments []string
	if raw := fields[uploadKey]; len(raw) > 0 && json.Unmarshal(raw, &oldUploads) != nil {
		return nil, nil, nil, errors.New("invalid original uploaded inputs")
	}
	if raw := fields[attachmentKey]; len(raw) > 0 && json.Unmarshal(raw, &oldAttachments) != nil {
		return nil, nil, nil, errors.New("invalid original source inputs")
	}
	addedUploads, addedAttachments := []string{}, []string{}
	for _, id := range uploads {
		if !attachment.ValidUploadID(id) {
			return nil, nil, nil, errors.New("invalid uploaded input identity")
		}
		if !slices.Contains(oldUploads, id) {
			oldUploads = append(oldUploads, id)
			addedUploads = append(addedUploads, id)
		}
	}
	for _, path := range attachments {
		if path == "" || len(path) > 4096 || strings.ContainsRune(path, 0) || filepath.IsAbs(path) || filepath.Clean(path) != path || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
			return nil, nil, nil, errors.New("invalid selected source input")
		}
		if !slices.Contains(oldAttachments, path) {
			oldAttachments = append(oldAttachments, path)
			addedAttachments = append(addedAttachments, path)
		}
	}
	if len(oldUploads)+len(oldAttachments) > attachment.MaxInputs {
		return nil, nil, nil, errors.New("combined task inputs exceed the attachment limit")
	}
	if len(addedUploads) > 0 {
		fields[uploadKey], _ = json.Marshal(oldUploads)
	}
	if len(addedAttachments) > 0 {
		fields[attachmentKey], _ = json.Marshal(oldAttachments)
	}
	after, err := json.Marshal(fields)
	return after, addedUploads, addedAttachments, err
}

func validatePlanInputChanges(r Request) error {
	if r.Plan == nil || len(r.Plan.InputChanges) == 0 {
		return nil
	}
	if len(r.Plan.InputChanges) > 32 {
		return errors.New("too many input scope changes")
	}
	previous := ""
	seen := map[string]bool{}
	for _, change := range r.Plan.InputChanges {
		key := change.ParentID + ":" + change.TurnID
		if validID(change.ParentID) != nil || !planDigest(change.TurnID) || seen[key] || len(change.BeforeOptions) > 128*1024 || len(change.AddedUploads)+len(change.AddedAttachments) == 0 || previous != "" && optionsHash(change.BeforeOptions) != previous {
			return errors.New("invalid input scope history")
		}
		after, uploads, attachments, err := mergePlanInputs(change.BeforeOptions, change.AddedUploads, change.AddedAttachments)
		if err != nil || !slices.Equal(uploads, change.AddedUploads) || !slices.Equal(attachments, change.AddedAttachments) || optionsHash(after) != change.AfterSHA256 {
			return errors.New("input change altered original settings or input identity")
		}
		previous = change.AfterSHA256
		seen[key] = true
	}
	if previous != optionsHash(r.Options) {
		return errors.New("effective options differ from reviewed input history")
	}
	return nil
}

// MatchesPlanInputEvolution accepts only the canonical chain from its original
// immutable admission. Existing scope/account pin verification remains required.
func MatchesPlanInputEvolution(r Request, original, effective json.RawMessage) bool {
	return r.Plan != nil && len(r.Plan.InputChanges) > 0 && validateTaskPlan(r) == nil && optionsHash(r.Plan.InputChanges[0].BeforeOptions) == optionsHash(original) && optionsHash(r.Options) == optionsHash(effective)
}
