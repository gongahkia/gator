package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gongahkia/gator/internal/agent"
	sharedusage "github.com/gongahkia/gator/internal/usage"
	"github.com/gongahkia/gator/internal/workapp"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

// Planning uses the selected model without a Work runner, filesystem capture,
// connector invocation, coding worker or permission-bearing tool surface.
func prepareDesktopTaskPlan(ctx context.Context, stateDir string, r worksession.Request) (worksession.PlanContent, error) {
	store, err := worksession.Open(stateDir)
	if err != nil {
		return worksession.PlanContent{}, err
	}
	if r.Plan == nil || r.State != "queued" || r.Plan.Phase != "preparing" {
		return worksession.PlanContent{}, worksession.ErrPlanChanged
	}
	if err := store.CheckPlanBinding(r.Plan.Binding.ConversationID, r); err != nil {
		return worksession.PlanContent{}, err
	}
	var options workapp.RunOptions
	if json.Unmarshal(r.Options, &options) != nil || !options.PlanFirst {
		return worksession.PlanContent{}, errors.New("invalid plan preparation settings")
	}
	defaults, err := configuredDefaults()
	if err != nil {
		return worksession.PlanContent{}, err
	}
	if options.ModelProvider != "" {
		defaults.Provider = options.ModelProvider
	}
	if options.ModelName != "" {
		defaults.Model = options.ModelName
	}
	provider, model, err := resolveConfiguredProvider(defaults.Provider, defaults.Model)
	if err != nil {
		return worksession.PlanContent{}, err
	}
	backend, err := nativeWorkModel(provider, model, "")
	if err != nil {
		return worksession.PlanContent{}, err
	}
	history := []agent.Message{}
	if r.Plan.Binding.HeadRevision != "" {
		lineage, err := store.Lineage(r.Plan.Binding.ConversationID, r.Plan.Binding.HeadRevision)
		if err != nil {
			return worksession.PlanContent{}, err
		}
		for _, turn := range lineage {
			history = append(history, agent.Message{Role: agent.RoleUser, Content: turn.Objective})
			if turn.FinalText != "" {
				history = append(history, agent.Message{Role: agent.RoleAgent, Content: turn.FinalText})
			}
		}
	}
	ctx = sharedusage.WithState(sharedusage.WithCategory(ctx, "planning"), stateDir)
	budget := &agent.Budget{Limits: agent.Limits{ModelRequests: 1, WallSeconds: 60}}
	return workrun.PrepareTaskPlan(ctx, agent.WithBudget(backend, budget), r, history)
}

// The native execution boundary independently reloads the claimed plan. It
// narrows connected reads to the sources in the actual reviewed content before
// constructing connector adapters. Workrun rechecks the reference and scope.
func bindDesktopTaskPlan(stateDir string, options workapp.RunOptions, request *workrun.Request) error {
	if !options.PlanFirst && options.TaskPlanRequestID == "" {
		return nil
	}
	if !options.PlanFirst || options.TaskPlanRequestID == "" {
		return worksession.ErrPlanChanged
	}
	store, err := worksession.Open(stateDir)
	if err != nil {
		return err
	}
	j, err := store.LoadRequests(request.ConversationID)
	if err != nil {
		return err
	}
	for _, r := range j.Requests {
		if r.ID != options.TaskPlanRequestID {
			continue
		}
		if r.Plan == nil || r.State != "running" || r.Plan.Phase != "dispatching" || r.Text != request.Objective {
			return worksession.ErrPlanChanged
		}
		if err := store.CheckPlanDispatch(request.ConversationID, r); err != nil {
			return err
		}
		request.TaskPlan = &workrun.TaskPlanReference{RequestID: r.ID, Revision: r.Plan.Revision, Digest: r.Plan.Digest}
		request.ConnectorIDs = append([]string{}, r.Plan.Content.Sources...)
		return nil
	}
	return worksession.ErrPlanChanged
}
