package goal

import "errors"

// Cross-project links grant only the linked step's reported status to a scoped
// agenda/watch. They do not expose another project's notes, context or files.
func (s Store) DependencyStates(plan Plan) (map[string]StepStatus, error) {
	states := map[string]StepStatus{}
	plans := map[string]Plan{plan.GoalID: plan}
	for _, step := range plan.Steps {
		states[step.ID] = step.Status
	}
	for _, step := range plan.Steps {
		for _, ref := range step.DependsOn {
			id, target := dependencyTarget(plan.GoalID, ref)
			other, ok := plans[id]
			if !ok {
				var err error
				other, err = s.LoadPlan(id)
				if err != nil {
					return nil, err
				}
				plans[id] = other
			}
			found := false
			for _, candidate := range other.Steps {
				if candidate.ID == target {
					states[ref], found = candidate.Status, true
					break
				}
			}
			if !found {
				return nil, errors.New("linked dependency is missing; review the plan")
			}
		}
	}
	return states, nil
}

// Called while holding the global graph write lock. Validate removals as well
// as new edges so two concurrent edits cannot introduce a cross-plan cycle.
func (s Store) validateDependencyGraph(candidate Plan) error {
	records, err := s.List()
	if err != nil {
		return err
	}
	if len(records) > 1024 {
		return errors.New("dependency graph exceeds 1024 goals")
	}
	edges := map[string][]string{}
	for _, record := range records {
		plan := candidate
		if record.ID != candidate.GoalID {
			plan, err = s.loadPlan(record.ID)
			if err != nil {
				return err
			}
		}
		for _, step := range plan.Steps {
			key := record.ID + "/" + step.ID
			edges[key] = []string{}
			for _, ref := range step.DependsOn {
				id, target := dependencyTarget(record.ID, ref)
				edges[key] = append(edges[key], id+"/"+target)
			}
		}
	}
	visited, visiting := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(key string) error {
		if visiting[key] {
			return errors.New("cross-project dependencies contain a cycle")
		}
		links, exists := edges[key]
		if !exists {
			return errors.New("dependency step is missing; unlink it before removal")
		}
		if visited[key] {
			return nil
		}
		visiting[key] = true
		for _, link := range links {
			if err := visit(link); err != nil {
				return err
			}
		}
		visiting[key], visited[key] = false, true
		return nil
	}
	for key := range edges {
		if err := visit(key); err != nil {
			return err
		}
	}
	return nil
}
