package main

import (
	"context"
	"errors"

	"github.com/gongahkia/gator/internal/lifecycle"
	sharedusage "github.com/gongahkia/gator/internal/usage"
	"github.com/gongahkia/gator/internal/workrun"
)

func (c gatorAppController) prepareUsage(change workrun.AppChange) (workrun.AppMutation, error) {
	store, err := sharedusage.Open(c.stateDir)
	if err != nil {
		return workrun.AppMutation{}, err
	}
	prior, err := store.Policy()
	if err != nil {
		return workrun.AppMutation{}, err
	}
	next := prior
	if change.Setting == "usage_pause" {
		var v struct {
			Paused *bool `json:"paused"`
		}
		if err := workrun.DecodeAppArguments(change.Value, &v); err != nil {
			return workrun.AppMutation{}, err
		}
		if v.Paused == nil {
			return workrun.AppMutation{}, errors.New("provide paused true or false")
		}
		next.Paused = *v.Paused
	} else {
		var v struct {
			Requests   *int   `json:"daily_requests"`
			Tokens     *int64 `json:"daily_tokens"`
			Concurrent *int   `json:"concurrent_requests"`
		}
		if err := workrun.DecodeAppArguments(change.Value, &v); err != nil {
			return workrun.AppMutation{}, err
		}
		if v.Requests == nil && v.Tokens == nil && v.Concurrent == nil {
			return workrun.AppMutation{}, errors.New("provide at least one usage limit; zero means uncapped")
		}
		if v.Requests != nil {
			next.DailyRequests = *v.Requests
		}
		if v.Tokens != nil {
			next.DailyTokens = *v.Tokens
		}
		if v.Concurrent != nil {
			next.ConcurrentRequests = *v.Concurrent
		}
	}
	if err := next.Validate(); err != nil {
		return workrun.AppMutation{}, err
	}
	return workrun.AppMutation{Review: true, AfterReply: true, Preview: map[string]any{"setting": change.Setting, "before": prior, "after": next, "effect": "Applies after this reply. Pausing or exhausting a limit blocks later chat requests; use Usage & limits to resume."}, Apply: func(context.Context) (any, error) { return next, store.SaveReviewedPolicy(next, prior) }}, nil
}

func (c gatorAppController) prepareRetention(change workrun.AppChange) (workrun.AppMutation, error) {
	store, err := lifecycle.Open(c.stateDir)
	if err != nil {
		return workrun.AppMutation{}, err
	}
	prior, err := store.Policy()
	if err != nil {
		return workrun.AppMutation{}, err
	}
	var v struct {
		History   *int `json:"history_days"`
		Knowledge *int `json:"knowledge_days"`
		Snapshots *int `json:"snapshot_days"`
	}
	if err := workrun.DecodeAppArguments(change.Value, &v); err != nil {
		return workrun.AppMutation{}, err
	}
	if v.History == nil && v.Knowledge == nil && v.Snapshots == nil {
		return workrun.AppMutation{}, errors.New("provide retention days; zero means keep until deleted")
	}
	next := prior
	if v.History != nil {
		next.HistoryDays = *v.History
	}
	if v.Knowledge != nil {
		next.KnowledgeDays = *v.Knowledge
	}
	if v.Snapshots != nil {
		next.SnapshotDays = *v.Snapshots
	}
	if err := next.Validate(); err != nil {
		return workrun.AppMutation{}, err
	}
	return workrun.AppMutation{Review: true, AfterRun: true, Preview: map[string]any{"setting": "retention", "before": prior, "after": next, "effect": "Eligible private content expires automatically at startup and during background maintenance. Zero keeps content. No cleanup runs as part of this settings change."}, Apply: func(context.Context) (any, error) { return next, store.SaveReviewedPolicy(next, prior) }}, nil
}
