package workrun

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/executiontarget"
	"github.com/gongahkia/gator/internal/workspace"
)

const inspectionLimit = 64 * 1024

var inspectionActorID = regexp.MustCompile(`^inspection-[a-f0-9]{64}$`)

// Inspection selects public progress without copying protocol payloads, tool
// arguments/results, interaction previews or control metadata. Narration is
// untrusted user-work content and may itself mention sensitive information.
type Inspection struct {
	Target           executiontarget.Target `json:"target"`
	RunID            string                 `json:"run_id"`
	ConversationID   string                 `json:"conversation_id,omitempty"`
	Status           string                 `json:"status"`
	StartedAt        time.Time              `json:"started_at"`
	ObservedAt       time.Time              `json:"observed_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	Progress         []string               `json:"progress"`
	Specialists      []InspectionSpecialist `json:"specialists"`
	PendingApprovals int                    `json:"pending_approvals"`
	Limitations      []string               `json:"limitations"`
	Controls         []ControlOffer         `json:"controls,omitempty"`
}

type InspectionSpecialist struct {
	ID              string `json:"id"`
	Role            string `json:"role"`
	Status          string `json:"status"`
	NativeStatus    string `json:"native_status,omitempty"`
	PendingQuestion bool   `json:"pending_question"`
}

type InspectionResponse struct {
	Targets     []Inspection `json:"targets"`
	Unavailable bool         `json:"unavailable,omitempty"`
	Reason      string       `json:"reason,omitempty"`
	Truncated   bool         `json:"truncated,omitempty"`
}

// Live data belongs to the existing execution owner. Grants disappear with it.
type liveInspection struct {
	mu       sync.Mutex
	value    Inspection
	grants   map[string]executiontarget.Grant
	pending  func() int
	controls *remoteControls
}

func newLiveInspection(run, conversation string, started time.Time, pending func() int) *liveInspection {
	return &liveInspection{value: Inspection{RunID: run, ConversationID: conversation, Status: "running", StartedAt: started, UpdatedAt: started,
		Progress: []string{}, Specialists: []InspectionSpecialist{}, Limitations: []string{"Work progress only; no PTY output or native terminal transcript. Controls require separate operation grants."}}, grants: map[string]executiontarget.Grant{}, pending: pending}
}

func inspectionText(s string) string {
	if len(s) > 512 {
		s = s[:512]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return strings.ToValidUTF8(s, "�")
}

func (l *liveInspection) record(event agent.Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.value.UpdatedAt = time.Now().UTC()
	var text string
	switch event.Kind {
	case agent.EventText:
		if event.TaskID == "" {
			text = inspectionText(event.Text)
		}
	case agent.EventToolCalled, agent.EventToolFinished:
		if event.TaskID == "" && event.ToolCall != nil {
			text = inspectionText("Tool: " + event.ToolCall.Name)
		}
	case agent.EventSubagent:
		var task struct {
			ID        string `json:"id"`
			GlobalID  string `json:"global_id"`
			ParentRun string `json:"parent_run"`
			Role      string `json:"role"`
			Status    string `json:"status"`
			Native    *struct {
				Status string `json:"status"`
			} `json:"native"`
		}
		if json.Unmarshal([]byte(event.Text), &task) == nil && task.ParentRun == l.value.RunID && task.GlobalID == task.ParentRun+"/"+task.ID {
			item := InspectionSpecialist{ID: inspectionText(task.ID), Role: inspectionText(task.Role), Status: inspectionText(task.Status)}
			if task.Native != nil {
				item.NativeStatus = inspectionText(task.Native.Status)
				item.PendingQuestion = task.Native.Status == "awaiting_input"
			}
			for i := range l.value.Specialists {
				if l.value.Specialists[i].ID == item.ID {
					l.value.Specialists[i] = item
					return
				}
			}
			if len(l.value.Specialists) < 16 {
				l.value.Specialists = append(l.value.Specialists, item)
			}
		}
	}
	if text != "" {
		l.value.Progress = append(l.value.Progress, text)
		if len(l.value.Progress) > 24 {
			l.value.Progress = l.value.Progress[1:]
		}
	}
}

func (l *liveInspection) install(ctx context.Context, mux *http.ServeMux, authorized func(http.HandlerFunc) http.HandlerFunc, binding executiontarget.Binding) {
	l.value.Target = executiontarget.Target{Binding: binding, Label: "Work " + inspectionText(l.value.RunID), Capabilities: []executiontarget.Operation{executiontarget.Inspect}, Available: true}
	if l.controls != nil {
		l.controls.install(ctx, mux, authorized, binding)
	}
	// Local administrative access uses the pre-existing private control token.
	// This route is never forwarded by the companion HTTP server.
	mux.HandleFunc("/inspection-access", authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", 405)
			return
		}
		var input struct {
			Actor   string `json:"actor"`
			Seconds int    `json:"seconds"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || !inspectionActorID.MatchString(input.Actor) || input.Seconds < 0 || input.Seconds > 900 {
			http.Error(w, "invalid grant", 400)
			return
		}
		if decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "invalid grant", 400)
			return
		}
		l.mu.Lock()
		defer l.mu.Unlock()
		if ctx.Err() != nil {
			http.Error(w, "owner stopped", 409)
			return
		}
		now := time.Now().UTC()
		for id, g := range l.grants {
			if !now.Before(g.ExpiresAt) {
				delete(l.grants, id)
			}
		}
		if input.Seconds == 0 {
			delete(l.grants, input.Actor)
		} else {
			if _, exists := l.grants[input.Actor]; !exists && len(l.grants) >= 32 {
				http.Error(w, "grant limit", 409)
				return
			}
			l.grants[input.Actor] = executiontarget.Grant{Actor: executiontarget.Actor{ID: input.Actor, Kind: executiontarget.Human, Origin: executiontarget.Remote}, Target: binding, Operation: executiontarget.Inspect, ExpiresAt: now.Add(time.Duration(input.Seconds) * time.Second)}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("/inspection", authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method", 405)
			return
		}
		q := r.URL.Query()
		if len(q) != 3 || len(q["actor"]) != 1 || len(q["target"]) != 1 || len(q["generation"]) != 1 {
			http.Error(w, "invalid inspection", 400)
			return
		}
		actor := executiontarget.Actor{ID: q.Get("actor"), Kind: executiontarget.Human, Origin: executiontarget.Remote}
		expected := binding
		expected.ID = q.Get("target")
		expected.Generation = q.Get("generation")
		pending := 0
		if l.pending != nil {
			pending = l.pending()
		}
		var offers []ControlOffer
		if l.controls != nil {
			offers = l.controls.offers(ctx, actor)
		}
		l.mu.Lock()
		defer l.mu.Unlock()
		current := l.value.Target
		current.Available = ctx.Err() == nil
		if !current.Available {
			current.UnavailableReason = "Owner stopped"
		}
		if err := executiontarget.Check(time.Now(), actor, expected, current, executiontarget.Inspect, l.grants[actor.ID], nil); err != nil {
			http.Error(w, "inspection unavailable or unauthorized", 403)
			return
		}
		value := l.value
		value.ObservedAt = time.Now().UTC()
		value.PendingApprovals = pending
		value.Controls = offers
		payload, err := json.Marshal(value)
		if err != nil || len(payload) > inspectionLimit {
			http.Error(w, "inspection exceeds bounds", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(payload)
	}))
}

// GrantWorkInspection is a local-owner operation, not a remote companion API.
func GrantWorkInspection(ctx context.Context, stateDir, run, actor string, seconds int) error {
	if !inspectionActorID.MatchString(actor) || seconds < 0 || seconds > 900 {
		return errors.New("invalid inspection grant")
	}
	client, state, err := connectWorkControl(ctx, stateDir, run)
	if err != nil {
		return err
	}
	if state.Inspection == nil {
		return errors.New("owner does not support inspection")
	}
	body, _ := json.Marshal(struct {
		Actor   string `json:"actor"`
		Seconds int    `json:"seconds"`
	}{actor, seconds})
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, state.URL+"/inspection-access", strings.NewReader(string(body)))
	r.Header.Set("Authorization", "Bearer "+state.Token)
	response, err := client.Do(r)
	if err != nil {
		return errors.New("owner unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return errors.New("owner refused inspection grant")
	}
	return nil
}

// InspectOwnedWork scans a bounded set of private control records. Retained
// metadata only locates an owner; nonce verification and owner authorization are
// required before any execution data can leave this process. It starts no worker.
func InspectOwnedWork(ctx context.Context, stateDir, actor, target, generation string) (InspectionResponse, error) {
	result := InspectionResponse{Targets: []Inspection{}}
	if !inspectionActorID.MatchString(actor) {
		return result, errors.New("invalid inspection actor")
	}
	if target != "" && (!regexp.MustCompile(`^target-[a-f0-9]{32}$`).MatchString(target) || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(generation)) {
		return result, errors.New("invalid inspection target")
	}
	if target == "" && generation != "" {
		return result, errors.New("unexpected generation")
	}
	directory := filepath.Join(stateDir, "gator", "work-control")
	file, err := os.Open(directory)
	if errors.Is(err, os.ErrNotExist) {
		if target != "" {
			result.Unavailable = true
			result.Reason = "Owner unavailable or access withdrawn"
		}
		return result, nil
	}
	if err != nil {
		return result, errors.New("inspection discovery unavailable")
	}
	defer file.Close()
	names, err := file.Readdirnames(129)
	if err != nil && err != io.EOF {
		return result, errors.New("inspection discovery unavailable")
	}
	if len(names) > 128 {
		names = names[:128]
		result.Truncated = true
	}
	sort.Strings(names)
	root, err := workspace.Open(directory)
	if err != nil {
		return result, err
	}
	for _, name := range names {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		run := strings.TrimSuffix(name, ".json")
		if name != run+".json" || !validInteractionRunID(run) {
			continue
		}
		data, err := root.ReadRegularFile(name, 4096)
		if err != nil {
			continue
		}
		var hint workControlState
		if json.Unmarshal(data, &hint) != nil || hint.Inspection == nil || hint.Inspection.Validate() != nil || hint.Inspection.Backend != executiontarget.Work {
			continue
		}
		if target != "" && (hint.Inspection.ID != target || hint.Inspection.Generation != generation) {
			continue
		}
		client, owner, err := connectWorkControl(ctx, stateDir, run)
		if err != nil || owner.Inspection == nil || *owner.Inspection != *hint.Inspection {
			continue
		}
		q := url.Values{"actor": {actor}, "target": {owner.Inspection.ID}, "generation": {owner.Inspection.Generation}}
		r, _ := http.NewRequestWithContext(ctx, http.MethodGet, owner.URL+"/inspection?"+q.Encode(), nil)
		r.Header.Set("Authorization", "Bearer "+owner.Token)
		response, err := client.Do(r)
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, inspectionLimit+1))
		response.Body.Close()
		var value Inspection
		if readErr != nil || len(body) > inspectionLimit || response.StatusCode != 200 || json.Unmarshal(body, &value) != nil || value.Target.Binding != *owner.Inspection || value.RunID != run || value.Target.Validate() != nil || !value.Target.Available {
			continue
		}
		// Grants remain private and authoritative in the live owner. Validate the
		// returned inspection capability without granting any additional operation.
		if !value.Target.Supports(executiontarget.Inspect) {
			continue
		}
		if target == "" {
			value.Progress = []string{}
			value.Specialists = []InspectionSpecialist{}
		}
		result.Targets = append(result.Targets, value)
		if target != "" {
			return result, nil
		}
		if len(result.Targets) == 16 {
			result.Truncated = true
			break
		}
	}
	if target != "" {
		result.Unavailable = true
		result.Reason = "Owner unavailable or access withdrawn"
	}
	return result, nil
}
