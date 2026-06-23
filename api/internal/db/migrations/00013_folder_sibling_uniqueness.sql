-- +goose Up
-- Inside any parent folder, titles must be unique regardless of owner.
-- Root-level folders (parent_folder_id IS NULL) keep per-owner uniqueness via the existing index.
CREATE UNIQUE INDEX folders_parent_title_idx
  ON folders (parent_folder_id, lower(title))
  WHERE parent_folder_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS folders_parent_title_idx;
