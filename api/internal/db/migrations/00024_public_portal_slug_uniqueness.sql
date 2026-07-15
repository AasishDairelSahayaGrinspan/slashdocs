-- +goose Up
-- Keep legacy settings aligned with the application/browser normalizer before
-- enforcing one enabled portal per public address.
UPDATE connected_knowledge_records
SET payload = jsonb_set(
  payload,
  '{portal,slug}',
  to_jsonb(LEFT(TRIM(BOTH '-' FROM REGEXP_REPLACE(LOWER(BTRIM(COALESCE(payload #>> '{portal,slug}', ''))), '[^a-z0-9]+', '-', 'g')), 80)),
  true
)
WHERE kind = 'settings' AND payload ? 'portal';

-- Invalid empty addresses cannot be routed. Disable them, and preserve the
-- earliest configured portal when historical rows normalize to the same slug.
UPDATE connected_knowledge_records
SET payload = jsonb_set(payload, '{portal,enabled}', 'false'::jsonb, true)
WHERE kind = 'settings'
  AND COALESCE(payload #>> '{portal,enabled}', 'false') = 'true'
  AND COALESCE(payload #>> '{portal,slug}', '') = '';

WITH ranked AS (
  SELECT
    id,
    ROW_NUMBER() OVER (
      PARTITION BY payload #>> '{portal,slug}'
      ORDER BY created_at, id
    ) AS position
  FROM connected_knowledge_records
  WHERE kind = 'settings'
    AND COALESCE(payload #>> '{portal,enabled}', 'false') = 'true'
    AND COALESCE(payload #>> '{portal,slug}', '') <> ''
)
UPDATE connected_knowledge_records AS records
SET payload = jsonb_set(records.payload, '{portal,enabled}', 'false'::jsonb, true)
FROM ranked
WHERE records.kind = 'settings'
  AND records.id = ranked.id
  AND ranked.position > 1;

CREATE UNIQUE INDEX connected_knowledge_enabled_portal_slug_uidx
  ON connected_knowledge_records (
    (LEFT(TRIM(BOTH '-' FROM REGEXP_REPLACE(LOWER(BTRIM(COALESCE(payload #>> '{portal,slug}', ''))), '[^a-z0-9]+', '-', 'g')), 80))
  )
  WHERE kind = 'settings'
    AND COALESCE(payload #>> '{portal,enabled}', 'false') = 'true'
    AND COALESCE(payload #>> '{portal,slug}', '') <> '';

-- +goose Down
DROP INDEX IF EXISTS connected_knowledge_enabled_portal_slug_uidx;
