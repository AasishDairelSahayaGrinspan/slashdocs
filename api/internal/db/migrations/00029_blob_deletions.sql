-- +goose Up
-- Deleting a document cascades its versions, and with them the only record of
-- each html_blob_key. The keys are written here in the same transaction so the
-- objects can still be removed after the rows are gone. Rows are deleted once
-- the object is gone and are never expired: expiry would leak the object.
CREATE TABLE blob_deletions (
  blob_key TEXT PRIMARY KEY,
  attempts INTEGER NOT NULL DEFAULT 0,
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX blob_deletions_pending ON blob_deletions(available_at);

-- +goose Down
DROP TABLE blob_deletions;
