-- +goose Up
ALTER TABLE comments ADD COLUMN edited_at TIMESTAMPTZ;
ALTER TABLE comments ADD COLUMN edit_revision BIGINT NOT NULL DEFAULT 0;

-- Each revision has its own delivery and immutable body, so quick successive
-- edits cannot overwrite pending messages or a worker's acknowledgement.
ALTER TABLE comment_notification_outbox ADD COLUMN revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE comment_notification_outbox ADD COLUMN body_snapshot TEXT;
ALTER TABLE comment_notification_outbox DROP CONSTRAINT comment_notification_outbox_pkey;
ALTER TABLE comment_notification_outbox ADD PRIMARY KEY (comment_id, recipient_id, revision);
ALTER TABLE comment_notification_outbox DROP CONSTRAINT comment_notification_outbox_reason_check;
ALTER TABLE comment_notification_outbox ADD CONSTRAINT comment_notification_outbox_reason_check
  CHECK (reason IN ('mention', 'reply', 'owner', 'edited_mention', 'edited_reply', 'edited_owner'));

-- +goose Down
DELETE FROM comment_notification_outbox WHERE revision > 0;
ALTER TABLE comment_notification_outbox DROP CONSTRAINT comment_notification_outbox_reason_check;
ALTER TABLE comment_notification_outbox ADD CONSTRAINT comment_notification_outbox_reason_check
  CHECK (reason IN ('mention', 'reply', 'owner'));
ALTER TABLE comment_notification_outbox DROP CONSTRAINT comment_notification_outbox_pkey;
ALTER TABLE comment_notification_outbox ADD PRIMARY KEY (comment_id, recipient_id);
ALTER TABLE comment_notification_outbox DROP COLUMN body_snapshot;
ALTER TABLE comment_notification_outbox DROP COLUMN revision;
ALTER TABLE comments DROP COLUMN edit_revision;
ALTER TABLE comments DROP COLUMN edited_at;
