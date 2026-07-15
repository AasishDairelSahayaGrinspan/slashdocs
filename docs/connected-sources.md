# Connected sources

Connected sources let an aidocs repository track one or more external code or
knowledge repositories. A source event never edits published documentation
directly. aidocs filters the changed paths, records an auditable sync run, and
opens or updates a protected documentation pull request for human review.

## Try it locally

Start the source checkout from the project root:

```bash
aidocs-dev
```

The local app includes a `Docs playground` repository, a seeded GitHub source,
an owner, a reviewer team, review rules, and a public portal. Open the
repository's **Sources** tab and use **Simulate** to preview a path change. Use
**Run check** to record it and create a pull request. Switch identities at
<http://127.0.0.1:8080/__dev> to approve and merge it as the reviewer.

The same flow is available through the normal `aidocs` CLI when it is logged in
to the local server:

```bash
aidocs repos list
aidocs repos sources list repo_1

# Read-only: reports whether this path would matter.
aidocs repos sources simulate repo_1 source_dev_github \
  --after local-preview \
  --changed-file api/refunds/handler.go

# Durable: records the run and opens or updates a protected doc PR.
aidocs repos sources check repo_1 source_dev_github \
  --after local-check-1 \
  --changed-file api/refunds/handler.go

aidocs repos sync-runs repo_1
aidocs repos activity repo_1
aidocs inbox
```

Passing `--changed-file` keeps the local workflow fully offline. Without it,
aidocs calls the configured provider to compare `--before` and `--after`.

## Connect a repository

Each aidocs repository may have several independently configured sources:

```bash
aidocs repos sources add repo_123 \
  --provider github \
  --repository razorpay/payments \
  --branch master \
  --include 'api/**,docs/**,README.md' \
  --exclude 'vendor/**,**/node_modules/**' \
  --target-document doc_123 \
  --trigger default_branch \
  --direction into_aidocs
```

Important source fields:

- `provider` identifies GitHub, Slack, DevRev, Google Drive, Confluence,
  Notion, or a manual source.
- `external_repository` is the provider-side repository or source identity.
- `branch` is the tracked default branch.
- `include_paths` and `exclude_paths` decide whether a change is relevant.
- `target_document_id` identifies the document updated by the current
  deterministic generator. Folder targets are retained for multi-document
  generators.
- `trigger_mode` supports default-branch pushes, merged pull requests, Slack or
  DevRev events, manual checks, and scheduled reconciliation.
- `direction` supports imports, bidirectional proposals, or export-only
  proposals. Exports are proposals; aidocs never pushes directly to a provider's
  default branch.

GitHub has a live compare client. Slack and DevRev have signed, replay-protected
webhook ingestion. The remaining knowledge providers share the source,
path-filter, sync-run, provenance, and proposal model and can be tested with
explicit changed paths.

## Configure GitHub webhooks

Set these server variables:

```text
AIDOCS_GITHUB_TOKEN=<fine-grained token or installation token>
AIDOCS_GITHUB_WEBHOOK_SECRET=<random shared secret>
```

Use a token with read-only Contents access, scoped only to repositories aidocs
may index. Configure the GitHub webhook URL as:

```text
https://aidocs.example.com/v1/webhooks/github
```

Choose JSON payloads, enter the same webhook secret, and subscribe to push and
pull-request events. aidocs accepts tracked default-branch pushes and merged
pull requests, verifies `X-Hub-Signature-256`, deduplicates
`X-GitHub-Delivery`, persists a queued run, and acknowledges before processing
the documentation proposal asynchronously.

If a later revision arrives while that source's pull request remains open,
aidocs adds a new patchset and dismisses approvals tied to the older patchset.
If the previous pull request was merged or closed, it opens a new one. Replaying
the same completed revision is a visible no-op.

## Connect Slack threads

Create a Slack source with `provider=slack`, `trigger_mode=event`, and an
`external_repository` in one of these forms:

- `TEAM_ID/CHANNEL_ID` tracks message and thread events in that channel.
- `TEAM_ID/CHANNEL_ID/THREAD_TS` tracks one thread.

Set `AIDOCS_SLACK_SIGNING_SECRET` to the Slack app's signing secret and use this
Events API Request URL:

```text
https://aidocs.example.com/v1/webhooks/slack
```

Subscribe the app to the message events needed for the channel types you use.
aidocs verifies `X-Slack-Signature` over the exact raw request body, rejects
timestamps outside Slack's five-minute replay window, completes the signed URL
verification challenge, and deduplicates `event_id`. Message text is treated as
untrusted input and escaped into the generated pull-request document.

## Connect DevRev tickets

Create a DevRev source with `provider=devrev`, `trigger_mode=event`, and set
`external_repository` to the DevRev webhook ID. This tracks every ticket event
delivered by that webhook. Set it to one ticket's DON or display ID instead to
track only that ticket.

When creating the DevRev webhook, subscribe to `work_created`, `work_updated`,
and `work_deleted`, configure this URL, and explicitly supply the same 8-32 byte
secret held in `AIDOCS_DEVREV_WEBHOOK_SECRET`:

```text
https://aidocs.example.com/v1/webhooks/devrev
```

aidocs verifies the bare hex `X-DevRev-Signature` HMAC-SHA256 over the raw body,
checks the RFC3339 event timestamp against a five-minute replay window, answers
the signed verification challenge, and deduplicates the event `id`. Ticket
title, body, and state are escaped into the reviewable document proposal.

## Review and provenance

Connected-source pull requests are authored by the `Connected Knowledge`
integration principal. They obey the repository's required approval count and
show readable document-text changes. The sync run stores the provider, source,
immutable revision URL, affected paths, target document/version, outcome, and
pull-request number.

Repository settings can require named users or teams globally or by path
pattern. Matching users receive review notifications, and merge remains blocked
until the current patchset has the configured approvals. Search and cited
questions only inspect documents the caller can read. Public portals must be
explicitly enabled and only return the selected repository's published content.

## Operational behavior

- Queued work is persisted in PostgreSQL and stale `processing` runs are
  recovered after restart.
- Duplicate webhook deliveries and completed source revisions are idempotent.
- Excluded or unrelated changes finish as `no_op`; they do not create PRs.
- Archived repositories remain readable but source events cannot change them.
- Source failures are recorded in sync history and freshness state.
- Source data is treated as untrusted content and cannot change credentials,
  access rules, or review policy.

See [Connected knowledge repositories](proposals/connected-knowledge.md) for
the model and acceptance ledger.
