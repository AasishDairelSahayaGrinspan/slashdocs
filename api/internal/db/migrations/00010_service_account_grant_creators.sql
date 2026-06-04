-- +goose Up
ALTER TABLE resource_grants DROP CONSTRAINT IF EXISTS resource_grants_granted_by_user_id_fkey;

ALTER TABLE resource_grants
  ADD COLUMN granted_by_type TEXT NOT NULL DEFAULT 'user';

CREATE INDEX idx_grants_granted_by
  ON resource_grants(granted_by_type, granted_by_user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_grants_granted_by;

ALTER TABLE resource_grants DROP COLUMN IF EXISTS granted_by_type;

ALTER TABLE resource_grants
  ADD CONSTRAINT resource_grants_granted_by_user_id_fkey FOREIGN KEY (granted_by_user_id) REFERENCES users(id);
