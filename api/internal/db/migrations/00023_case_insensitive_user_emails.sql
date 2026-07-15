-- +goose Up
-- Email addresses are identity keys in aidocs. Historical releases compared
-- them case-sensitively, which allowed an authenticated account and one or
-- more invitation placeholders for the same address to coexist. Consolidate
-- those rows before making both equality and uniqueness case-insensitive.
--
-- A group containing two authenticated subjects is intentionally rejected:
-- choosing either subject would silently break the other login. Operators can
-- resolve that exceptional identity conflict explicitly and rerun migration.
CREATE EXTENSION IF NOT EXISTS citext;

-- +goose StatementBegin
DO $$
DECLARE
  conflicting_email TEXT;
BEGIN
  SELECT lower(btrim(email))
    INTO conflicting_email
  FROM users
  GROUP BY lower(btrim(email))
  HAVING count(*) FILTER (WHERE google_sub IS NOT NULL) > 1
  ORDER BY lower(btrim(email))
  LIMIT 1;

  IF conflicting_email IS NOT NULL THEN
    RAISE EXCEPTION 'cannot merge multiple authenticated users for normalized email %', conflicting_email
      USING HINT = 'Reconcile the duplicate authenticated identities before rerunning migration 00023.';
  END IF;
END;
$$;
-- +goose StatementEnd

-- Keep migration-only relations inside PL/pgSQL so schema generators do not
-- treat temporary reconciliation state as part of the application model.
-- IDs are embedded in connected-knowledge JSON payloads; the rewrite table
-- also records later grant/review consolidation.
-- +goose StatementBegin
DO $$
BEGIN
  CREATE TEMP TABLE aidocs_user_email_merge ON COMMIT DROP AS
  WITH ranked AS (
    SELECT
      id AS old_id,
      first_value(id) OVER (
        PARTITION BY lower(btrim(email))
        ORDER BY (google_sub IS NOT NULL) DESC, created_at, id
      ) AS canonical_id
    FROM users
  )
  SELECT old_id, canonical_id
  FROM ranked
  WHERE old_id <> canonical_id;

  CREATE UNIQUE INDEX aidocs_user_email_merge_old_idx
    ON aidocs_user_email_merge(old_id);

  CREATE TEMP TABLE aidocs_identity_rewrites (
    old_value TEXT PRIMARY KEY,
    new_value TEXT NOT NULL
  ) ON COMMIT DROP;

  INSERT INTO aidocs_identity_rewrites(old_value, new_value)
  SELECT old_id, canonical_id FROM aidocs_user_email_merge;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION pg_temp.aidocs_replace_jsonb_string(input_value JSONB, old_value TEXT, new_value TEXT)
RETURNS JSONB
LANGUAGE SQL
IMMUTABLE
AS $function$
  SELECT CASE jsonb_typeof(input_value)
    WHEN 'string' THEN
      CASE WHEN input_value = to_jsonb(old_value) THEN to_jsonb(new_value) ELSE input_value END
    WHEN 'array' THEN
      COALESCE((
        SELECT jsonb_agg(
          pg_temp.aidocs_replace_jsonb_string(element, old_value, new_value)
          ORDER BY ordinal
        )
        FROM jsonb_array_elements(input_value) WITH ORDINALITY AS items(element, ordinal)
      ), '[]'::jsonb)
    WHEN 'object' THEN
      COALESCE((
        SELECT jsonb_object_agg(
          key,
          pg_temp.aidocs_replace_jsonb_string(element, old_value, new_value)
        )
        FROM jsonb_each(input_value) AS items(key, element)
      ), '{}'::jsonb)
    ELSE input_value
  END;
$function$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
DECLARE
  identity RECORD;
BEGIN
  FOR identity IN
    SELECT old_id, canonical_id
    FROM aidocs_user_email_merge
    ORDER BY old_id
  LOOP
    -- Keep the authenticated profile (or oldest placeholder) while filling any
    -- blank display fields from the duplicate before it is removed.
    UPDATE users AS canonical
    SET name = CASE WHEN canonical.name = '' THEN duplicate.name ELSE canonical.name END,
        picture_url = CASE WHEN canonical.picture_url = '' THEN duplicate.picture_url ELSE canonical.picture_url END,
        created_at = LEAST(canonical.created_at, duplicate.created_at)
    FROM users AS duplicate
    WHERE canonical.id = identity.canonical_id
      AND duplicate.id = identity.old_id;

    -- A principal may have been invited under both casings. Collapse duplicate
    -- grants to one row, retaining the strongest role and active state.
    INSERT INTO aidocs_identity_rewrites(old_value, new_value)
    SELECT duplicate_grant.id, canonical_grant.id
    FROM resource_grants AS duplicate_grant
    JOIN resource_grants AS canonical_grant
      ON canonical_grant.resource_type = duplicate_grant.resource_type
     AND canonical_grant.resource_id = duplicate_grant.resource_id
     AND canonical_grant.principal_type = 'user'
     AND canonical_grant.principal_id = identity.canonical_id
    WHERE duplicate_grant.principal_type = 'user'
      AND duplicate_grant.principal_id = identity.old_id
    ON CONFLICT (old_value) DO UPDATE SET new_value = EXCLUDED.new_value;

    UPDATE resource_grants AS canonical_grant
    SET role = CASE
          WHEN (CASE duplicate_grant.role WHEN 'owner' THEN 4 WHEN 'editor' THEN 3 WHEN 'commenter' THEN 2 ELSE 1 END)
             > (CASE canonical_grant.role WHEN 'owner' THEN 4 WHEN 'editor' THEN 3 WHEN 'commenter' THEN 2 ELSE 1 END)
            THEN duplicate_grant.role
          ELSE canonical_grant.role
        END,
        revoked_at = CASE
          WHEN canonical_grant.revoked_at IS NULL OR duplicate_grant.revoked_at IS NULL THEN NULL
          ELSE GREATEST(canonical_grant.revoked_at, duplicate_grant.revoked_at)
        END,
        created_at = LEAST(canonical_grant.created_at, duplicate_grant.created_at)
    FROM resource_grants AS duplicate_grant
    WHERE duplicate_grant.principal_type = 'user'
      AND duplicate_grant.principal_id = identity.old_id
      AND canonical_grant.principal_type = 'user'
      AND canonical_grant.principal_id = identity.canonical_id
      AND canonical_grant.resource_type = duplicate_grant.resource_type
      AND canonical_grant.resource_id = duplicate_grant.resource_id;

    DELETE FROM resource_grants AS duplicate_grant
    USING resource_grants AS canonical_grant
    WHERE duplicate_grant.principal_type = 'user'
      AND duplicate_grant.principal_id = identity.old_id
      AND canonical_grant.principal_type = 'user'
      AND canonical_grant.principal_id = identity.canonical_id
      AND canonical_grant.resource_type = duplicate_grant.resource_type
      AND canonical_grant.resource_id = duplicate_grant.resource_id;

    UPDATE resource_grants
    SET principal_id = identity.canonical_id
    WHERE principal_type = 'user' AND principal_id = identity.old_id;

    -- Workspace and pin rows use the principal ID in their primary key. Merge
    -- collisions while preserving the earliest add/pin and latest open time.
    INSERT INTO document_workspace_entries(document_id, principal_type, principal_id, added_at)
    SELECT document_id, 'user', identity.canonical_id, added_at
    FROM document_workspace_entries
    WHERE principal_type = 'user' AND principal_id = identity.old_id
    ON CONFLICT (document_id, principal_type, principal_id)
    DO UPDATE SET added_at = LEAST(document_workspace_entries.added_at, EXCLUDED.added_at);
    DELETE FROM document_workspace_entries
    WHERE principal_type = 'user' AND principal_id = identity.old_id;

    INSERT INTO folder_workspace_entries(folder_id, principal_type, principal_id, added_at, last_opened_at)
    SELECT folder_id, 'user', identity.canonical_id, added_at, last_opened_at
    FROM folder_workspace_entries
    WHERE principal_type = 'user' AND principal_id = identity.old_id
    ON CONFLICT (folder_id, principal_type, principal_id)
    DO UPDATE SET
      added_at = LEAST(folder_workspace_entries.added_at, EXCLUDED.added_at),
      last_opened_at = GREATEST(folder_workspace_entries.last_opened_at, EXCLUDED.last_opened_at);
    DELETE FROM folder_workspace_entries
    WHERE principal_type = 'user' AND principal_id = identity.old_id;

    INSERT INTO pins(principal_type, principal_id, resource_type, resource_id, pinned_at)
    SELECT 'user', identity.canonical_id, resource_type, resource_id, pinned_at
    FROM pins
    WHERE principal_type = 'user' AND principal_id = identity.old_id
    ON CONFLICT (principal_type, principal_id, resource_type, resource_id)
    DO UPDATE SET pinned_at = LEAST(pins.pinned_at, EXCLUDED.pinned_at);
    DELETE FROM pins
    WHERE principal_type = 'user' AND principal_id = identity.old_id;

    -- Reviews are the current verdict per reviewer and pull request. If both
    -- aliases reviewed the same head, retain the most recently submitted
    -- verdict and keep one stable review ID.
    INSERT INTO aidocs_identity_rewrites(old_value, new_value)
    SELECT duplicate_review.id, canonical_review.id
    FROM pull_request_reviews AS duplicate_review
    JOIN pull_request_reviews AS canonical_review
      ON canonical_review.pull_request_id = duplicate_review.pull_request_id
     AND canonical_review.reviewer_type = 'user'
     AND canonical_review.reviewer_id = identity.canonical_id
    WHERE duplicate_review.reviewer_type = 'user'
      AND duplicate_review.reviewer_id = identity.old_id
    ON CONFLICT (old_value) DO UPDATE SET new_value = EXCLUDED.new_value;

    UPDATE pull_request_reviews AS canonical_review
    SET head_version_id = duplicate_review.head_version_id,
        patchset = duplicate_review.patchset,
        reviewer_email = duplicate_review.reviewer_email,
        reviewer_name = duplicate_review.reviewer_name,
        state = duplicate_review.state,
        body = duplicate_review.body,
        submitted_at = duplicate_review.submitted_at
    FROM pull_request_reviews AS duplicate_review
    WHERE duplicate_review.reviewer_type = 'user'
      AND duplicate_review.reviewer_id = identity.old_id
      AND canonical_review.reviewer_type = 'user'
      AND canonical_review.reviewer_id = identity.canonical_id
      AND canonical_review.pull_request_id = duplicate_review.pull_request_id
      AND duplicate_review.submitted_at >= canonical_review.submitted_at;

    DELETE FROM pull_request_reviews AS duplicate_review
    USING pull_request_reviews AS canonical_review
    WHERE duplicate_review.reviewer_type = 'user'
      AND duplicate_review.reviewer_id = identity.old_id
      AND canonical_review.reviewer_type = 'user'
      AND canonical_review.reviewer_id = identity.canonical_id
      AND canonical_review.pull_request_id = duplicate_review.pull_request_id;

    UPDATE pull_request_reviews
    SET reviewer_id = identity.canonical_id
    WHERE reviewer_type = 'user' AND reviewer_id = identity.old_id;

    -- Rewrite every remaining relational identity reference before deleting
    -- the placeholder. Polymorphic columns are guarded by their type column;
    -- real foreign keys are updated unconditionally.
    UPDATE documents SET owner_id = identity.canonical_id
    WHERE owner_type = 'user' AND owner_id = identity.old_id;
    UPDATE versions SET created_by_id = identity.canonical_id
    WHERE created_by_type = 'user' AND created_by_id = identity.old_id;
    UPDATE service_accounts SET owner_user_id = identity.canonical_id
    WHERE owner_user_id = identity.old_id;
    UPDATE service_accounts SET created_by_user_id = identity.canonical_id
    WHERE created_by_user_id = identity.old_id;
    UPDATE cli_credentials SET user_id = identity.canonical_id
    WHERE user_id = identity.old_id;
    UPDATE resource_grants SET granted_by_user_id = identity.canonical_id
    WHERE granted_by_type = 'user' AND granted_by_user_id = identity.old_id;
    UPDATE comments SET author_id = identity.canonical_id
    WHERE author_type = 'user' AND author_id = identity.old_id;
    UPDATE ownership_transfers SET from_user_id = identity.canonical_id
    WHERE from_user_id = identity.old_id;
    UPDATE ownership_transfers SET to_user_id = identity.canonical_id
    WHERE to_user_id = identity.old_id;
    UPDATE folders SET owner_id = identity.canonical_id
    WHERE owner_type = 'user' AND owner_id = identity.old_id;
    UPDATE comment_mentions SET principal_id = identity.canonical_id
    WHERE principal_type = 'user' AND principal_id = identity.old_id;
    UPDATE document_kv SET updated_by_id = identity.canonical_id
    WHERE updated_by_type = 'user' AND updated_by_id = identity.old_id;
    UPDATE document_kv_events SET updated_by_id = identity.canonical_id
    WHERE updated_by_type = 'user' AND updated_by_id = identity.old_id;
    UPDATE repositories SET namespace_id = identity.canonical_id
    WHERE scope = 'personal' AND namespace_id = identity.old_id;
    UPDATE repositories SET created_by_id = identity.canonical_id
    WHERE created_by_type = 'user' AND created_by_id = identity.old_id;
    UPDATE pull_requests SET author_id = identity.canonical_id
    WHERE author_type = 'user' AND author_id = identity.old_id;
    UPDATE pull_requests SET merged_by_id = identity.canonical_id
    WHERE merged_by_type = 'user' AND merged_by_id = identity.old_id;
    UPDATE connected_knowledge_records SET user_id = identity.canonical_id
    WHERE user_id = identity.old_id;

    DELETE FROM users WHERE id = identity.old_id;
  END LOOP;
END;
$$;
-- +goose StatementEnd

-- Rewrite structured JSON references after all grant/review mappings have
-- been collected. This covers reviewer/team arrays, transfer state, source
-- creators, audit actors, notification recipients, and in-flight OAuth state.
-- +goose StatementBegin
DO $$
DECLARE
  rewrite RECORD;
BEGIN
  FOR rewrite IN
    SELECT old_value, new_value
    FROM aidocs_identity_rewrites
    ORDER BY old_value
  LOOP
    UPDATE connected_knowledge_records
    SET payload = pg_temp.aidocs_replace_jsonb_string(payload, rewrite.old_value, rewrite.new_value)
    WHERE payload <> pg_temp.aidocs_replace_jsonb_string(payload, rewrite.old_value, rewrite.new_value);

    UPDATE oauth_states
    SET state_json = pg_temp.aidocs_replace_jsonb_string(state_json, rewrite.old_value, rewrite.new_value)
    WHERE state_json <> pg_temp.aidocs_replace_jsonb_string(state_json, rewrite.old_value, rewrite.new_value);
  END LOOP;
END;
$$;
-- +goose StatementEnd

UPDATE users SET email = lower(btrim(email));

-- Changing the column type rebuilds users_email_key with citext equality.
-- Existing and rolling-version queries that use `email = $1` therefore become
-- case-insensitive without relying on every caller to normalize first.
ALTER TABLE users
  ALTER COLUMN email TYPE CITEXT USING email::citext;

-- +goose Down
-- Identity consolidation is intentionally irreversible: splitting merged ACL,
-- ownership, and activity rows would require inventing provenance. Reverting
-- restores the TEXT type while retaining the safe normalized identities.
ALTER TABLE users
  ALTER COLUMN email TYPE TEXT USING email::text;
