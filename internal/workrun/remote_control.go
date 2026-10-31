package workrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/actionlog"
	"github.com/gongahkia/gator/internal/cliagent"
	"github.com/gongahkia/gator/internal/executiontarget"
	"github.com/gongahkia/gator/internal/orchestrator"
)

var controlToken = regexp.MustCompile(`^[a-f0-9]{32}$`)
var controlDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

// RemoteAction is a finite semantic request, never a terminal input envelope.
type RemoteAction struct {
	ID            string                    `json:"id"`
	RunID         string                    `json:"run_id"`
	Target        executiontarget.Binding   `json:"target"`
	Operation     executiontarget.Operation `json:"operation"`
	Specialist    string                    `json:"specialist,omitempty"`
	Question      string                    `json:"question,omitempty"`
	Instruction   string                    `json:"instruction,omitempty"`
	Answers       map[string][]string       `json:"answers,omitempty"`
	DecisionID    int                       `json:"decision_id,omitempty"`
	PreviewDigest string                    `json:"preview_digest,omitempty"`
	Approved      *bool                     `json:"approved,omitempty"`
}

func (a RemoteAction) scope() string {
	switch a.Operation {
	case executiontarget.Steer:
		return a.Specialist
	case executiontarget.Answer:
		return a.Specialist + "/" + a.Question
	case executiontarget.Decision:
		return strconv.Itoa(a.DecisionID) + "/" + a.PreviewDigest
	}
	return ""
}
func (a RemoteAction) digest(actor executiontarget.Actor) string {
	bytes, _ := json.Marshal(struct {
		Actor  executiontarget.Actor
		Action RemoteAction
	}{actor, a})
	return hashText(string(bytes))
}
func validInput(s string) bool {
	return strings.TrimSpace(s) != "" && len(s) <= 8192 && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
func (a RemoteAction) Validate() error {
	if !actionlog.ControlID.MatchString(a.ID) || !validInteractionRunID(a.RunID) || a.Target.Validate() != nil || a.Target.Backend != executiontarget.Work {
		return errors.New("invalid action identity")
	}
	if a.Operation != executiontarget.Steer && a.Instruction != "" || a.Operation != executiontarget.Answer && (a.Question != "" || len(a.Answers) > 0) || a.Operation != executiontarget.Decision && (a.DecisionID != 0 || a.PreviewDigest != "" || a.Approved != nil) || a.Operation == executiontarget.Cancel && a.Specialist != "" {
		return errors.New("unrelated action fields")
	}
	switch a.Operation {
	case executiontarget.Cancel:
	case executiontarget.Steer:
		if !validSpecialistTaskID(a.Specialist) || !validInput(a.Instruction) {
			return errors.New("invalid specialist steering")
		}
	case executiontarget.Answer:
		if !validSpecialistTaskID(a.Specialist) || !controlDigest.MatchString(a.Question) || len(a.Answers) < 1 || len(a.Answers) > 8 {
			return errors.New("invalid specialist question")
		}
		for k, v := range a.Answers {
			if len(k) > 128 || len(k) == 0 || len(v) != 1 || !validInput(v[0]) {
				return errors.New("invalid answer")
			}
		}
	case executiontarget.Decision:
		if a.Specialist != "" || a.DecisionID < 1 || !controlDigest.MatchString(a.PreviewDigest) || a.Approved == nil {
			return errors.New("invalid approval decision")
		}
	default:
		return errors.New("unsupported remote operation")
	}
	bytes, err := json.Marshal(a)
	if err != nil || len(bytes) > 24*1024 {
		return errors.New("action exceeds bounds")
	}
	return nil
}

type ControlOffer struct {
	Operation     executiontarget.Operation `json:"operation"`
	Scope         string                    `json:"scope"`
	ExpiresAt     time.Time                 `json:"expires_at"`
	Specialist    string                    `json:"specialist,omitempty"`
	Question      string                    `json:"question,omitempty"`
	Questions     []cliagent.NativeQuestion `json:"questions,omitempty"`
	DecisionID    int                       `json:"decision_id,omitempty"`
	PreviewDigest string                    `json:"preview_digest,omitempty"`
	Preview       []string                  `json:"preview,omitempty"`
}
type ControlAccess struct {
	Actor     string                    `json:"actor"`
	Operation executiontarget.Operation `json:"operation"`
	Scope     string                    `json:"scope"`
	Seconds   int                       `json:"seconds"`
	Authority CompanionAuthority        `json:"authority"`
}
type scopedControlGrant struct {
	Grant     executiontarget.Grant
	Scope     string
	Authority CompanionAuthority
}
type ownerControlInput struct {
	Actor  string       `json:"actor"`
	Action RemoteAction `json:"action"`
}
type ControlReply struct {
	Observation string                    `json:"observation,omitempty"`
	Authorized  bool                      `json:"authorized"`
	Receipt     *actionlog.ControlReceipt `json:"receipt,omitempty"`
	Error       string                    `json:"error,omitempty"`
}
type remoteControls struct {
	mu            sync.Mutex
	dispatchMu    sync.Mutex
	stateDir, run string
	binding       executiontarget.Binding
	grants        map[string]scopedControlGrant
	cancel        context.CancelFunc
	steer         func(string, string) error
	answer        func(string, string, map[string][]string) error
	respond       func(int, string, bool) error
	inspect       func(string) (orchestrator.Task, error)
	review        func(int) (Interaction, string, error)
	// Deterministic crash-window injection is private to package tests.
	afterClaim    func()
	afterDispatch func()
}

func newRemoteControls(state, run string, cancel context.CancelFunc, steer func(string, string) error, answer func(string, string, map[string][]string) error, respond func(int, string, bool) error, inspect func(string) (orchestrator.Task, error), review func(int) (Interaction, string, error)) *remoteControls {
	return &remoteControls{stateDir: state, run: run, grants: map[string]scopedControlGrant{}, cancel: cancel, steer: steer, answer: answer, respond: respond, inspect: inspect, review: review}
}

// Initial approval support deliberately covers bounded command argv only.
// No generic Preview object, truncated preview or model summary grants approval.
func (o *Operation) reviewRemoteDecision(id int) (Interaction, string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	p, ok := o.pending[id]
	if !ok || p.interaction.Kind != "command" {
		return Interaction{}, "", errors.New("unsupported or consumed approval")
	}
	argv, ok := p.interaction.Preview.([]string)
	if !ok || len(argv) == 0 || len(argv) > 32 {
		return Interaction{}, "", errors.New("unsupported approval preview")
	}
	// Synthetic tool permission argv is not a complete process review. In
	// particular, terminal_write contains only an input digest, never its text.
	switch argv[0] {
	case "terminal_write", "terminal_detach", "extension", "mcp", "lsp", "web_search", "http_fetch":
		return Interaction{}, "", errors.New("tool permission review is unsupported remotely")
	}
	for _, arg := range argv {
		if !utf8.ValidString(arg) || strings.ContainsRune(arg, 0) {
			return Interaction{}, "", errors.New("unsupported approval text")
		}
	}
	data, err := json.Marshal(argv)
	if err != nil || len(data) > 8192 {
		return Interaction{}, "", errors.New("approval preview exceeds bounds")
	}
	i := p.interaction
	i.Preview = append([]string(nil), argv...)
	if digest, err := InteractionDigest(i); err != nil || digest != p.digest {
		return Interaction{}, "", errors.New("approval preview changed")
	}
	return i, p.digest, nil
}

func controlGrantKey(actor string, op executiontarget.Operation, scope string) string {
	return actor + ":" + string(op) + ":" + scope
}
func (c *remoteControls) authorize(ctx context.Context, actor executiontarget.Actor, a RemoteAction) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.authorizeLocked(ctx, actor, a)
}
func (c *remoteControls) authorizeLocked(ctx context.Context, actor executiontarget.Actor, a RemoteAction) error {
	if ctx.Err() != nil || a.RunID != c.run || a.Target != c.binding {
		return errors.New("owner unavailable or changed")
	}
	g := c.grants[controlGrantKey(actor.ID, a.Operation, a.scope())]
	t := executiontarget.Target{Binding: c.binding, Label: "Owned Work", Available: true, Capabilities: []executiontarget.Operation{a.Operation}}
	if err := executiontarget.Check(time.Now(), actor, a.Target, t, a.Operation, g.Grant, nil); err != nil {
		return err
	}
	id, err := g.Authority.actor()
	if err != nil || id != actor.ID {
		return errors.New("device access revoked or changed")
	}
	return nil
}

func (c *remoteControls) preflight(a RemoteAction) (ControlOffer, error) {
	o := ControlOffer{Operation: a.Operation, Scope: a.scope(), Specialist: a.Specialist}
	switch a.Operation {
	case executiontarget.Cancel:
		if c.cancel == nil {
			return o, errors.New("cancellation unavailable")
		}
	case executiontarget.Steer, executiontarget.Answer:
		if c.inspect == nil {
			return o, errors.New("specialist unavailable")
		}
		t, err := c.inspect(a.Specialist)
		if err != nil || t.ParentRun != c.run || t.GlobalID != c.run+"/"+a.Specialist {
			return o, errors.New("specialist changed")
		}
		if a.Operation == executiontarget.Steer {
			if c.steer == nil || t.Role == "cli_claude" || (t.Status != "running" && t.Status != "queued") || t.Native != nil && t.Native.Status != "running" {
				return o, errors.New("specialist cannot be steered")
			}
		} else {
			if c.answer == nil || t.Status != "awaiting_input" || t.Native == nil || t.Native.QuestionID != a.Question {
				return o, errors.New("question changed or consumed")
			}
			o.Question = a.Question
			o.Questions = t.Native.Questions
			data, _ := json.Marshal(o.Questions)
			if len(data) > 8192 || len(o.Questions) > 8 || len(o.Questions) == 0 {
				return o, errors.New("question review exceeds bounds")
			}
			if a.Answers != nil {
				if len(a.Answers) != len(o.Questions) {
					return o, errors.New("answer set changed")
				}
				for _, q := range o.Questions {
					if len(a.Answers[q.ID]) != 1 || !validInput(a.Answers[q.ID][0]) {
						return o, errors.New("answer set changed")
					}
				}
			}
		}
	case executiontarget.Decision:
		if c.review == nil || c.respond == nil {
			return o, errors.New("approval unavailable")
		}
		i, d, err := c.review(a.DecisionID)
		if err != nil || d != a.PreviewDigest {
			return o, errors.New("approval changed or consumed")
		}
		argv, ok := i.Preview.([]string)
		if !ok {
			return o, errors.New("approval type unsupported")
		}
		o.DecisionID = i.ID
		o.PreviewDigest = d
		o.Preview = argv
	default:
		return o, errors.New("operation unsupported")
	}
	return o, nil
}

func (c *remoteControls) grant(ctx context.Context, input ControlAccess) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !inspectionActorID.MatchString(input.Actor) || input.Seconds < 0 || input.Seconds > 900 || ctx.Err() != nil {
		return errors.New("invalid control access")
	}
	switch input.Operation {
	case executiontarget.Cancel, executiontarget.Steer, executiontarget.Answer, executiontarget.Decision:
	default:
		return errors.New("operation is not grantable")
	}
	id, err := input.Authority.actor()
	if err != nil || id != input.Actor {
		return errors.New("device authority changed")
	}
	if input.Seconds == 0 {
		for key, g := range c.grants {
			if g.Grant.Actor.ID == input.Actor && g.Grant.Operation == input.Operation && (g.Scope == input.Scope || (input.Operation == executiontarget.Answer || input.Operation == executiontarget.Decision) && strings.HasPrefix(g.Scope, input.Scope+"/")) {
				delete(c.grants, key)
			}
		}
		return nil
	}
	a := RemoteAction{RunID: c.run, Target: c.binding, Operation: input.Operation}
	switch input.Operation {
	case executiontarget.Cancel:
		if input.Scope != "" {
			return errors.New("cancel scope must be empty")
		}
	case executiontarget.Steer:
		a.Specialist = input.Scope
	case executiontarget.Answer:
		if c.inspect == nil {
			return errors.New("specialist unavailable")
		}
		t, err := c.inspect(input.Scope)
		if err != nil || t.Native == nil {
			return errors.New("no pending native question")
		}
		a.Specialist = input.Scope
		a.Question = t.Native.QuestionID
	case executiontarget.Decision:
		n, err := strconv.Atoi(input.Scope)
		if err != nil || c.review == nil {
			return errors.New("invalid decision scope")
		}
		_, d, err := c.review(n)
		if err != nil {
			return err
		}
		a.DecisionID = n
		a.PreviewDigest = d
	default:
		return errors.New("operation is not grantable")
	}
	key := controlGrantKey(input.Actor, input.Operation, a.scope())
	if input.Seconds == 0 {
		delete(c.grants, key)
		return nil
	}
	if _, err := c.preflight(a); err != nil {
		return err
	}
	now := time.Now().UTC()
	for k, g := range c.grants {
		if !now.Before(g.Grant.ExpiresAt) {
			delete(c.grants, k)
		}
	}
	if _, exists := c.grants[key]; !exists && len(c.grants) >= 64 {
		return errors.New("control grant limit")
	}
	c.grants[key] = scopedControlGrant{Grant: executiontarget.Grant{Actor: executiontarget.Actor{ID: input.Actor, Kind: executiontarget.Human, Origin: executiontarget.Remote}, Target: c.binding, Operation: input.Operation, ExpiresAt: now.Add(time.Duration(input.Seconds) * time.Second)}, Scope: a.scope(), Authority: input.Authority}
	return nil
}

func actionForScope(op executiontarget.Operation, scope string) RemoteAction {
	a := RemoteAction{Operation: op}
	switch op {
	case executiontarget.Steer:
		a.Specialist = scope
	case executiontarget.Answer:
		a.Specialist, a.Question, _ = strings.Cut(scope, "/")
	case executiontarget.Decision:
		n, d, _ := strings.Cut(scope, "/")
		a.DecisionID, _ = strconv.Atoi(n)
		a.PreviewDigest = d
	}
	return a
}
func (c *remoteControls) offers(ctx context.Context, actor executiontarget.Actor) []ControlOffer {
	c.mu.Lock()
	grants := make([]scopedControlGrant, 0, len(c.grants))
	for _, g := range c.grants {
		grants = append(grants, g)
	}
	c.mu.Unlock()
	out := []ControlOffer{}
	for _, g := range grants {
		if g.Grant.Actor != actor {
			continue
		}
		a := actionForScope(g.Grant.Operation, g.Scope)
		a.RunID = c.run
		a.Target = c.binding
		if c.authorize(ctx, actor, a) != nil {
			continue
		}
		o, err := c.preflight(a)
		if err == nil {
			o.ExpiresAt = g.Grant.ExpiresAt
			candidate := append(append([]ControlOffer(nil), out...), o)
			data, _ := json.Marshal(candidate)
			if len(data) > 24*1024 {
				continue
			}
			out = candidate
			if len(out) >= 8 {
				break
			}
		}
	}
	return out
}

func (c *remoteControls) dispatch(ctx context.Context, actor executiontarget.Actor, a RemoteAction) (ControlReply, error) {
	if err := a.Validate(); err != nil {
		return ControlReply{}, err
	}
	c.dispatchMu.Lock()
	defer c.dispatchMu.Unlock()
	if err := c.authorize(ctx, actor, a); err != nil {
		return ControlReply{}, err
	}
	store, err := actionlog.Open(c.stateDir)
	if err != nil {
		return ControlReply{}, err
	}
	r, first, err := store.ClaimControl(actionlog.ControlReceipt{ID: a.ID, RunID: c.run, Target: a.Target, Actor: actor, Operation: a.Operation, Scope: a.scope(), Digest: a.digest(actor), CreatedAt: time.Now().UTC()})
	if err != nil {
		return ControlReply{}, err
	}
	if !first {
		return ControlReply{Receipt: &r}, nil
	}
	if c.afterClaim != nil {
		c.afterClaim()
	}
	delivery, note := executiontarget.NotDispatched, "Rejected before semantic dispatch."
	// Fresh authority and canonical semantic state, after durable admission.
	if c.authorize(ctx, actor, a) == nil {
		if _, err = c.preflight(a); err == nil {
			// No asynchronous gap between this final authority check and dispatch.
			// Grant revocation can run during durable admission/preflight. This
			// final critical section linearizes dispatch against local revocation.
			c.mu.Lock()
			if err = c.authorizeLocked(ctx, actor, a); err == nil {
				delivery = executiontarget.OutcomeUnknown
				note = "Semantic dispatch may have started; reconcile without replay."
				switch a.Operation {
				case executiontarget.Cancel:
					c.cancel()
					err = nil
				case executiontarget.Steer:
					err = c.steer(a.Specialist, a.Instruction)
				case executiontarget.Answer:
					err = c.answer(a.Specialist, a.Question, a.Answers)
				case executiontarget.Decision:
					err = c.respond(a.DecisionID, a.PreviewDigest, *a.Approved)
				}
				if c.afterDispatch != nil {
					c.afterDispatch()
				}
				if err == nil {
					delivery = executiontarget.DispatchAcknowledged
					note = "Accepted by the semantic control path; task completion is not established."
					if a.Operation == executiontarget.Cancel {
						note = "Cancellation requested; execution has not yet been observed stopped."
					}
				}
			}
			c.mu.Unlock()
		}
	}
	settled, finishErr := store.FinishControl(r, delivery, note)
	if finishErr != nil {
		return ControlReply{Receipt: &r}, nil
	}
	return ControlReply{Receipt: &settled}, nil
}

func decodeControl(w http.ResponseWriter, r *http.Request, value any) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 28*1024))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("trailing request data")
	}
	return nil
}
func (c *remoteControls) install(ctx context.Context, mux *http.ServeMux, authorized func(http.HandlerFunc) http.HandlerFunc, binding executiontarget.Binding) {
	c.binding = binding
	mux.HandleFunc("/control-access", authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method", 405)
			return
		}
		var input ControlAccess
		if decodeControl(w, r, &input) != nil || c.grant(ctx, input) != nil {
			http.Error(w, "control grant rejected", 403)
			return
		}
		w.WriteHeader(204)
	}))
	for _, path := range []string{"/remote-check", "/remote-action"} {
		path := path
		mux.HandleFunc(path, authorized(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				http.Error(w, "method", 405)
				return
			}
			var input ownerControlInput
			if decodeControl(w, r, &input) != nil || !inspectionActorID.MatchString(input.Actor) || input.Action.Validate() != nil {
				http.Error(w, "invalid semantic action", 400)
				return
			}
			actor := executiontarget.Actor{ID: input.Actor, Kind: executiontarget.Human, Origin: executiontarget.Remote}
			var reply ControlReply
			var err error
			if path == "/remote-check" {
				err = c.authorize(ctx, actor, input.Action)
				if err == nil {
					_, err = c.preflight(input.Action)
				}
				reply.Authorized = err == nil
			} else {
				reply, err = c.dispatch(ctx, actor, input.Action)
			}
			if err != nil {
				http.Error(w, "semantic action rejected", 403)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(reply)
		}))
	}
}

// The trusted local helper uses the existing verified Work channel. No caller
// can provide an endpoint, credential or owner substitution.
func GrantRemoteControl(ctx context.Context, stateDir, run string, input ControlAccess) error {
	client, state, err := connectWorkControl(ctx, stateDir, run)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(input)
	r, _ := http.NewRequestWithContext(ctx, "POST", state.URL+"/control-access", strings.NewReader(string(data)))
	r.Header.Set("Authorization", "Bearer "+state.Token)
	response, err := client.Do(r)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 204 {
		return errors.New("owner refused control grant")
	}
	return nil
}

func RouteRemoteControl(ctx context.Context, stateDir, actor, mode string, a RemoteAction) (ControlReply, error) {
	if !inspectionActorID.MatchString(actor) || a.Validate() != nil {
		return ControlReply{}, errors.New("invalid remote action")
	}
	principal := executiontarget.Actor{ID: actor, Kind: executiontarget.Human, Origin: executiontarget.Remote}
	store, err := actionlog.Open(stateDir)
	if err != nil {
		return ControlReply{}, err
	}
	if prior, readErr := store.ReadControl(a.RunID, a.Target, a.ID); readErr == nil {
		if prior.Actor != principal || prior.Digest != a.digest(principal) {
			return ControlReply{}, errors.New("action ID conflicts with retained request")
		}
		return ControlReply{Receipt: &prior}, nil
	} else if !errors.Is(readErr, io.EOF) && !errors.Is(readErr, os.ErrNotExist) {
		return ControlReply{}, readErr
	}
	if mode != "check" && mode != "dispatch" {
		return ControlReply{}, errors.New("invalid remote mode")
	}
	client, owner, err := connectWorkControl(ctx, stateDir, a.RunID)
	if err != nil {
		return ControlReply{}, err
	}
	if owner.Inspection == nil || *owner.Inspection != a.Target {
		return ControlReply{}, errors.New("target owner changed")
	}
	path := "/remote-check"
	if mode == "dispatch" {
		path = "/remote-action"
	}
	data, _ := json.Marshal(ownerControlInput{Actor: actor, Action: a})
	r, _ := http.NewRequestWithContext(ctx, "POST", owner.URL+path, strings.NewReader(string(data)))
	r.Header.Set("Authorization", "Bearer "+owner.Token)
	response, err := client.Do(r)
	if err != nil {
		return ControlReply{}, errors.New("owner response lost; retrieve receipt without resending")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return ControlReply{}, fmt.Errorf("owner refused semantic request (%d)", response.StatusCode)
	}
	var reply ControlReply
	decoder := json.NewDecoder(io.LimitReader(response.Body, 16384))
	if decoder.Decode(&reply) != nil {
		return ControlReply{}, errors.New("unconfirmed owner response")
	}
	return reply, nil
}
