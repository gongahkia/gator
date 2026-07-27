package store

import "context"

// OperationalGauges reads durable state so alerts survive process restarts.
func (s *Store) OperationalGauges(ctx context.Context) (map[string]int64, error) {
	values := map[string]int64{}
	row := s.pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM jobs WHERE state IN ('queued','running','blocked')),
  (SELECT count(*) FROM outbox_events WHERE state='queued'),
  (SELECT count(*) FROM outbox_events WHERE state='running'),
	  (SELECT count(*) FROM outbox_events WHERE state='dead'),
	  (SELECT count(*) FROM agent_actions WHERE state='pending' AND (expires_at IS NULL OR expires_at>now())),
	  (SELECT count(*) FROM runs WHERE workspace_status='failed'),
	  (SELECT count(*) FROM deployments WHERE is_current AND status='failed'),
	  (SELECT count(*) FROM provider_observations WHERE observed_at>=now()-interval '1 hour' AND metadata->>'outcome'='error'),
	  (SELECT count(*) FROM provider_observations WHERE observed_at>=now()-interval '1 hour' AND metadata->>'rate_limited'='true'),
	  (SELECT COALESCE(sum(input_tokens+output_tokens),0) FROM provider_usage WHERE created_at>=now()-interval '24 hours')`)
	var queueDepth, outboxQueued, outboxRunning, outboxDead, pendingApprovals, workspaceFailures, deploymentsFailed, providerErrors, providerRateLimited, providerTokens int64
	if err := row.Scan(&queueDepth, &outboxQueued, &outboxRunning, &outboxDead, &pendingApprovals, &workspaceFailures, &deploymentsFailed, &providerErrors, &providerRateLimited, &providerTokens); err != nil {
		return nil, err
	}
	values["queue_depth"] = queueDepth
	values["outbox_queued"] = outboxQueued
	values["outbox_running"] = outboxRunning
	values["outbox_dead"] = outboxDead
	values["pending_approvals"] = pendingApprovals
	values["workspace_failures"] = workspaceFailures
	values["deployments_failed"] = deploymentsFailed
	values["provider_errors_1h"] = providerErrors
	values["provider_rate_limited_1h"] = providerRateLimited
	values["provider_tokens_24h"] = providerTokens
	return values, nil
}
