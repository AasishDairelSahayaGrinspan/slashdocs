-- +goose Up
ALTER TABLE folder_workspace_entries ADD COLUMN last_opened_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- +goose Down
ALTER TABLE folder_workspace_entries DROP COLUMN last_opened_at;
