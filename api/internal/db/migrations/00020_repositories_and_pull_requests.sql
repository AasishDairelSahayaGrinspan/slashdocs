-- +goose Up
-- Document repositories and their protected pull-request workflow.
CREATE TABLE repositories (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  scope TEXT NOT NULL CHECK (scope IN ('personal', 'organization')),
  namespace_id TEXT NOT NULL,
  namespace_name TEXT NOT NULL,
  root_folder_id TEXT NOT NULL UNIQUE REFERENCES folders(id),
  created_by_type TEXT NOT NULL,
  created_by_id TEXT NOT NULL,
  created_by_email TEXT NOT NULL DEFAULT '',
  created_by_name TEXT NOT NULL DEFAULT '',
  required_approvals INTEGER NOT NULL DEFAULT 1 CHECK (required_approvals >= 0),
  protected BOOLEAN NOT NULL DEFAULT true,
  next_pull_request_number BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX repositories_namespace_slug_idx
  ON repositories(scope, namespace_id, lower(slug));

-- Proposed versions have no published document version number until merge.
-- Existing version queries filter them out of the published version history.
ALTER TABLE versions ALTER COLUMN number DROP NOT NULL;

CREATE TABLE pull_requests (
  id TEXT PRIMARY KEY,
  repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
  number BIGINT NOT NULL,
  document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  base_version_id TEXT NOT NULL REFERENCES versions(id),
  head_version_id TEXT NOT NULL REFERENCES versions(id),
  title TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  change_summary TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'merged', 'closed')),
  author_type TEXT NOT NULL,
  author_id TEXT NOT NULL,
  author_email TEXT NOT NULL DEFAULT '',
  author_name TEXT NOT NULL DEFAULT '',
  merged_by_type TEXT,
  merged_by_id TEXT,
  merged_by_email TEXT,
  merged_by_name TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  merged_at TIMESTAMPTZ,
  closed_at TIMESTAMPTZ,
  UNIQUE(repository_id, number)
);

CREATE INDEX pull_requests_repository_status_idx
  ON pull_requests(repository_id, status, updated_at DESC);
CREATE INDEX pull_requests_document_idx
  ON pull_requests(document_id, created_at DESC);

CREATE TABLE pull_request_reviews (
  id TEXT PRIMARY KEY,
  pull_request_id TEXT NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
  head_version_id TEXT NOT NULL REFERENCES versions(id),
  reviewer_type TEXT NOT NULL,
  reviewer_id TEXT NOT NULL,
  reviewer_email TEXT NOT NULL DEFAULT '',
  reviewer_name TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL CHECK (state IN ('approved', 'changes_requested')),
  body TEXT NOT NULL DEFAULT '',
  submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(pull_request_id, reviewer_type, reviewer_id)
);

CREATE INDEX pull_request_reviews_head_idx
  ON pull_request_reviews(pull_request_id, head_version_id);

-- +goose Down
DROP TABLE IF EXISTS pull_request_reviews;
DROP TABLE IF EXISTS pull_requests;
DELETE FROM comment_placements
WHERE version_id IN (SELECT id FROM versions WHERE number IS NULL);
DELETE FROM comments
WHERE created_on_version_id IN (SELECT id FROM versions WHERE number IS NULL);
DELETE FROM versions WHERE number IS NULL;
ALTER TABLE versions ALTER COLUMN number SET NOT NULL;
DROP INDEX IF EXISTS repositories_namespace_slug_idx;
DROP TABLE IF EXISTS repositories;
