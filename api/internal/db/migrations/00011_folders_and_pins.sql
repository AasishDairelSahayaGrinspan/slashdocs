-- +goose Up

-- folders: like documents but without versions/HTML — just metadata and grants
CREATE TABLE folders (
  id         TEXT PRIMARY KEY,
  title      TEXT NOT NULL,
  owner_type TEXT NOT NULL,
  owner_id   TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- folder_documents: many-to-many between folders and documents
CREATE TABLE folder_documents (
  folder_id     TEXT NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
  document_id   TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  added_by_type TEXT NOT NULL,
  added_by_id   TEXT NOT NULL,
  PRIMARY KEY (folder_id, document_id)
);

-- pins: per-principal pins for documents and folders
CREATE TABLE pins (
  principal_type TEXT NOT NULL,
  principal_id   TEXT NOT NULL,
  resource_type  TEXT NOT NULL,
  resource_id    TEXT NOT NULL,
  pinned_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (principal_type, principal_id, resource_type, resource_id)
);

CREATE INDEX idx_folder_docs_document ON folder_documents(document_id);
CREATE INDEX idx_pins_principal ON pins(principal_type, principal_id);

-- +goose Down
DROP INDEX IF EXISTS idx_pins_principal;
DROP INDEX IF EXISTS idx_folder_docs_document;
DROP TABLE IF EXISTS pins;
DROP TABLE IF EXISTS folder_documents;
DROP TABLE IF EXISTS folders;
