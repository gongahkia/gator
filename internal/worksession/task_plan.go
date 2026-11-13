package worksession

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// TaskPlan is a deliberation/approval gate on an existing admitted request. It
// does not grant command, connected-service, terminal or artifact authority.
type TaskPlan struct {
	Version  int           `json:"version"`
	Revision uint64        `json:"revision"`
	Phase    string        `json:"phase"`
	Digest   string        `json:"digest,omitempty"`
	Binding  PlanBinding   `json:"binding"`
	Sources  []string      `json:"sources"`
	Budget   PlanBudget    `json:"budget"`
	Content  *PlanContent  `json:"content,omitempty"`
	Approval *PlanApproval `json:"approval,omitempty"`
	Edit     *PlanEdit     `json:"edit,omitempty"`
	// DrainingRun fences a reviewed execution before its tool-free replan.
	// The owner must retain that run's result and measured budget before review.
	DrainingRun string        `json:"draining_run,omitempty"`
	Attempts    []PlanAttempt `json:"attempts,omitempty"`
}
type PlanBinding struct {
	UpdatedAt      time.Time `json:"conversation_updated_at"`
	SnapshotID     string    `json:"snapshot_id,omitempty"`
	ConversationID string    `json:"conversation_id"`
	RequestID      string    `json:"request_id"`
	HeadRevision   string    `json:"head_revision,omitempty"`
	SourceSHA256   string    `json:"source_sha256"`
	OptionsSHA256  string    `json:"options_sha256"`
	PromptSHA256   string    `json:"prompt_sha256"`
}
type PlanBudget struct {
	MaxSteps     int `json:"max_steps"`
	CodeMaxSteps int `json:"code_max_steps"`
}
type PlanStep struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}
type PlanContent struct {
	Outcome string     `json:"outcome"`
	Steps   []PlanStep `json:"steps"`
	Sources []string   `json:"sources"`
	Effects []string   `json:"effects"`
}
type PlanApproval struct {
	Revision uint64    `json:"revision"`
	Digest   string    `json:"digest"`
	At       time.Time `json:"at"`
}

var ErrPlanChanged = errors.New("the plan, task or conversation changed; review its current revision")

func planHash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func optionsHash(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return planHash(canonical)
}
func boundedPlanText(value string, limit int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= limit && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
func planSourceID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
func validPlanContent(content PlanContent, sources []string) bool {
	if !boundedPlanText(content.Outcome, 4096) || len(content.Steps) < 1 || len(content.Steps) > 24 || len(content.Sources) > 16 || len(content.Effects) < 1 || len(content.Effects) > 7 {
		return false
	}
	seen := map[string]bool{}
	for _, step := range content.Steps {
		if !catalogIdentity.MatchString(step.ID) || len(step.ID) > 64 || seen[step.ID] || !boundedPlanText(step.Text, 1024) {
			return false
		}
		seen[step.ID] = true
	}
	seen = map[string]bool{}
	for _, source := range content.Sources {
		if !slices.Contains(sources, source) || seen[source] {
			return false
		}
		seen[source] = true
	}
	seen = map[string]bool{}
	for _, effect := range content.Effects {
		if !slices.Contains([]string{"read", "draft", "code", "checks", "connected_write", "settings", "browser"}, effect) || seen[effect] {
			return false
		}
		seen[effect] = true
	}
	return true
}

// Validate checks a proposal against the frozen selected source identities.
func (c PlanContent) Validate(sources []string) error {
	if !validPlanContent(c, sources) {
		return errors.New("invalid bounded plan content or source scope")
	}
	return nil
}

func (p TaskPlan) contentDigest() string {
	var edit *PlanEditReference
	if p.Edit != nil {
		ref := p.Edit.PlanEditReference
		edit = &ref
	}
	data, _ := json.Marshal(struct {
		Version     int
		Revision    uint64
		Binding     PlanBinding
		Sources     []string
		Budget      PlanBudget
		Content     *PlanContent
		Edit        *PlanEditReference `json:",omitempty"`
		DrainingRun string             `json:",omitempty"`
		Attempts    []PlanAttempt      `json:",omitempty"`
	}{p.Version, p.Revision, p.Binding, p.Sources, p.Budget, p.Content, edit, p.DrainingRun, p.Attempts})
	return planHash(data)
}
func validateTaskPlan(r Request) error {
	p := r.Plan
	if p == nil {
		return nil
	}
	if err := validatePlanAttempts(p.Attempts); err != nil {
		return err
	}
	if p.Version != 1 || !catalogIdentity.MatchString(p.Binding.ConversationID) || p.Binding.UpdatedAt.IsZero() || p.Binding.RequestID != r.ID || p.Binding.OptionsSHA256 != optionsHash(r.Options) || p.Binding.PromptSHA256 != planHash([]byte(r.Text)) || (len(p.Binding.SourceSHA256) != 64 || strings.Trim(p.Binding.SourceSHA256, "0123456789abcdef") != "") || len(p.Sources) > 16 || p.Budget.MaxSteps < 0 || p.Budget.MaxSteps > 100000 || p.Budget.CodeMaxSteps < 0 || p.Budget.CodeMaxSteps > 100000 {
		return errors.New("invalid retained plan binding")
	}
	var frozen struct {
		PlanFirst    bool
		ConnectorIDs []string
	}
	if json.Unmarshal(r.Options, &frozen) != nil || !frozen.PlanFirst || !slices.Equal(p.Sources, frozen.ConnectorIDs) {
		return errors.New("plan sources or opt-in differ from frozen request")
	}
	seen := map[string]bool{}
	for _, id := range p.Sources {
		if !planSourceID(id) || seen[id] {
			return errors.New("invalid plan sources")
		}
		seen[id] = true
	}
	if p.Binding.HeadRevision != "" && !catalogIdentity.MatchString(p.Binding.HeadRevision) {
		return errors.New("invalid plan boundary")
	}
	if p.Content == nil {
		initial := p.Edit == nil && p.Revision == 0 && p.Digest == "" && (p.Phase == "preparing" || p.Phase == "interrupted")
		correcting := p.Edit != nil && p.Revision > 0 && p.Digest == p.contentDigest() && (p.Phase == "revising" || p.Phase == "interrupted") && p.Edit.State != "applied"
		if (!initial && !correcting) || p.Approval != nil {
			return errors.New("invalid pending plan")
		}
	} else if p.Revision == 0 || !validPlanContent(*p.Content, p.Sources) || p.Digest != p.contentDigest() {
		return errors.New("invalid retained plan content")
	}
	switch p.Phase {
	case "preparing", "revising", "review", "approved", "dispatching", "interrupted", "finished":
	default:
		return errors.New("invalid plan phase")
	}
	if p.Content == nil && p.Phase != "preparing" && p.Phase != "revising" && p.Phase != "interrupted" {
		return errors.New("missing reviewed plan")
	}
	if err := validatePlanEdit(r); err != nil {
		return err
	}
	if p.DrainingRun != "" && (validID(p.DrainingRun) != nil || p.Phase != "revising" && p.Phase != "interrupted" || p.Approval != nil || p.Edit == nil) {
		return errors.New("invalid draining plan execution")
	}
	if p.Approval != nil {
		if p.Approval.Revision != p.Revision || p.Approval.Digest != p.Digest || p.Approval.At.IsZero() {
			return errors.New("stale plan approval")
		}
	}
	if (p.Phase == "approved" || p.Phase == "dispatching" || p.Phase == "finished") && p.Approval == nil {
		return errors.New("plan approval missing")
	}
	return nil
}

// AdmitPlanRequest freezes the task/options/source scope without executing it.
// Matching transport retries acknowledge the same durable admission.
func (s Store) AdmitPlanRequest(id, text string, options json.RawMessage, clientID string, sources []string, budget PlanBudget, now time.Time) (RequestJournal, Request, error) {
	c, err := s.Load(id)
	if err != nil {
		return RequestJournal{}, Request{}, err
	}
	j, err := s.LoadRequests(id)
	if err != nil {
		return j, Request{}, err
	}
	for _, r := range j.Requests {
		if clientID != "" && r.ClientID == clientID {
			if r.Plan == nil || r.Text != strings.TrimSpace(text) || optionsHash(r.Options) != optionsHash(options) || !slices.Equal(r.Plan.Sources, sources) || r.Plan.Budget != budget {
				return j, Request{}, ErrPlanChanged
			}
			return j, r, nil
		}
	}
	pending := 0
	for _, r := range j.Requests {
		if r.Pending() {
			pending++
		}
	}
	if pending >= 17 {
		return j, Request{}, errors.New("review or remove a retained message before sending another")
	}
	r, err := NewRequest(text, options, now)
	if err != nil {
		return j, r, err
	}
	r.ClientID = clientID
	r.Plan = newTaskPlan(c, r, sources, budget)
	j.Requests = append(j.Requests, r)
	// Match ordinary request retention. Only terminal records can leave the
	// bounded journal; pending/uncertain requests are never discarded for space.
	for len(j.Requests) > MaxRetainedRequests {
		index := slices.IndexFunc(j.Requests, func(candidate Request) bool { return !candidate.Pending() })
		if index < 0 {
			return j, Request{}, errors.New("too many retained pending messages")
		}
		j.Requests = slices.Delete(j.Requests, index, index+1)
	}
	next, err := s.SaveRequests(id, j)
	return next, r, err
}

func (s Store) CheckPlanBinding(id string, r Request) error {
	if r.Plan == nil || validateTaskPlan(r) != nil || r.Plan.Binding.ConversationID != id {
		return ErrPlanChanged
	}
	c, err := s.Load(id)
	if err != nil {
		return err
	}
	// The first admission may update an empty chat's title/timestamp without
	// changing its context. A retained head also binds its update epoch, so an
	// intervening head move cannot restore old authority by moving back.
	if (r.Plan.Binding.HeadRevision != "" && !c.UpdatedAt.Equal(r.Plan.Binding.UpdatedAt)) || c.SnapshotID != r.Plan.Binding.SnapshotID || c.HeadRevision != r.Plan.Binding.HeadRevision || planHash([]byte(c.SourcePath)) != r.Plan.Binding.SourceSHA256 {
		return ErrPlanChanged
	}
	return nil
}
func (s Store) taskPlanRequest(id, request string, revision uint64, digest string) (RequestJournal, int, error) {
	j, err := s.LoadRequests(id)
	if err != nil {
		return j, -1, err
	}
	for i, r := range j.Requests {
		if r.ID == request {
			if r.Plan == nil || r.Plan.Revision != revision || r.Plan.Digest != digest {
				return j, -1, ErrPlanChanged
			}
			if validateTaskPlan(r) != nil || r.Plan.Binding.ConversationID != id {
				return j, -1, ErrPlanChanged
			}
			return j, i, nil
		}
	}
	return j, -1, ErrPlanChanged
}

// ReviseTaskPlan is used both for a tool-free model proposal and a human edit.
// It never preserves approval after content changes, including effect changes.
func (s Store) ReviseTaskPlan(id, request string, revision uint64, digest string, content PlanContent) (RequestJournal, error) {
	j, index, err := s.taskPlanRequest(id, request, revision, digest)
	if err != nil {
		return j, err
	}
	r := &j.Requests[index]
	if err := s.CheckPlanBinding(id, *r); err != nil {
		return j, err
	}
	if r.State != "queued" || r.Plan.Phase != "preparing" && r.Plan.Phase != "review" && r.Plan.Phase != "approved" || revision == math.MaxUint64 || !validPlanContent(content, r.Plan.Sources) {
		return j, ErrPlanChanged
	}
	// Detach content and slices from caller/UI buffers before computing approval.
	raw, _ := json.Marshal(content)
	var retained PlanContent
	_ = json.Unmarshal(raw, &retained)
	p := *r.Plan
	p.Revision++
	p.Content = &retained
	p.Phase = "review"
	p.Approval = nil
	p.Digest = p.contentDigest()
	r.Plan = &p
	r.UpdatedAt = time.Now().UTC()
	return s.SaveRequests(id, j)
}

// ApproveTaskPlan is idempotent for the same exact review. Its boolean is true
// only for a new queued approval; a duplicate cannot schedule a second dispatch.
func (s Store) ApproveTaskPlan(id, request string, revision uint64, digest string) (RequestJournal, bool, error) {
	j, index, err := s.taskPlanRequest(id, request, revision, digest)
	if err != nil {
		return j, false, err
	}
	r := &j.Requests[index]
	if err := s.checkPlanAttemptEffects(r.Plan); err != nil {
		return j, false, err
	}
	if len(r.Plan.Attempts) > 0 {
		if _, _, err := r.Plan.RemainingExecutionBudget(); err != nil {
			return j, false, err
		}
	}
	if r.Plan.Approval != nil && r.Plan.Approval.Revision == revision && r.Plan.Approval.Digest == digest {
		return j, false, nil
	}
	if err := s.CheckPlanBinding(id, *r); err != nil {
		return j, false, err
	}
	if r.State != "queued" || r.Plan.Phase != "review" || r.Plan.Content == nil {
		return j, false, ErrPlanChanged
	}
	p := *r.Plan
	p.Phase = "approved"
	p.Approval = &PlanApproval{Revision: revision, Digest: digest, At: time.Now().UTC()}
	r.Plan = &p
	r.UpdatedAt = p.Approval.At
	next, err := s.SaveRequests(id, j)
	return next, err == nil, err
}
func (s Store) CancelTaskPlan(id, request string, revision uint64, digest string) (RequestJournal, error) {
	j, index, err := s.taskPlanRequest(id, request, revision, digest)
	if err != nil {
		return j, err
	}
	r := &j.Requests[index]
	if r.State == "discarded" {
		return j, nil
	}
	if r.State != "queued" && r.State != "interrupted" {
		return j, errors.New("this plan has already dispatched; use Stop to request cancellation")
	}
	wasInterrupted := r.State == "interrupted"
	r.State = "discarded"
	r.UpdatedAt = time.Now().UTC()
	p := *r.Plan
	p.Phase = "interrupted"
	if p.Edit != nil && p.Edit.State == "preparing" {
		p.Edit = p.Edit.clone()
		p.Edit.State, p.Edit.Issue = "cancelled", "Plan revision cancelled before implementation."
	}
	if !wasInterrupted {
		p.Approval = nil
	}
	r.Plan = &p
	return s.SaveRequests(id, j)
}

// CheckPlanDispatch is rechecked under the execution lease and again at the
// Work boundary. Inspecting or approving a plan does not dispatch anything.
func (s Store) CheckPlanDispatch(id string, r Request) error {
	if r.Plan != nil {
		if err := s.checkPlanAttemptEffects(r.Plan); err != nil {
			return err
		}
	}
	if err := s.CheckPlanBinding(id, r); err != nil {
		return err
	}
	if r.Plan.Content == nil || r.Plan.Approval == nil || !(r.Plan.Phase == "approved" && r.State == "queued" || r.Plan.Phase == "dispatching" && r.State == "running") {
		return ErrPlanChanged
	}
	return nil
}
func (p *TaskPlan) Clone() *TaskPlan {
	if p == nil {
		return nil
	}
	raw, _ := json.Marshal(p)
	var next TaskPlan
	_ = json.Unmarshal(raw, &next)
	return &next
}

func newTaskPlan(c Conversation, r Request, sources []string, budget PlanBudget) *TaskPlan {
	return &TaskPlan{Version: 1, Phase: "preparing", Sources: append([]string{}, sources...), Budget: budget, Binding: PlanBinding{UpdatedAt: c.UpdatedAt, ConversationID: c.ID, RequestID: r.ID, HeadRevision: c.HeadRevision, SnapshotID: c.SnapshotID, SourceSHA256: planHash([]byte(c.SourcePath)), OptionsSHA256: optionsHash(r.Options), PromptSHA256: planHash([]byte(r.Text))}}
}

// BeginQueuedTaskPlan preserves an already-admitted queue entry and its frozen
// options. Its context boundary is captured only when planning actually starts.
func (s Store) BeginQueuedTaskPlan(id, request string, sources []string, budget PlanBudget) (RequestJournal, Request, error) {
	c, err := s.Load(id)
	if err != nil {
		return RequestJournal{}, Request{}, err
	}
	j, err := s.LoadRequests(id)
	if err != nil {
		return j, Request{}, err
	}
	for i, r := range j.Requests {
		if r.ID != request {
			continue
		}
		if r.State != "queued" {
			return j, r, ErrPlanChanged
		}
		if r.Plan != nil {
			return j, r, nil
		}
		r.Plan = newTaskPlan(c, r, sources, budget)
		r.UpdatedAt = time.Now().UTC()
		j.Requests[i] = r
		next, err := s.SaveRequests(id, j)
		return next, r, err
	}
	return j, Request{}, ErrPlanChanged
}
