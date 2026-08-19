-- +goose Up
-- Avatar-review comments carry a per-comment display identity ("Priya's
-- avatar") and a severity tag. Both are empty for ordinary human comments.
ALTER TABLE comments ADD COLUMN author_display TEXT NOT NULL DEFAULT '';
ALTER TABLE comments ADD COLUMN severity TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE comments DROP COLUMN IF EXISTS severity;
ALTER TABLE comments DROP COLUMN IF EXISTS author_display;
