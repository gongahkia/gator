package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/gongahkia/norbot/internal/domain"
)

const agentTurnColumns = `id,run_id,session_id,external_id,role,idempotency_key,prompt,history,state,final,provider_id,created_at,updated_at`
const agentActionColumns = `id,turn_id,run_id,tool,role,params,digest,state,result,error,approved_by,decided_at,created_at,updated_at`

func (s *Store) CreateAgentTurn(ctx context.Context, value domain.AgentTurn) (domain.AgentTurn, bool, error) {
	history, err := json.Marshal(value.History); if err != nil { return domain.AgentTurn{}, false, err }
	row := s.pool.QueryRow(ctx, `INSERT INTO agent_turns(`+agentTurnColumns+`) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now(),now()) ON CONFLICT(run_id,idempotency_key) DO NOTHING RETURNING `+agentTurnColumns, value.ID,value.RunID,value.SessionID,value.ExternalID,value.Role,value.IdempotencyKey,value.Prompt,history,value.State,value.Final,value.ProviderID)
	created, err := scanAgentTurn(row)
	if errors.Is(err, pgx.ErrNoRows) { existing, getErr := s.AgentTurnByKey(ctx,value.RunID,value.IdempotencyKey); return existing,false,getErr }
	return created,err==nil,err
}
func (s *Store) AgentTurn(ctx,id string)(domain.AgentTurn,error){ return scanAgentTurn(s.pool.QueryRow(ctx,`SELECT `+agentTurnColumns+` FROM agent_turns WHERE id=$1`,id)) }
func (s *Store) AgentTurnByKey(ctx,runID,key string)(domain.AgentTurn,error){ return scanAgentTurn(s.pool.QueryRow(ctx,`SELECT `+agentTurnColumns+` FROM agent_turns WHERE run_id=$1 AND idempotency_key=$2`,runID,key)) }
func (s *Store) UpdateAgentTurn(ctx context.Context, value domain.AgentTurn) error { history,err:=json.Marshal(value.History);if err!=nil{return err}; result,err:=s.pool.Exec(ctx,`UPDATE agent_turns SET history=$2,state=$3,final=$4,updated_at=now() WHERE id=$1`,value.ID,history,value.State,value.Final);if err!=nil{return err};if result.RowsAffected()!=1{return ErrNotFound};return nil }
func (s *Store) AgentTurnForSession(ctx,sessionID string)(domain.AgentTurn,error){return scanAgentTurn(s.pool.QueryRow(ctx,`SELECT `+agentTurnColumns+` FROM agent_turns WHERE session_id=$1 ORDER BY created_at DESC LIMIT 1`,sessionID))}

func (s *Store) CreateAgentAction(ctx context.Context, value domain.AgentAction) (domain.AgentAction,error) { params,err:=json.Marshal(value.Params);if err!=nil{return domain.AgentAction{},err};result,err:=json.Marshal(value.Result);if err!=nil{return domain.AgentAction{},err};return scanAgentAction(s.pool.QueryRow(ctx,`INSERT INTO agent_actions(id,turn_id,run_id,tool,role,params,digest,state,result,error,approved_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now(),now()) RETURNING `+agentActionColumns,value.ID,value.TurnID,value.RunID,value.Tool,value.Role,params,value.Digest,value.State,result,value.Error,value.ApprovedBy)) }
func (s *Store) AgentAction(ctx,id string)(domain.AgentAction,error){return scanAgentAction(s.pool.QueryRow(ctx,`SELECT `+agentActionColumns+` FROM agent_actions WHERE id=$1`,id))}
func (s *Store) PendingAgentActions(ctx context.Context,limit int)([]domain.AgentAction,error){if limit<1||limit>200{limit=100};rows,err:=s.pool.Query(ctx,`SELECT `+agentActionColumns+` FROM agent_actions WHERE state='pending' ORDER BY created_at LIMIT $1`,limit);if err!=nil{return nil,err};defer rows.Close();values:=[]domain.AgentAction{};for rows.Next(){value,err:=scanAgentAction(rows);if err!=nil{return nil,err};values=append(values,value)};return values,rows.Err()}
func (s *Store) DecideAgentAction(ctx,id,state,operator string)(domain.AgentAction,error){if state!="approved"&&state!="rejected"{return domain.AgentAction{},fmt.Errorf("invalid action decision")};return scanAgentAction(s.pool.QueryRow(ctx,`UPDATE agent_actions SET state=$2,approved_by=$3,decided_at=now(),updated_at=now() WHERE id=$1 AND state='pending' RETURNING `+agentActionColumns,id,state,operator))}
func (s *Store) CompleteAgentAction(ctx,id,state string,result map[string]any,errorText string)(domain.AgentAction,error){encoded,err:=json.Marshal(result);if err!=nil{return domain.AgentAction{},err};return scanAgentAction(s.pool.QueryRow(ctx,`UPDATE agent_actions SET state=$2,result=$3,error=$4,updated_at=now() WHERE id=$1 AND state IN ('approved','running') RETURNING `+agentActionColumns,id,state,encoded,errorText))}
func (s *Store) CreateSandboxExecution(ctx,id,actionID,runID,target,tool string) error { _,err:=s.pool.Exec(ctx,`INSERT INTO sandbox_executions(id,action_id,run_id,target,tool,state) VALUES($1,$2,$3,$4,$5,'running')`,id,actionID,runID,target,tool);return err }
func (s *Store) CompleteSandboxExecution(ctx,id,state string,exitCode int,output string) error { _,err:=s.pool.Exec(ctx,`UPDATE sandbox_executions SET state=$2,exit_code=$3,output=$4,completed_at=now() WHERE id=$1 AND state='running'`,id,state,exitCode,output);return err }

func (s *Store) CreateManagedArtifact(ctx,value domain.ManagedArtifact)(domain.ManagedArtifact,error){return scanManagedArtifact(s.pool.QueryRow(ctx,`INSERT INTO managed_artifacts(id,run_id,owner_type,owner_id,object_key,filename,content_type,size_bytes,digest,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id,COALESCE(run_id,''),owner_type,owner_id,object_key,filename,content_type,size_bytes,digest,expires_at,created_at`,value.ID,nullIfEmpty(value.RunID),value.OwnerType,value.OwnerID,value.Key,value.Filename,value.ContentType,value.Size,value.Digest,value.ExpiresAt))}
func (s *Store) ManagedArtifact(ctx,id string)(domain.ManagedArtifact,error){return scanManagedArtifact(s.pool.QueryRow(ctx,`SELECT id,COALESCE(run_id,''),owner_type,owner_id,object_key,filename,content_type,size_bytes,digest,expires_at,created_at FROM managed_artifacts WHERE id=$1`,id))}
func (s *Store) ExpiredManagedArtifacts(ctx,now time.Time,limit int)([]domain.ManagedArtifact,error){if limit<1||limit>500{limit=100};rows,err:=s.pool.Query(ctx,`SELECT id,COALESCE(run_id,''),owner_type,owner_id,object_key,filename,content_type,size_bytes,digest,expires_at,created_at FROM managed_artifacts WHERE expires_at<=$1 ORDER BY expires_at LIMIT $2`,now,limit);if err!=nil{return nil,err};defer rows.Close();values:=[]domain.ManagedArtifact{};for rows.Next(){value,err:=scanManagedArtifact(rows);if err!=nil{return nil,err};values=append(values,value)};return values,rows.Err()}
func (s *Store) DeleteManagedArtifact(ctx,id string)error{_,err:=s.pool.Exec(ctx,`DELETE FROM managed_artifacts WHERE id=$1`,id);return err}

func scanAgentTurn(row interface{Scan(...any)error})(domain.AgentTurn,error){var value domain.AgentTurn;var history []byte;err:=row.Scan(&value.ID,&value.RunID,&value.SessionID,&value.ExternalID,&value.Role,&value.IdempotencyKey,&value.Prompt,&history,&value.State,&value.Final,&value.ProviderID,&value.CreatedAt,&value.UpdatedAt);if errors.Is(err,pgx.ErrNoRows){return domain.AgentTurn{},ErrNotFound};if err!=nil{return domain.AgentTurn{},err};if err:=json.Unmarshal(history,&value.History);err!=nil{return domain.AgentTurn{},err};return value,nil}
func scanAgentAction(row interface{Scan(...any)error})(domain.AgentAction,error){var value domain.AgentAction;var params,result []byte;err:=row.Scan(&value.ID,&value.TurnID,&value.RunID,&value.Tool,&value.Role,&params,&value.Digest,&value.State,&result,&value.Error,&value.ApprovedBy,&value.DecidedAt,&value.CreatedAt,&value.UpdatedAt);if errors.Is(err,pgx.ErrNoRows){return domain.AgentAction{},ErrNotFound};if err!=nil{return domain.AgentAction{},err};if err:=json.Unmarshal(params,&value.Params);err!=nil{return domain.AgentAction{},err};if err:=json.Unmarshal(result,&value.Result);err!=nil{return domain.AgentAction{},err};return value,nil}
func scanManagedArtifact(row interface{Scan(...any)error})(domain.ManagedArtifact,error){var value domain.ManagedArtifact;err:=row.Scan(&value.ID,&value.RunID,&value.OwnerType,&value.OwnerID,&value.Key,&value.Filename,&value.ContentType,&value.Size,&value.Digest,&value.ExpiresAt,&value.CreatedAt);if errors.Is(err,pgx.ErrNoRows){return domain.ManagedArtifact{},ErrNotFound};return value,err}
func nullIfEmpty(value string)any{if value==""{return nil};return value}
