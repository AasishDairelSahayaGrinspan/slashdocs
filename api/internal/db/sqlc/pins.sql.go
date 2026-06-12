// Hand-written query layer for pins (mirrors sqlc generated style).
// source: pins.sql

package dbsqlc

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const upsertPin = `INSERT INTO pins(principal_type, principal_id, resource_type, resource_id)
VALUES($1, $2, $3, $4)
ON CONFLICT DO NOTHING`

type UpsertPinParams struct {
	PrincipalType string
	PrincipalID   string
	ResourceType  string
	ResourceID    string
}

func (q *Queries) UpsertPin(ctx context.Context, arg UpsertPinParams) error {
	_, err := q.db.Exec(ctx, upsertPin, arg.PrincipalType, arg.PrincipalID, arg.ResourceType, arg.ResourceID)
	return err
}

const deletePin = `DELETE FROM pins
WHERE principal_type=$1 AND principal_id=$2 AND resource_type=$3 AND resource_id=$4`

type DeletePinParams struct {
	PrincipalType string
	PrincipalID   string
	ResourceType  string
	ResourceID    string
}

func (q *Queries) DeletePin(ctx context.Context, arg DeletePinParams) error {
	_, err := q.db.Exec(ctx, deletePin, arg.PrincipalType, arg.PrincipalID, arg.ResourceType, arg.ResourceID)
	return err
}

const listPins = `SELECT resource_type, resource_id, pinned_at
FROM pins
WHERE principal_type=$1 AND principal_id=$2
ORDER BY pinned_at DESC`

type ListPinsParams struct {
	PrincipalType string
	PrincipalID   string
}

type ListPinsRow struct {
	ResourceType string
	ResourceID   string
	PinnedAt     pgtype.Timestamptz
}

func (q *Queries) ListPins(ctx context.Context, arg ListPinsParams) ([]ListPinsRow, error) {
	rows, err := q.db.Query(ctx, listPins, arg.PrincipalType, arg.PrincipalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ListPinsRow
	for rows.Next() {
		var i ListPinsRow
		if err := rows.Scan(&i.ResourceType, &i.ResourceID, &i.PinnedAt); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}
