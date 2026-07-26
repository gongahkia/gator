package store

import (
	"context"
	"fmt"
)

type RetentionResult struct {
	AgentTurns, ChannelMessages, RunEvents, ProviderUsage int64
}

// PurgeRetainedData removes only explicitly configured aged records. Pending approvals retain their turns.
func (s *Store) PurgeRetainedData(ctx context.Context, agentTurnsDays, channelMessagesDays, runEventsDays, providerUsageDays int) (RetentionResult, error) {
	var result RetentionResult
	if agentTurnsDays > 0 {
		count, err := s.deleteRetained(ctx, `DELETE FROM agent_turns t WHERE t.created_at < now()-$1::interval AND NOT EXISTS (SELECT 1 FROM agent_actions a WHERE a.turn_id=t.id AND a.state='pending')`, agentTurnsDays)
		if err != nil {
			return result, err
		}
		result.AgentTurns = count
	}
	if channelMessagesDays > 0 {
		count, err := s.deleteRetained(ctx, `DELETE FROM channel_messages WHERE created_at < now()-$1::interval`, channelMessagesDays)
		if err != nil {
			return result, err
		}
		result.ChannelMessages = count
	}
	if runEventsDays > 0 {
		count, err := s.deleteRetained(ctx, `DELETE FROM run_events WHERE created_at < now()-$1::interval`, runEventsDays)
		if err != nil {
			return result, err
		}
		result.RunEvents = count
	}
	if providerUsageDays > 0 {
		count, err := s.deleteRetained(ctx, `DELETE FROM provider_usage WHERE created_at < now()-$1::interval`, providerUsageDays)
		if err != nil {
			return result, err
		}
		result.ProviderUsage = count
	}
	return result, nil
}

func (s *Store) deleteRetained(ctx context.Context, statement string, days int) (int64, error) {
	result, err := s.pool.Exec(ctx, statement, fmt.Sprintf("%d days", days))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}
