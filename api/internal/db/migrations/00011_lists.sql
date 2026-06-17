-- +goose Up
CREATE TABLE lists (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  owner_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE list_items (
  list_id     TEXT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
  document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  added_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (list_id, document_id)
);

CREATE INDEX idx_list_items_list_id ON list_items(list_id);

-- +goose Down
DROP INDEX IF EXISTS idx_list_items_list_id;
DROP TABLE IF EXISTS list_items;
DROP TABLE IF EXISTS lists;
