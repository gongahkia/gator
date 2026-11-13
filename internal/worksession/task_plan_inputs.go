package worksession

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/attachment"
)

// PlanInputChange retains the original immutable options and the exact additions
// requiring the replacement review. It never changes accounts or other grants.
type PlanInputChange struct {
	ParentID           string          `json:"parent_id"`
	TurnID             string          `json:"turn_id"`
	BeforeOptions      json.RawMessage `json:"before_options"`
	AddedUploads       []string        `json:"added_uploads,omitempty"`
	AddedAttachments   []string        `json:"added_attachments,omitempty"`
	RemovedUploads     []string        `json:"removed_uploads,omitempty"`
	RemovedAttachments []string        `json:"removed_attachments,omitempty"`
	Manual             bool            `json:"manual,omitempty"`
	AfterSHA256        string          `json:"after_sha256"`
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
		if validID(change.ParentID) != nil || !planDigest(change.TurnID) || seen[key] || len(change.BeforeOptions) > 128*1024 || len(change.AddedUploads)+len(change.AddedAttachments)+len(change.RemovedUploads)+len(change.RemovedAttachments) == 0 || previous != "" && optionsHash(change.BeforeOptions) != previous {
			return errors.New("invalid input scope history")
		}
		base := change.BeforeOptions
		if len(change.RemovedUploads)+len(change.RemovedAttachments) > 0 {
			var err error
			base, err = removePlanInputs(base, change.RemovedUploads, change.RemovedAttachments)
			if err != nil {
				return err
			}
		}
		after, uploads, attachments, err := mergePlanInputs(base, change.AddedUploads, change.AddedAttachments)
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

// PlanInputRef is an exact selected input. Name is presentation metadata only.
type PlanInputRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

func removePlanInputs(before json.RawMessage, uploads, attachments []string) (json.RawMessage, error) {
	validated, _, _, err := mergePlanInputs(before, nil, nil)
	if err != nil {
		return nil, err
	}
	if len(uploads)+len(attachments) == 0 || len(uploads)+len(attachments) > attachment.MaxInputs {
		return nil, errors.New("select the exact inputs to remove")
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(validated, &fields)
	for _, selection := range []struct {
		key     string
		removed []string
	}{{"Uploads", uploads}, {"Attachments", attachments}} {
		if len(selection.removed) == 0 {
			continue
		}
		key := selection.key
		for candidate := range fields {
			if strings.EqualFold(candidate, key) {
				key = candidate
				break
			}
		}
		var original []string
		if json.Unmarshal(fields[key], &original) != nil {
			return nil, errors.New("selected input list is unavailable")
		}
		removing := map[string]bool{}
		for _, id := range selection.removed {
			if removing[id] || !slices.Contains(original, id) {
				return nil, errors.New("input is no longer selected in this exact task")
			}
			removing[id] = true
		}
		kept := []string{}
		for _, id := range original {
			if !removing[id] {
				kept = append(kept, id)
			}
		}
		fields[key], _ = json.Marshal(kept)
	}
	return json.Marshal(fields)
}

// RemoveTaskPlanInputs narrows a pending review. Originals stay in the audit
// history, and no changed scope can execute until its new exact approval.
func (s Store) RemoveTaskPlanInputs(id, request string, revision uint64, digest, requestID string, inputs []PlanInputRef) (RequestJournal, error) {
	j, index, err := s.taskPlanRequest(id, request, revision, digest)
	if err != nil {
		return j, err
	}
	r := &j.Requests[index]
	p := r.Plan.Clone()
	if r.State != "queued" || (p.Phase != "review" && p.Phase != "approved") || p.Content == nil || p.DrainingRun != "" || len(p.InputChanges) >= 32 || revision == ^uint64(0) || requestID == "" || len(requestID) > 64 {
		return j, ErrPlanChanged
	}
	if err := s.CheckPlanBinding(id, *r); err != nil {
		return j, err
	}
	uploads, attachments := []string{}, []string{}
	for _, input := range inputs {
		switch input.Kind {
		case "upload":
			uploads = append(uploads, input.ID)
		case "source":
			attachments = append(attachments, input.ID)
		default:
			return j, errors.New("unsupported selected input kind")
		}
	}
	after, err := removePlanInputs(r.Options, uploads, attachments)
	if err != nil {
		return j, err
	}
	p.InputChanges = append(p.InputChanges, PlanInputChange{ParentID: id, TurnID: planHash([]byte("manual-input-removal:" + requestID)), BeforeOptions: append(json.RawMessage(nil), r.Options...), RemovedUploads: uploads, RemovedAttachments: attachments, AfterSHA256: optionsHash(after), Manual: true})
	r.Options = after
	p.Binding.OptionsSHA256 = optionsHash(after)
	p.Revision++
	p.Approval = nil
	p.Phase = "review"
	p.Digest = p.contentDigest()
	r.Plan = p
	r.UpdatedAt = time.Now().UTC()
	r.Issue = ""
	return s.SaveRequests(id, j)
}
