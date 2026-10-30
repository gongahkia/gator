package goal

import (
	"errors"
	"strings"
	"time"
)

func validateSchedule(start, end time.Time, dependencies []string) error {
	if start.IsZero() != end.IsZero() || !start.IsZero() && (!end.After(start) || end.Sub(start) > 31*24*time.Hour) {
		return errors.New("scheduled steps need an explicit start and later end, at most 31 days apart")
	}
	if len(dependencies) > 8 {
		return errors.New("a step supports at most eight dependencies")
	}
	seen := map[string]bool{}
	for _, id := range dependencies {
		if !validDependency(id) || seen[id] {
			return errors.New("invalid or duplicate step dependency")
		}
		seen[id] = true
	}
	return nil
}

func validateDependencies(steps []Step) error {
	byID := map[string]Step{}
	for _, s := range steps {
		byID[s.ID] = s
	}
	visited, visiting := map[string]bool{}, map[string]bool{}
	var walk func(string) error
	walk = func(id string) error {
		if visiting[id] {
			return errors.New("step dependencies contain a cycle")
		}
		step, exists := byID[id]
		if !exists {
			return errors.New("dependency step is missing; clear references before removing it")
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dep := range step.DependsOn {
			if strings.Contains(dep, "/") {
				continue
			} // Checked across plans by the store.
			if err := walk(dep); err != nil {
				return err
			}
		}
		visiting[id], visited[id] = false, true
		return nil
	}
	for _, s := range steps {
		if err := walk(s.ID); err != nil {
			return err
		}
	}
	return nil
}

func validDependency(ref string) bool {
	parts := strings.Split(ref, "/")
	return len(parts) == 1 && validStepID(ref) || len(parts) == 2 && validID(parts[0]) && validStepID(parts[1])
}

func dependencyTarget(goalID, ref string) (string, string) {
	if g, step, ok := strings.Cut(ref, "/"); ok {
		return g, step
	}
	return goalID, ref
}
