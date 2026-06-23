-- +goose Up
CREATE TABLE folder_workspace_entries (
    folder_id      TEXT NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
    principal_type TEXT NOT NULL,
    principal_id   TEXT NOT NULL,
    added_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (folder_id, principal_type, principal_id)
);

CREATE INDEX idx_folder_workspace_entries_principal
    ON folder_workspace_entries (principal_type, principal_id);

-- +goose Down
DROP TABLE folder_workspace_entries;
