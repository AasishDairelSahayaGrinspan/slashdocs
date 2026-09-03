-- +goose Up
CREATE TABLE google_groups (
  id TEXT PRIMARY KEY CHECK (id ~ '^groups/[^/[:space:]]+$'),
  customer_id TEXT NOT NULL CHECK (customer_id ~ '^C[^/[:space:]]+$'),
  email TEXT NOT NULL CHECK (position('@' in email) > 1 AND email !~ '[[:space:]]'),
  display_name TEXT NOT NULL DEFAULT '',
  last_verified_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Group addresses can change, so ACLs reference google_groups.id. This index
-- only prevents two current metadata rows from claiming the same address.
CREATE UNIQUE INDEX google_groups_email_ci_unique ON google_groups (lower(email));

CREATE TABLE google_group_memberships (
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  group_id TEXT NOT NULL REFERENCES google_groups(id) ON DELETE CASCADE,
  synced_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (user_id, group_id)
);

CREATE INDEX google_group_memberships_group_idx ON google_group_memberships (group_id);

CREATE TABLE google_group_sync_state (
  user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  last_attempt_at TIMESTAMPTZ NOT NULL,
  last_success_at TIMESTAMPTZ,
  last_error_code TEXT NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE IF EXISTS google_group_sync_state;
DROP INDEX IF EXISTS google_group_memberships_group_idx;
DROP TABLE IF EXISTS google_group_memberships;
DROP INDEX IF EXISTS google_groups_email_ci_unique;
DROP TABLE IF EXISTS google_groups;
