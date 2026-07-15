-- +goose Up
-- Extend repository pull requests to atomic multi-document change sets.
-- Keep the original pull-request columns as a backwards-compatible projection
-- of the first change. Multi-document and create-only pull requests may not
-- have an existing document/version to project, so those columns become null.
ALTER TABLE pull_requests
  ALTER COLUMN document_id DROP NOT NULL,
  ALTER COLUMN base_version_id DROP NOT NULL,
  ALTER COLUMN head_version_id DROP NOT NULL,
  ADD COLUMN patchset INTEGER NOT NULL DEFAULT 1 CHECK (patchset >= 1);

ALTER TABLE pull_requests DROP CONSTRAINT pull_requests_document_id_fkey;
ALTER TABLE pull_requests
  ADD CONSTRAINT pull_requests_document_id_fkey
  FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE SET NULL;
ALTER TABLE pull_requests DROP CONSTRAINT pull_requests_base_version_id_fkey;
ALTER TABLE pull_requests
  ADD CONSTRAINT pull_requests_base_version_id_fkey
  FOREIGN KEY (base_version_id) REFERENCES versions(id) ON DELETE SET NULL;
ALTER TABLE pull_requests DROP CONSTRAINT pull_requests_head_version_id_fkey;
ALTER TABLE pull_requests
  ADD CONSTRAINT pull_requests_head_version_id_fkey
  FOREIGN KEY (head_version_id) REFERENCES versions(id) ON DELETE SET NULL;

ALTER TABLE pull_request_reviews
  ALTER COLUMN head_version_id DROP NOT NULL,
  ADD COLUMN patchset INTEGER NOT NULL DEFAULT 1 CHECK (patchset >= 1);
ALTER TABLE pull_request_reviews DROP CONSTRAINT pull_request_reviews_head_version_id_fkey;
ALTER TABLE pull_request_reviews
  ADD CONSTRAINT pull_request_reviews_head_version_id_fkey
  FOREIGN KEY (head_version_id) REFERENCES versions(id) ON DELETE SET NULL;

CREATE TABLE pull_request_changes (
  id TEXT PRIMARY KEY,
  pull_request_id TEXT NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
  patchset INTEGER NOT NULL CHECK (patchset >= 1),
  position INTEGER NOT NULL CHECK (position >= 1),
  operation TEXT NOT NULL CHECK (operation IN ('update', 'create', 'rename', 'move', 'delete')),
  document_id TEXT,
  folder_id TEXT,
  base_version_id TEXT,
  head_version_id TEXT,
  base_title TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  base_path TEXT NOT NULL DEFAULT '',
  path TEXT NOT NULL DEFAULT '',
  head_html_blob_key TEXT,
  head_sha256 TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (pull_request_id, patchset, position)
);

CREATE INDEX pull_request_changes_current_idx
  ON pull_request_changes(pull_request_id, patchset, position);
CREATE INDEX pull_request_changes_document_idx
  ON pull_request_changes(document_id, created_at DESC);

-- Materialize existing single-document pull requests as one update change.
INSERT INTO pull_request_changes(
  id,pull_request_id,patchset,position,operation,document_id,folder_id,
  base_version_id,head_version_id,base_title,title,base_path,path,
  head_html_blob_key,head_sha256
)
SELECT pr.id || '_change_1_1',pr.id,1,1,'update',pr.document_id,d.parent_folder_id,
       pr.base_version_id,pr.head_version_id,d.title,d.title,
       COALESCE(d.parent_folder_id,''),COALESCE(d.parent_folder_id,''),
       v.html_blob_key,v.sha256
FROM pull_requests pr
JOIN documents d ON d.id=pr.document_id
JOIN versions v ON v.id=pr.head_version_id;

-- +goose Down
DROP TABLE IF EXISTS pull_request_changes;

-- Rows which deleted their compatibility document/version cannot be represented
-- by the legacy schema. Remove only those rows before restoring its constraints.
DELETE FROM pull_requests
WHERE document_id IS NULL OR base_version_id IS NULL OR head_version_id IS NULL;

ALTER TABLE pull_request_reviews DROP CONSTRAINT pull_request_reviews_head_version_id_fkey;
ALTER TABLE pull_request_reviews
  ADD CONSTRAINT pull_request_reviews_head_version_id_fkey
  FOREIGN KEY (head_version_id) REFERENCES versions(id);
ALTER TABLE pull_request_reviews
  ALTER COLUMN head_version_id SET NOT NULL,
  DROP COLUMN patchset;

ALTER TABLE pull_requests DROP CONSTRAINT pull_requests_document_id_fkey;
ALTER TABLE pull_requests
  ADD CONSTRAINT pull_requests_document_id_fkey
  FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE;
ALTER TABLE pull_requests DROP CONSTRAINT pull_requests_base_version_id_fkey;
ALTER TABLE pull_requests
  ADD CONSTRAINT pull_requests_base_version_id_fkey
  FOREIGN KEY (base_version_id) REFERENCES versions(id);
ALTER TABLE pull_requests DROP CONSTRAINT pull_requests_head_version_id_fkey;
ALTER TABLE pull_requests
  ADD CONSTRAINT pull_requests_head_version_id_fkey
  FOREIGN KEY (head_version_id) REFERENCES versions(id);
ALTER TABLE pull_requests
  ALTER COLUMN document_id SET NOT NULL,
  ALTER COLUMN base_version_id SET NOT NULL,
  ALTER COLUMN head_version_id SET NOT NULL,
  DROP COLUMN patchset;
