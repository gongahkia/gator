package goal

import (
	"errors"
	"sort"
	"time"
)

type AgendaItem struct {
	GoalID       string   `json:"goal_id"`
	GoalTitle    string   `json:"goal_title"`
	PlanRevision int      `json:"plan_revision"`
	Step         Step     `json:"step"`
	Overdue      bool     `json:"overdue"`
	Scheduled    bool     `json:"scheduled"`
	BlockedBy    []string `json:"blocked_by,omitempty"`
	Conflicts    []string `json:"conflicts,omitempty"`
}

type Agenda struct {
	AsOf      time.Time    `json:"as_of"`
	Until     time.Time    `json:"until"`
	Items     []AgendaItem `json:"items"`
	Total     int          `json:"total"`
	Truncated bool         `json:"truncated"`
}

// Agenda derives a bounded view from the plan store; it never infers progress or
// writes a second task database. Undated and overdue work remains visible.
func (s Store) Agenda(scope string, now, until time.Time) (Agenda, error) {
	result := Agenda{AsOf: now, Until: until, Items: []AgendaItem{}}
	if now.IsZero() || !until.After(now) || until.Sub(now) > 31*24*time.Hour {
		return result, errors.New("agenda window must be between now and 31 days ahead")
	}
	var records []Record
	if scope != "" {
		r, err := s.Load(scope)
		if err != nil {
			return result, err
		}
		records = []Record{r}
	} else {
		var err error
		records, err = s.List()
		if err != nil {
			return result, err
		}
	}
	// Avoid silent partial scanning; pagination can be added if this limit is reached.
	if len(records) > 1024 {
		return result, errors.New("too many goals for a global agenda; select a goal")
	}
	for _, r := range records {
		if r.Status != Active {
			continue
		}
		plan, err := s.LoadPlan(r.ID)
		if err != nil {
			return result, err
		}
		status := map[string]StepStatus{}
		for _, step := range plan.Steps {
			status[step.ID] = step.Status
		}
		for _, step := range plan.Steps {
			if step.Status == StepDone {
				continue
			}
			// Keep overdue, undated and scheduled-within-window work; an expired block
			// remains visible as unfinished rather than silently becoming completed.
			if (step.DueAt.IsZero() || step.DueAt.After(until)) && !step.StartsAt.IsZero() && step.StartsAt.After(until) {
				continue
			}
			if step.StartsAt.IsZero() && !step.DueAt.IsZero() && step.DueAt.After(until) {
				continue
			}
			item := AgendaItem{GoalID: r.ID, GoalTitle: r.Title, PlanRevision: plan.Revision, Step: step, Overdue: !step.DueAt.IsZero() && step.DueAt.Before(now), Scheduled: !step.StartsAt.IsZero()}
			for _, dep := range step.DependsOn {
				if status[dep] != StepDone {
					item.BlockedBy = append(item.BlockedBy, dep)
				}
			}
			result.Items = append(result.Items, item)
		}
	}
	sort.Slice(result.Items, func(i, j int) bool {
		a, b := result.Items[i], result.Items[j]
		if a.Overdue != b.Overdue {
			return a.Overdue
		}
		at, bt := agendaTime(a.Step), agendaTime(b.Step)
		if !at.Equal(bt) {
			if at.IsZero() {
				return false
			}
			if bt.IsZero() {
				return true
			}
			return at.Before(bt)
		}
		if a.GoalID != b.GoalID {
			return a.GoalID < b.GoalID
		}
		return a.Step.ID < b.Step.ID
	})
	result.Total = len(result.Items)
	if len(result.Items) > 128 {
		result.Items = result.Items[:128]
		result.Truncated = true
	}
	// Report overlap only for future/current time blocks, using half-open intervals.
	for i := range result.Items {
		a := &result.Items[i]
		if !a.Step.EndsAt.After(now) {
			continue
		}
		for j := range result.Items {
			b := result.Items[j]
			if i != j && b.Step.EndsAt.After(now) && a.Step.StartsAt.Before(b.Step.EndsAt) && b.Step.StartsAt.Before(a.Step.EndsAt) {
				a.Conflicts = append(a.Conflicts, b.GoalID+"/"+b.Step.ID)
			}
		}
	}
	return result, nil
}
func agendaTime(s Step) time.Time {
	if !s.StartsAt.IsZero() {
		return s.StartsAt
	}
	return s.DueAt
}
