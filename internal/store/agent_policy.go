package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gongahkia/norbot/internal/domain"
	"github.com/jackc/pgx/v5"
)

const runAgentPolicyColumns = `run_id,version,policy,digest,updated_by,created_at,updated_at`

func (s *Store) RunAgentPolicy(ctx context.Context, runID string) (domain.RunAgentPolicy, error) {
	return scanRunAgentPolicy(s.pool.QueryRow(ctx, `SELECT `+runAgentPolicyColumns+` FROM run_agent_policies WHERE run_id=$1`, runID))
}

func (s *Store) EnsureRunAgentPolicy(ctx context.Context, policy domain.RunAgentPolicy) (domain.RunAgentPolicy, error) {
	if policy.RunID == "" {
		return domain.RunAgentPolicy{}, fmt.Errorf("run_id is required")
	}
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runs WHERE id=$1)`, policy.RunID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		policy.Version = 1
		policy.UpdatedBy = "legacy-default"
		policy.Digest = runAgentPolicyDigest(policy)
		encoded, err := json.Marshal(policyBody(policy))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO run_agent_policies(run_id,version,policy,digest,updated_by) VALUES($1,$2,$3,$4,$5) ON CONFLICT(run_id) DO NOTHING`, policy.RunID, policy.Version, encoded, policy.Digest, policy.UpdatedBy)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO run_agent_policy_events(run_id,version,policy,digest,actor) VALUES($1,$2,$3,$4,$5) ON CONFLICT(run_id,version) DO NOTHING`, policy.RunID, policy.Version, encoded, policy.Digest, policy.UpdatedBy); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return domain.RunAgentPolicy{}, err
	}
	return s.RunAgentPolicy(ctx, policy.RunID)
}

// RestrictRunAgentPolicy accepts only a subset of the immutable manifest ceiling
// and the current run policy. A run can therefore never regain a revoked tool.
func (s *Store) RestrictRunAgentPolicy(ctx context.Context, runID string, requested, ceiling domain.RunAgentPolicy, actor string) (domain.RunAgentPolicy, error) {
	var result domain.RunAgentPolicy
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		current, err := scanRunAgentPolicy(tx.QueryRow(ctx, `SELECT `+runAgentPolicyColumns+` FROM run_agent_policies WHERE run_id=$1 FOR UPDATE`, runID))
		if err != nil {
			return err
		}
		if err := policyIsRestriction(requested, current, ceiling); err != nil {
			return err
		}
		requested.RunID = runID
		requested.Version = current.Version + 1
		requested.UpdatedBy = actor
		requested.Digest = runAgentPolicyDigest(requested)
		encoded, err := json.Marshal(policyBody(requested))
		if err != nil {
			return err
		}
		result, err = scanRunAgentPolicy(tx.QueryRow(ctx, `UPDATE run_agent_policies SET version=$2,policy=$3,digest=$4,updated_by=$5,updated_at=now() WHERE run_id=$1 RETURNING `+runAgentPolicyColumns, runID, requested.Version, encoded, requested.Digest, actor))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO run_agent_policy_events(run_id,version,policy,digest,actor) VALUES($1,$2,$3,$4,$5)`, runID, result.Version, encoded, result.Digest, actor); err != nil {
			return err
		}
		if err := s.insertEvent(ctx, tx, runID, "agent_policy_restricted", "Agent capabilities restricted by operator", map[string]any{"version": result.Version, "digest": result.Digest, "actor": actor}); err != nil {
			return err
		}
		return s.insertAuditEvent(ctx, tx, actor, "run_agent_policy.restricted", "run", runID, map[string]any{"version": result.Version, "digest": result.Digest})
	})
	return result, err
}

func insertRunAgentPolicy(ctx context.Context, tx pgx.Tx, policy domain.RunAgentPolicy, actor string) error {
	if policy.Version == 0 {
		policy.Version = 1
	}
	policy.Digest = runAgentPolicyDigest(policy)
	encoded, err := json.Marshal(policyBody(policy))
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO run_agent_policies(run_id,version,policy,digest,updated_by) VALUES($1,$2,$3,$4,$5)`, policy.RunID, policy.Version, encoded, policy.Digest, actor); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO run_agent_policy_events(run_id,version,policy,digest,actor) VALUES($1,$2,$3,$4,$5)`, policy.RunID, policy.Version, encoded, policy.Digest, actor); err != nil {
		return err
	}
	return nil
}

func scanRunAgentPolicy(row interface{ Scan(...any) error }) (domain.RunAgentPolicy, error) {
	var value domain.RunAgentPolicy
	var encoded []byte
	err := row.Scan(&value.RunID, &value.Version, &encoded, &value.Digest, &value.UpdatedBy, &value.CreatedAt, &value.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.RunAgentPolicy{}, ErrNotFound
		}
		return domain.RunAgentPolicy{}, err
	}
	var body struct {
		Stages map[domain.Stage]domain.InternalAgentPolicy `json:"stages"`
		Tools  map[string]domain.AgentToolPolicy           `json:"tools"`
	}
	if err := json.Unmarshal(encoded, &body); err != nil {
		return domain.RunAgentPolicy{}, err
	}
	value.Stages, value.Tools = body.Stages, body.Tools
	return value, nil
}

func policyBody(policy domain.RunAgentPolicy) any {
	return struct {
		Stages map[domain.Stage]domain.InternalAgentPolicy `json:"stages"`
		Tools  map[string]domain.AgentToolPolicy           `json:"tools"`
	}{policy.Stages, policy.Tools}
}

func runAgentPolicyDigest(policy domain.RunAgentPolicy) string {
	encoded, _ := json.Marshal(policyBody(policy))
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func policyIsRestriction(candidate, current, ceiling domain.RunAgentPolicy) error {
	if err := stagePoliciesRestricted(candidate.Stages, current.Stages, ceiling.Stages); err != nil {
		return err
	}
	for name, currentTool := range current.Tools {
		candidateTool, ok := candidate.Tools[name]
		if !ok {
			return fmt.Errorf("tool %q cannot be removed from policy shape", name)
		}
		ceilingTool, ok := ceiling.Tools[name]
		if !ok || !toolPolicyRestricted(candidateTool, currentTool, ceilingTool) {
			return fmt.Errorf("tool %q policy would expand permissions", name)
		}
	}
	for name := range candidate.Tools {
		if _, ok := current.Tools[name]; !ok {
			return fmt.Errorf("tool %q is not in this run policy", name)
		}
	}
	return nil
}

func stagePoliciesRestricted(candidate, current, ceiling map[domain.Stage]domain.InternalAgentPolicy) error {
	for _, stage := range domain.Stages {
		c, ok := candidate[stage]
		if !ok {
			return fmt.Errorf("stage %q is required", stage)
		}
		now, ok := current[stage]
		if !ok {
			return fmt.Errorf("stage %q is unavailable", stage)
		}
		max, ok := ceiling[stage]
		if !ok || (!boolRestricted(c.Enabled, now.Enabled, max.Enabled) || !boolRestricted(c.AllowModel, now.AllowModel, max.AllowModel) || !boolRestricted(c.AllowNetwork, now.AllowNetwork, max.AllowNetwork) || !boolRestricted(c.AllowCLI, now.AllowCLI, max.AllowCLI)) {
			return fmt.Errorf("stage %q policy would expand permissions", stage)
		}
	}
	for stage := range candidate {
		if !stage.Valid() {
			return fmt.Errorf("unknown stage %q", stage)
		}
	}
	return nil
}

func toolPolicyRestricted(c, now, max domain.AgentToolPolicy) bool {
	return boolRestricted(c.Enabled, now.Enabled, max.Enabled) && boolRestricted(!c.ApprovalRequired, !now.ApprovalRequired, !max.ApprovalRequired) && subset(c.Roles, now.Roles) && subset(c.Roles, max.Roles) && subset(c.AllowedHosts, now.AllowedHosts) && subset(c.AllowedHosts, max.AllowedHosts) && subset(c.AllowedCommands, now.AllowedCommands) && subset(c.AllowedCommands, max.AllowedCommands) && subset(c.AllowedPathPrefixes, now.AllowedPathPrefixes) && subset(c.AllowedPathPrefixes, max.AllowedPathPrefixes) && c.MaxCalls > 0 && c.MaxCalls <= now.MaxCalls && c.MaxCalls <= max.MaxCalls
}

func boolRestricted(candidate, current, ceiling bool) bool { return !candidate || current && ceiling }

func subset(candidate, superset []string) bool {
	values := map[string]struct{}{}
	for _, value := range superset {
		values[strings.TrimSpace(value)] = struct{}{}
	}
	for _, value := range candidate {
		if _, ok := values[strings.TrimSpace(value)]; !ok {
			return false
		}
	}
	return true
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
