-- +goose Up
-- Enforce per-user uniqueness on list names so a user cannot accidentally
-- create two lists with the same name. Scoped to (owner_id, name) so two
-- different users can each have a list called "Tech Specs".
ALTER TABLE lists ADD CONSTRAINT lists_owner_name_unique UNIQUE (owner_id, name);

-- +goose Down
ALTER TABLE lists DROP CONSTRAINT IF EXISTS lists_owner_name_unique;
