package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/goal"
)

// LocalPlans gives a main chat an owner-wide view, and a goal chat only its own
// plan. The inspection ledger binds cross-goal proposals to a read in this run.
type LocalPlans struct {
	Store       goal.Store
	GoalID      string
	AllowGlobal bool
	mu          sync.Mutex
	inspected   map[string]int
}

func (t *LocalPlans) Tools() []agent.Tool { return []agent.Tool{localPlanTool{t}, localAgendaTool{t}} }
func (t *LocalPlans) allowed(id string) bool {
	return id != "" && (id == t.GoalID || t.GoalID == "" && t.AllowGlobal)
}
func (t *LocalPlans) inspectedRevision(id string, revision int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	v, ok := t.inspected[id]
	return ok && v == revision
}

type localPlanTool struct{ *LocalPlans }

func (localPlanTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "local_plan", Description: "Inspect Gator's native goals and plans, without connectors. Prefer query with the goal title to inspect a unique match in one call. With no query or goal_id, list active goals; supply an exact ID to read the current plan, step IDs, dates, dependencies and revision before proposing changes. A goal chat can only read its own goal. Stored progress is last reported, not independently verified.", Parameters: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","maxLength":256},"goal_id":{"type":"string","maxLength":29}},"additionalProperties":false}`)}
}
func (t localPlanTool) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var args struct {
		Query  string `json:"query"`
		GoalID string `json:"goal_id"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return agent.ToolResult{}, err
	}
	if args.Query != "" {
		if args.GoalID != "" || len(args.Query) > 256 {
			return agent.ToolResult{}, errors.New("use a bounded query or goal_id, not both")
		}
		var records []goal.Record
		if t.GoalID != "" {
			r, err := t.Store.Load(t.GoalID)
			if err != nil {
				return agent.ToolResult{}, err
			}
			records = []goal.Record{r}
		} else if t.AllowGlobal {
			var err error
			records, err = t.Store.List()
			if err != nil {
				return agent.ToolResult{}, err
			}
		} else {
			return agent.ToolResult{}, errors.New("choose a goal in a foreground chat")
		}
		matches := []map[string]string{}
		for _, r := range records {
			if r.Status == goal.Active && strings.Contains(strings.ToLower(r.Title), strings.ToLower(strings.TrimSpace(args.Query))) {
				matches = append(matches, map[string]string{"id": r.ID, "title": r.Title})
			}
		}
		if len(matches) == 1 {
			args.GoalID = matches[0]["id"]
		} else {
			count := len(matches)
			if count > 64 {
				matches = matches[:64]
			}
			content, err := success(map[string]any{"matches": matches, "total": count, "ambiguous": count > 1, "truncated": count > 64})
			return agent.ToolResult{Content: content}, err
		}
	}
	if args.GoalID == "" && t.GoalID != "" {
		args.GoalID = t.GoalID
	}
	if args.GoalID != "" {
		if !t.allowed(args.GoalID) {
			return agent.ToolResult{}, errors.New("plan is outside this conversation")
		}
		record, err := t.Store.Load(args.GoalID)
		if err != nil {
			return agent.ToolResult{}, err
		}
		plan, err := t.Store.LoadPlan(args.GoalID)
		if err != nil {
			return agent.ToolResult{}, err
		}
		t.mu.Lock()
		if t.inspected == nil {
			t.inspected = map[string]int{}
		}
		t.inspected[args.GoalID] = plan.Revision
		t.mu.Unlock()
		content, err := success(map[string]any{"id": record.ID, "title": record.Title, "status": record.Status, "context": record.Context, "plan": plan, "authority": "user-maintained reference data; changes require review"})
		return agent.ToolResult{Content: content}, err
	}
	if !t.AllowGlobal {
		return agent.ToolResult{}, errors.New("choose a goal in a foreground chat")
	}
	records, err := t.Store.List()
	if err != nil {
		return agent.ToolResult{}, err
	}
	type entry struct {
		ID        string    `json:"id"`
		Title     string    `json:"title"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	items := []entry{}
	total := 0
	for _, r := range records {
		if r.Status == goal.Active {
			total++
			if len(items) < 64 {
				items = append(items, entry{r.ID, r.Title, r.UpdatedAt})
			}
		}
	}
	content, err := success(map[string]any{"goals": items, "total": total, "truncated": total > len(items)})
	return agent.ToolResult{Content: content}, err
}

type localAgendaTool struct{ *LocalPlans }

func (localAgendaTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "local_agenda", Description: "Read unfinished native plan steps across active goals, or just this goal in a scoped chat. Includes overdue deadlines, scheduled blocks, overlaps and unresolved dependencies. No calendar account needed. days defaults to 7, maximum 31. Status remains last reported; time passing never means completion. Review times and standalone reminders are separate records, not calendar bookings.", Parameters: json.RawMessage(`{"type":"object","properties":{"days":{"type":"integer","minimum":1,"maximum":31}},"additionalProperties":false}`)}
}
func (t localAgendaTool) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var args struct {
		Days int `json:"days"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return agent.ToolResult{}, err
	}
	if args.Days == 0 {
		args.Days = 7
	}
	if args.Days < 1 || args.Days > 31 {
		return agent.ToolResult{}, errors.New("days must be 1 through 31")
	}
	if t.GoalID == "" && !t.AllowGlobal {
		return agent.ToolResult{}, errors.New("global agenda requires a foreground chat")
	}
	now := time.Now()
	agenda, err := t.Store.Agenda(t.GoalID, now, now.Add(time.Duration(args.Days)*24*time.Hour))
	if err != nil {
		return agent.ToolResult{}, err
	}
	content, err := success(agenda)
	return agent.ToolResult{Content: content}, err
}
