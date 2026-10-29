package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

const maxMemoryEvidence = 8

// Evidence retains bounded provenance, not another copy of superseded personal
// text. A matching text hash is evidence of repetition, never semantic agreement.
type Evidence struct {
	ObservedAt     time.Time `json:"observed_at"`
	Basis          string    `json:"basis"`
	ConversationID string    `json:"conversation_id,omitempty"`
	RunID          string    `json:"run_id,omitempty"`
	TextSHA256     string    `json:"text_sha256"`
}

func textDigest(text string) string {
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:])
}

func (r Record) alreadyObserved(input ChatInput) bool {
	if r.Origin != nil && r.Origin.ConversationID == input.ConversationID && r.Origin.RunID == input.RunID && r.Origin.Quote == input.Quote {
		return true
	}
	for _, evidence := range r.Evidence {
		if evidence.ConversationID == input.ConversationID && evidence.RunID == input.RunID && evidence.TextSHA256 == textDigest(input.Quote) {
			return true
		}
	}
	return false
}

// Only a new user statement, explicit user edit or reviewed file publication
// calls observe. Search, profile selection and embedding refresh never call it.
func (r *Record) observe(basis string, origin *ChatOrigin) {
	evidence := Evidence{ObservedAt: r.UpdatedAt, Basis: basis, TextSHA256: textDigest(r.Text)}
	if origin != nil {
		evidence.ConversationID, evidence.RunID = origin.ConversationID, origin.RunID
	}
	r.LastObservedAt, r.LastConfirmedAt = r.UpdatedAt, r.UpdatedAt
	r.Evidence = append(r.Evidence, evidence)
	if len(r.Evidence) > maxMemoryEvidence {
		r.Evidence = append([]Evidence(nil), r.Evidence[len(r.Evidence)-maxMemoryEvidence:]...)
	}
}

func validateEvidence(record Record) error {
	if len(record.Evidence) > maxMemoryEvidence || record.LastObservedAt.After(record.UpdatedAt) || record.LastConfirmedAt.After(record.LastObservedAt) {
		return errors.New("invalid memory evidence timestamps or count")
	}
	var previous time.Time
	for _, evidence := range record.Evidence {
		if evidence.ObservedAt.IsZero() || evidence.ObservedAt.Before(record.CreatedAt) || evidence.ObservedAt.Before(previous) || evidence.ObservedAt.After(record.LastObservedAt) || len(evidence.ConversationID) > 128 || len(evidence.RunID) > 128 || len(evidence.TextSHA256) != 64 {
			return errors.New("invalid memory evidence")
		}
		if _, err := hex.DecodeString(evidence.TextSHA256); err != nil {
			return errors.New("invalid memory evidence digest")
		}
		switch evidence.Basis {
		case "user-stated", "user-corrected", "user-added", "file-reviewed":
		default:
			return errors.New("invalid memory evidence basis")
		}
		previous = evidence.ObservedAt
	}
	return nil
}
