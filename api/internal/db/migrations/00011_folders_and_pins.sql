CREATE TABLE IF NOT EXISTS folders (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  owner_type TEXT NOT NULL,
  owner_id TEXT NOT NULL,
  parent_folder_id TEXT NULL REFERENCES folders(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (parent_folder_id IS NULL OR parent_folder_id <> id)
);

ALTER TABLE documents ADD COLUMN IF NOT EXISTS parent_folder_id TEXT NULL REFERENCES folders(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS pins (
  principal_type TEXT NOT NULL,
  principal_id TEXT NOT NULL,
  resource_type TEXT NOT NULL CHECK (resource_type IN ('document','folder')),
  resource_id TEXT NOT NULL,
  pinned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (principal_type, principal_id, resource_type, resource_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS folders_owner_sibling_title_idx
  ON folders (owner_type, owner_id, COALESCE(parent_folder_id, ''), lower(title));
CREATE INDEX IF NOT EXISTS folders_parent_folder_id_idx ON folders(parent_folder_id);
CREATE INDEX IF NOT EXISTS documents_parent_folder_id_idx ON documents(parent_folder_id);
CREATE INDEX IF NOT EXISTS pins_principal_idx ON pins(principal_type, principal_id);
