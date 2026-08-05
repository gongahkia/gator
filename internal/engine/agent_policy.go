package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/store"
)

var defaultAgentPathPrefixes = []string{"agent-input/", "agent-output/", "generated-app/", "stage-output/"}

func (s *Service) defaultRunAgentPolicy(run domain.Run) domain.RunAgentPolicy {
	policy := domain.RunAgentPolicy{RunID: run.ID, Version: 1, Stages: map[domain.Stage]domain.InternalAgentPolicy{}, Tools: map[string]domain.AgentToolPolicy{}}
	for _, stage := range domain.Stages {
		stagePolicy := domain.InternalAgentPolicy{Enabled: true}
		if stage == domain.StageDeployer {
			stagePolicy.AllowCLI, stagePolicy.AllowNetwork = true, true
		} else {
			stagePolicy.AllowModel = true
			if provider, ok := s.config.Manifest.Provider(run.Providers[stage], stage); ok {
				switch provider.Kind {
				case "cli":
					stagePolicy.AllowCLI = true
					network := strings.TrimSpace(provider.Network)
					if network == "" {
						network = "none"
					}
					stagePolicy.AllowNetwork = network != "none"
				case "plugin":
					stagePolicy.AllowCLI, stagePolicy.AllowNetwork = true, true
				default:
					stagePolicy.AllowNetwork = true
				}
			}
		}
		policy.Stages[stage] = stagePolicy
	}
	for name, configured := range s.config.Manifest.ToolPolicy {
		paths := append([]string(nil), configured.AllowedPathPrefixes...)
		if len(paths) == 0 && (name == "artifact_read" || name == "file_write") {
			paths = append([]string(nil), defaultAgentPathPrefixes...)
		}
		maxCalls := configured.MaxCalls
		if maxCalls == 0 {
			maxCalls = agentMaxToolCalls
		}
		policy.Tools[name] = domain.AgentToolPolicy{Enabled: configured.Enabled, ApprovalRequired: configured.ApprovalRequired, Roles: append([]string(nil), configured.Roles...), AllowedHosts: append([]string(nil), configured.AllowedHosts...), AllowedCommands: append([]string(nil), configured.AllowedCommands...), AllowedPathPrefixes: paths, MaxCalls: maxCalls}
	}
	return policy
}

func (s *Service) AgentPolicy(ctx context.Context, runID string) (domain.RunAgentPolicy, error) {
	policy, err := s.store.RunAgentPolicy(ctx, runID)
	if err == nil {
		return policy, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return domain.RunAgentPolicy{}, err
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return domain.RunAgentPolicy{}, err
	}
	return s.store.EnsureRunAgentPolicy(ctx, s.defaultRunAgentPolicy(run))
}

func (s *Service) RestrictAgentPolicy(ctx context.Context, runID string, requested domain.RunAgentPolicy, operator string) (domain.RunAgentPolicy, error) {
	current, err := s.AgentPolicy(ctx, runID)
	if err != nil {
		return domain.RunAgentPolicy{}, err
	}
	ceiling := current
	return s.store.RestrictRunAgentPolicy(ctx, runID, requested, ceiling, operator)
}

func (s *Service) enforceInternalAgentPolicy(ctx context.Context, run domain.Run, stage domain.Stage, provider config.Provider) error {
	policy, err := s.AgentPolicy(ctx, run.ID)
	if err != nil {
		return err
	}
	stagePolicy, ok := policy.Stages[stage]
	if !ok || !stagePolicy.Enabled {
		return fmt.Errorf("operator disabled %s agent for this run", stage)
	}
	if stage != domain.StageDeployer && !stagePolicy.AllowModel {
		return fmt.Errorf("operator disabled model access for %s", stage)
	}
	if stage == domain.StageDeployer {
		if !stagePolicy.AllowCLI || !stagePolicy.AllowNetwork {
			return fmt.Errorf("operator disabled deployment runtime capability")
		}
		return nil
	}
	switch provider.Kind {
	case "cli", "plugin":
		if !stagePolicy.AllowCLI {
			return fmt.Errorf("operator disabled local runtime access for %s", stage)
		}
		if provider.Kind == "plugin" || strings.TrimSpace(provider.Network) != "none" {
			if !stagePolicy.AllowNetwork {
				return fmt.Errorf("operator disabled network access for %s", stage)
			}
		}
	default:
		if !stagePolicy.AllowNetwork {
			return fmt.Errorf("operator disabled model network access for %s", stage)
		}
	}
	return nil
}
