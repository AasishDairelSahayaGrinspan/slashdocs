-- +goose Up
CREATE TABLE comment_mentions (
  comment_id TEXT NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
  principal_type TEXT NOT NULL CHECK (principal_type IN ('user', 'service_account')),
  principal_id TEXT NOT NULL CHECK (principal_id <> ''),
  offset_utf16 INTEGER NOT NULL CHECK (offset_utf16 >= 0),
  length_utf16 INTEGER NOT NULL CHECK (length_utf16 > 0),
  label_text TEXT NOT NULL CHECK (label_text <> ''),
  PRIMARY KEY (comment_id, offset_utf16)
);

COMMENT ON TABLE comment_mentions IS
  'Structured principals mentioned in comment bodies. Mention metadata grants no access; future notification delivery must re-check effective document access and apply rate limits.';
COMMENT ON COLUMN comment_mentions.offset_utf16 IS
  'Zero-based offset in UTF-16 code units, matching JavaScript string indexes.';
COMMENT ON COLUMN comment_mentions.length_utf16 IS
  'Mention length in UTF-16 code units, matching JavaScript string indexes.';
COMMENT ON COLUMN comment_mentions.label_text IS
  'Internal snapshot of the exact visible @label, used to preserve valid mentions across principal renames.';

-- Supports future notification delivery by mentioned principal. The primary
-- key already covers comment-scoped reads ordered by offset.
CREATE INDEX idx_comment_mentions_principal
  ON comment_mentions(principal_type, principal_id, comment_id, offset_utf16);

-- Keep mixed-version rolling deployments safe. Older servers may still write
-- U+2063 markers, and body-only updates do not know how to maintain structured
-- ranges. Normalize every writer and clear stale ranges before the row changes;
-- the new server replaces them later in the same transaction when supplied.
-- +goose StatementBegin
CREATE FUNCTION normalize_comment_body_mentions() RETURNS trigger AS $$
BEGIN
  IF strpos(NEW.body, chr(8291)) > 0 THEN
    NEW.body := regexp_replace(NEW.body, chr(8291), '', 'g');
  END IF;

  IF TG_OP = 'UPDATE' AND NEW.body IS DISTINCT FROM OLD.body THEN
    DELETE FROM comment_mentions WHERE comment_id = OLD.id;
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER normalize_comment_body_mentions_trigger
BEFORE INSERT OR UPDATE OF body ON comments
FOR EACH ROW EXECUTE FUNCTION normalize_comment_body_mentions();

-- +goose Down
DROP TRIGGER IF EXISTS normalize_comment_body_mentions_trigger ON comments;
DROP FUNCTION IF EXISTS normalize_comment_body_mentions();
DROP INDEX IF EXISTS idx_comment_mentions_principal;
DROP TABLE IF EXISTS comment_mentions;
