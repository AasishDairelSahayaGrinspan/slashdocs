package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	dbsqlc "github.com/razorpay/aidocs/api/internal/db/sqlc"
	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
)

func (s *Store) CreateList(ctx context.Context, owner auth.Principal, name string) (repo.List, error) {
	id := newID("lst")
	row, err := s.q.CreateList(ctx, dbsqlc.CreateListParams{
		ID:      id,
		Name:    name,
		OwnerID: owner.ID,
	})
	if err != nil {
		return repo.List{}, err
	}
	return repo.List{ID: row.ID, Name: row.Name, Owner: owner}, nil
}

func (s *Store) GetList(ctx context.Context, id string) (repo.List, error) {
	row, err := s.q.GetList(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.List{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.List{}, err
	}
	u, _ := s.ResolveUser(ctx, row.OwnerID)
	return repo.List{ID: row.ID, Name: row.Name, Owner: u}, nil
}

func (s *Store) ListLists(ctx context.Context, owner auth.Principal) ([]repo.List, error) {
	rows, err := s.q.ListListsByOwner(ctx, owner.ID)
	if err != nil {
		return nil, err
	}
	out := make([]repo.List, 0, len(rows))
	for _, r := range rows {
		out = append(out, repo.List{ID: r.ID, Name: r.Name, Owner: owner})
	}
	return out, nil
}

func (s *Store) UpdateList(ctx context.Context, id, name string) (repo.List, error) {
	row, err := s.q.UpdateList(ctx, dbsqlc.UpdateListParams{ID: id, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.List{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.List{}, err
	}
	u, _ := s.ResolveUser(ctx, row.OwnerID)
	return repo.List{ID: row.ID, Name: row.Name, Owner: u}, nil
}

func (s *Store) DeleteList(ctx context.Context, id string) error {
	return s.q.DeleteList(ctx, id)
}

func (s *Store) AddDocumentToList(ctx context.Context, listID, documentID string) error {
	return s.q.AddDocumentToList(ctx, dbsqlc.AddDocumentToListParams{
		ListID:     listID,
		DocumentID: documentID,
	})
}

func (s *Store) RemoveDocumentFromList(ctx context.Context, listID, documentID string) error {
	return s.q.RemoveDocumentFromList(ctx, dbsqlc.RemoveDocumentFromListParams{
		ListID:     listID,
		DocumentID: documentID,
	})
}

func (s *Store) ListDocumentsByList(ctx context.Context, listID string) ([]repo.Document, error) {
	rows, err := s.q.ListDocumentsByList(ctx, listID)
	if err != nil {
		return nil, err
	}
	out := make([]repo.Document, 0, len(rows))
	for _, r := range rows {
		owner := auth.Principal{
			Type:  auth.PrincipalType(r.OwnerType),
			ID:    r.OwnerID,
			Email: r.OwnerEmail,
			Name:  r.OwnerName,
		}
		out = append(out, repo.Document{
			ID:               r.ID,
			Title:            r.Title,
			Owner:            owner,
			CurrentVersionID: r.CurrentVersionID.String,
		})
	}
	return out, nil
}
