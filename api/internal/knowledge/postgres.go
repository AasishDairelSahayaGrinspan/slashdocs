package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	recordSettings     = "settings"
	recordSource       = "source"
	recordRun          = "sync_run"
	recordAudit        = "audit"
	recordNotification = "notification"
	recordTeam         = "team"
	portalSlugIndex    = "connected_knowledge_enabled_portal_slug_uidx"
	portalSlugSQL      = `LEFT(TRIM(BOTH '-' FROM REGEXP_REPLACE(LOWER(BTRIM(COALESCE(payload #>> '{portal,slug}', ''))), '[^a-z0-9]+', '-', 'g')), 80)`
)

type Postgres struct{ db *pgxpool.Pool }

func NewPostgres(db *pgxpool.Pool) *Postgres { return &Postgres{db: db} }

func marshal(value any) ([]byte, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal knowledge record: %w", err)
	}
	return b, nil
}

func (s *Postgres) put(ctx context.Context, kind, id, repositoryID, userID string, value any) error {
	b, err := marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO connected_knowledge_records(kind,id,repository_id,user_id,payload)
		VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),$5::jsonb)
		ON CONFLICT(kind,id) DO UPDATE SET repository_id=EXCLUDED.repository_id,user_id=EXCLUDED.user_id,payload=EXCLUDED.payload,updated_at=now()`, kind, id, repositoryID, userID, b)
	return err
}

func getRecord[T any](ctx context.Context, db *pgxpool.Pool, kind, id string) (T, error) {
	var out T
	var raw []byte
	err := db.QueryRow(ctx, `SELECT payload FROM connected_knowledge_records WHERE kind=$1 AND id=$2`, kind, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("decode knowledge record: %w", err)
	}
	return out, nil
}

func listRecords[T any](ctx context.Context, db *pgxpool.Pool, kind, repositoryID, userID string) ([]T, error) {
	rows, err := db.Query(ctx, `SELECT payload FROM connected_knowledge_records
		WHERE kind=$1 AND ($2='' OR repository_id=$2) AND ($3='' OR user_id=$3) ORDER BY created_at DESC`, kind, repositoryID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, fmt.Errorf("decode knowledge record: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Postgres) GetSettings(ctx context.Context, repositoryID string) (Settings, error) {
	v, err := getRecord[Settings](ctx, s.db, recordSettings, repositoryID)
	if errors.Is(err, ErrNotFound) {
		return DefaultSettings(repositoryID), nil
	}
	return v, err
}
func (s *Postgres) PutSettings(ctx context.Context, v Settings) (Settings, error) {
	v.Portal.Slug = NormalizePortalSlug(v.Portal.Slug)
	if v.Portal.Enabled && v.Portal.Slug == "" {
		return Settings{}, ErrInvalid
	}
	v.UpdatedAt = Now()
	err := s.put(ctx, recordSettings, v.RepositoryID, v.RepositoryID, "", v)
	var postgresErr *pgconn.PgError
	if errors.As(err, &postgresErr) && postgresErr.Code == "23505" && postgresErr.ConstraintName == portalSlugIndex {
		return Settings{}, ErrConflict
	}
	return v, err
}
func (s *Postgres) FindPortal(ctx context.Context, slug string) (Settings, error) {
	var out Settings
	var raw []byte
	slug = NormalizePortalSlug(slug)
	if slug == "" {
		return out, ErrNotFound
	}
	err := s.db.QueryRow(ctx, `SELECT payload FROM connected_knowledge_records WHERE kind=$1 AND `+portalSlugSQL+`=$2 AND COALESCE(payload #>> '{portal,enabled}', 'false')='true' ORDER BY updated_at DESC, id LIMIT 1`, recordSettings, slug).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, err
	}
	return out, nil
}
func (s *Postgres) ListSources(ctx context.Context, repositoryID string) ([]Source, error) {
	return listRecords[Source](ctx, s.db, recordSource, repositoryID, "")
}
func (s *Postgres) ListAllSources(ctx context.Context) ([]Source, error) {
	return listRecords[Source](ctx, s.db, recordSource, "", "")
}
func (s *Postgres) GetSource(ctx context.Context, repositoryID, id string) (Source, error) {
	v, err := getRecord[Source](ctx, s.db, recordSource, id)
	if err == nil && v.RepositoryID != repositoryID {
		return Source{}, ErrNotFound
	}
	return v, err
}
func (s *Postgres) PutSource(ctx context.Context, v Source) (Source, error) {
	return v, s.put(ctx, recordSource, v.ID, v.RepositoryID, "", v)
}
func (s *Postgres) DeleteSource(ctx context.Context, repositoryID, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM connected_knowledge_records WHERE kind=$1 AND id=$2 AND repository_id=$3`, recordSource, id, repositoryID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
func (s *Postgres) AcceptDelivery(ctx context.Context, provider, id string) (bool, error) {
	tag, err := s.db.Exec(ctx, `INSERT INTO connected_knowledge_webhook_deliveries(provider,delivery_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, provider, id)
	return err == nil && tag.RowsAffected() == 1, err
}
func (s *Postgres) QueueDelivery(ctx context.Context, provider, id string, runs []SyncRun) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `INSERT INTO connected_knowledge_webhook_deliveries(provider,delivery_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, provider, id)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	for _, run := range runs {
		raw, marshalErr := marshal(run)
		if marshalErr != nil {
			return false, marshalErr
		}
		if _, err := tx.Exec(ctx, `INSERT INTO connected_knowledge_records(kind,id,repository_id,user_id,payload)
			VALUES($1,$2,NULLIF($3,''),NULL,$4::jsonb)`, recordRun, run.ID, run.RepositoryID, raw); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
func (s *Postgres) AddSyncRun(ctx context.Context, v SyncRun) (SyncRun, error) {
	return v, s.put(ctx, recordRun, v.ID, v.RepositoryID, "", v)
}
func (s *Postgres) ListSyncRuns(ctx context.Context, repositoryID string) ([]SyncRun, error) {
	return listRecords[SyncRun](ctx, s.db, recordRun, repositoryID, "")
}
func (s *Postgres) ClaimQueuedSyncRuns(ctx context.Context, limit int) ([]SyncRun, error) {
	if limit < 1 {
		limit = 1
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT id,payload FROM connected_knowledge_records WHERE kind=$1 AND (payload->>'status'='queued' OR (payload->>'status'='processing' AND updated_at < now()-interval '10 minutes')) ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT $2`, recordRun, limit)
	if err != nil {
		return nil, err
	}
	type selected struct {
		id  string
		run SyncRun
	}
	picked := []selected{}
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		var run SyncRun
		if err := json.Unmarshal(raw, &run); err != nil {
			rows.Close()
			return nil, err
		}
		run.Status = "processing"
		picked = append(picked, selected{id, run})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]SyncRun, 0, len(picked))
	for _, item := range picked {
		raw, _ := json.Marshal(item.run)
		if _, err := tx.Exec(ctx, `UPDATE connected_knowledge_records SET payload=$3::jsonb,updated_at=now() WHERE kind=$1 AND id=$2`, recordRun, item.id, raw); err != nil {
			return nil, err
		}
		out = append(out, item.run)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Postgres) RecoverSyncRuns(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `UPDATE connected_knowledge_records SET payload=jsonb_set(payload,'{status}','"queued"'::jsonb),updated_at=now() WHERE kind=$1 AND payload->>'status'='processing' AND updated_at < now()-interval '10 minutes'`, recordRun)
	return err
}
func (s *Postgres) AddAudit(ctx context.Context, v AuditEvent) error {
	return s.put(ctx, recordAudit, v.ID, v.RepositoryID, "", v)
}
func (s *Postgres) ListAudit(ctx context.Context, repositoryID string) ([]AuditEvent, error) {
	return listRecords[AuditEvent](ctx, s.db, recordAudit, repositoryID, "")
}
func (s *Postgres) AddNotification(ctx context.Context, v Notification) error {
	return s.put(ctx, recordNotification, v.ID, v.RepositoryID, v.UserID, v)
}
func (s *Postgres) ListNotifications(ctx context.Context, userID string) ([]Notification, error) {
	return listRecords[Notification](ctx, s.db, recordNotification, "", userID)
}
func (s *Postgres) SetNotificationRead(ctx context.Context, userID, id string, read bool) (Notification, error) {
	v, err := getRecord[Notification](ctx, s.db, recordNotification, id)
	if err != nil || v.UserID != userID {
		if err == nil {
			err = ErrNotFound
		}
		return Notification{}, err
	}
	v.Read = read
	return v, s.put(ctx, recordNotification, v.ID, v.RepositoryID, v.UserID, v)
}
func (s *Postgres) ListTeams(ctx context.Context, repositoryID string) ([]Team, error) {
	return listRecords[Team](ctx, s.db, recordTeam, repositoryID, "")
}
func (s *Postgres) GetTeam(ctx context.Context, repositoryID, id string) (Team, error) {
	v, err := getRecord[Team](ctx, s.db, recordTeam, id)
	if err == nil && v.RepositoryID != repositoryID {
		return Team{}, ErrNotFound
	}
	return v, err
}
func (s *Postgres) PutTeam(ctx context.Context, v Team) (Team, error) {
	return v, s.put(ctx, recordTeam, v.ID, v.RepositoryID, "", v)
}
func (s *Postgres) DeleteTeam(ctx context.Context, repositoryID, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM connected_knowledge_records WHERE kind=$1 AND id=$2 AND repository_id=$3`, recordTeam, id, repositoryID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
