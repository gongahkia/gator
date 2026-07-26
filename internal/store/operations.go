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
  (SELECT count(*) FROM runs WHERE workspace_status='failed')`)
	var queueDepth, outboxQueued, outboxRunning, outboxDead, pendingApprovals, workspaceFailures int64
	if err := row.Scan(&queueDepth, &outboxQueued, &outboxRunning, &outboxDead, &pendingApprovals, &workspaceFailures); err != nil {
		return nil, err
	}
	values["queue_depth"] = queueDepth
	values["outbox_queued"] = outboxQueued
	values["outbox_running"] = outboxRunning
	values["outbox_dead"] = outboxDead
	values["pending_approvals"] = pendingApprovals
	values["workspace_failures"] = workspaceFailures
	return values, nil
}
