package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/gongahkia/norbot/internal/domain"
)

func (s *Store) UpsertSkillPackage(ctx context.Context, value domain.SkillPackage) (domain.SkillPackage, error) {
	if value.Digest == "" || value.ID == "" || value.Version == "" || value.Name == "" || value.Path == "" {
		return domain.SkillPackage{}, fmt.Errorf("invalid skill package")
	}
	manifest, err := json.Marshal(value.Manifest)
	if err != nil {
		return domain.SkillPackage{}, err
	}
	var raw []byte
	err = s.pool.QueryRow(ctx, `INSERT INTO skill_packages(digest,skill_id,version,name,description,manifest,path)
VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(digest) DO UPDATE SET manifest=EXCLUDED.manifest
RETURNING manifest,created_at`, value.Digest, value.ID, value.Version, value.Name, value.Description, manifest, value.Path).Scan(&raw, &value.CreatedAt)
	if err != nil {
		return domain.SkillPackage{}, err
	}
	if err := json.Unmarshal(raw, &value.Manifest); err != nil {
		return domain.SkillPackage{}, err
	}
	return value, nil
}

func (s *Store) CreateSkillImport(ctx context.Context, value domain.SkillImport) (domain.SkillImport, error) {
	if value.SourceType != "git" && value.SourceType != "oci" {
		return domain.SkillImport{}, fmt.Errorf("unsupported skill source type")
	}
	if value.SourceURI == "" || value.State == "" {
		return domain.SkillImport{}, fmt.Errorf("invalid skill import")
	}
	findings, err := json.Marshal(value.Findings)
	if err != nil {
		return domain.SkillImport{}, err
	}
	var raw []byte
	err = s.pool.QueryRow(ctx, `INSERT INTO skill_imports(source_type,source_uri,source_ref,credential_env,digest,state,findings)
VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,findings,activated_at,created_at`, value.SourceType, value.SourceURI, value.SourceRef, value.CredentialEnv, value.Digest, value.State, findings).Scan(&value.ID, &raw, &value.ActivatedAt, &value.CreatedAt)
	if err != nil {
		return domain.SkillImport{}, err
	}
	if err := json.Unmarshal(raw, &value.Findings); err != nil {
		return domain.SkillImport{}, err
	}
	return value, nil
}

func (s *Store) SkillImports(ctx context.Context) ([]domain.SkillImport, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,source_type,source_uri,source_ref,credential_env,digest,state,findings,activated_at,created_at FROM skill_imports ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []domain.SkillImport{}
	for rows.Next() {
		value, err := scanSkillImport(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) ActivateSkillImport(ctx context.Context, id int64) (domain.SkillImport, error) {
	row := s.pool.QueryRow(ctx, `UPDATE skill_imports SET state='active',activated_at=now() WHERE id=$1 AND state='scanned' RETURNING id,source_type,source_uri,source_ref,credential_env,digest,state,findings,activated_at,created_at`, id)
	return scanSkillImport(row)
}

func (s *Store) ActiveSkill(ctx context.Context, digest string) (domain.SkillPackage, error) {
	var value domain.SkillPackage
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT p.digest,p.skill_id,p.version,p.name,p.description,p.manifest,p.path,p.created_at FROM skill_packages p JOIN skill_imports i ON i.digest=p.digest WHERE p.digest=$1 AND i.state='active' ORDER BY i.id DESC LIMIT 1`, digest).Scan(&value.Digest, &value.ID, &value.Version, &value.Name, &value.Description, &raw, &value.Path, &value.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SkillPackage{}, ErrNotFound
	}
	if err != nil {
		return domain.SkillPackage{}, err
	}
	if err := json.Unmarshal(raw, &value.Manifest); err != nil {
		return domain.SkillPackage{}, err
	}
	return value, nil
}

func (s *Store) SelectRunSkills(ctx context.Context, runID string, digests []string) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		for _, digest := range digests {
			result, err := tx.Exec(ctx, `INSERT INTO run_skills(run_id,skill_digest) SELECT $1,$2 WHERE EXISTS(SELECT 1 FROM skill_imports WHERE digest=$2 AND state='active') ON CONFLICT DO NOTHING`, runID, digest)
			if err != nil {
				return err
			}
			if result.RowsAffected() != 1 {
				var exists bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM run_skills WHERE run_id=$1 AND skill_digest=$2)`, runID, digest).Scan(&exists); err != nil || !exists {
					return fmt.Errorf("skill %q is not active", digest)
				}
			}
		}
		return nil
	})
}

func (s *Store) RunSkills(ctx context.Context, runID string) ([]domain.SkillPackage, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.digest,p.skill_id,p.version,p.name,p.description,p.manifest,p.path,p.created_at FROM run_skills r JOIN skill_packages p ON p.digest=r.skill_digest WHERE r.run_id=$1 ORDER BY r.selected_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []domain.SkillPackage{}
	for rows.Next() {
		var value domain.SkillPackage
		var raw []byte
		if err := rows.Scan(&value.Digest, &value.ID, &value.Version, &value.Name, &value.Description, &raw, &value.Path, &value.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &value.Manifest); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func scanSkillImport(row interface{ Scan(...any) error }) (domain.SkillImport, error) {
	var value domain.SkillImport
	var raw []byte
	err := row.Scan(&value.ID, &value.SourceType, &value.SourceURI, &value.SourceRef, &value.CredentialEnv, &value.Digest, &value.State, &raw, &value.ActivatedAt, &value.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SkillImport{}, ErrNotFound
	}
	if err != nil {
		return domain.SkillImport{}, err
	}
	if err := json.Unmarshal(raw, &value.Findings); err != nil {
		return domain.SkillImport{}, err
	}
	return value, nil
}
