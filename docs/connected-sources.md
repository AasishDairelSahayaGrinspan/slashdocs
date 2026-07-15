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
demo personal accounts for all four supported connected providers, an owner, a
reviewer team, review rules, and a public portal. Demo accounts do not call the
real provider APIs. Open the repository's **Sources** tab and use **Simulate**
to preview a path change. Use **Run check** to record it and create a pull
request. Switch identities at <http://127.0.0.1:8080/__dev> to approve and merge
it as the reviewer.

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

## Connect an account and source

On a configured production deployment, each person connects their own account
under **Settings > Connections**:

- GitHub, Slack, and Google Drive use OAuth. Tokens belong to the signed-in
  aidocs user and are never shared as organization-wide credentials.
- DevRev uses a personal access token (PAT) pasted by that user.
- Provider credentials are encrypted at rest. Disconnecting the account stops
  future checks but does not remove prior sync history or reviewed documents.

An aidocs repository owner can then open **Sources**, choose a provider and one
of their usable accounts, identify the repository/thread/file/ticket, select a
target document, and save the source. A source stores only the connected-account
ID; it never copies a token into repository configuration or source responses.

The CLI example below describes the legacy/manual source-definition path. The
current CLI does not bind a personal `connection_id`, so use the web UI for real
per-user provider connections. Explicit changed-file simulation remains useful
offline.

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

GitHub, a specific Slack thread, a selected Google Drive file, and a DevRev work
item can be inspected on demand through their linked personal accounts. GitHub,
Slack, and DevRev also retain signed, replay-protected webhook ingestion. The
remaining knowledge providers share the source, path-filter, sync-run,
provenance, and proposal model and can be tested with explicit changed paths.

## Personal Sync now and polling

**Sync now** starts an immediate provider read. The same inspection path also
runs automatically for active linked sources whose trigger is not `manual`.
The production interval defaults to 15 minutes and is configured with
`AIDOCS_SOURCE_POLL_INTERVAL`; set it to `0` to disable background polling or a
Go duration of at least `1m` to change it.

- **GitHub** reads the configured branch head. The first check records the
  repository tree as context; later checks compare the prior and current commit
  and propose the changed paths.
- **Slack** reads one `TEAM_ID/CHANNEL_ID/THREAD_TS` with
  `conversations.replies`, following cursor pages. Slack currently limits
  non-Marketplace apps to one request per minute and 15 messages per page. If
  Slack returns `429 Retry-After`, aidocs does not sleep inside the web request;
  wait for that interval and run the check again. See Slack's
  [`conversations.replies` reference](https://docs.slack.dev/reference/methods/conversations.replies/).
- **Google Drive** reads metadata and exports or downloads the one file chosen
  with Google Picker. The revision is a deterministic content hash.
- **DevRev** calls `works.get` for the configured ticket/work item and hashes
  the returned work state.

Every replica may discover a polling window, but `QueueDelivery` persists a
provider/source/window key in PostgreSQL. That key deduplicates the queue across
replicas, so each eligible source is enqueued at most once per interval. The
provider still sees one request per source. If many Slack thread sources share
one user token, use a longer interval or set those sources to a `manual` trigger
to stay within Slack's limits.

An unchanged revision records a visible no-op. A changed source with a target
document opens or updates a protected aidocs pull request; without a target it
remains a preview. Provider text is untrusted, escaped content. The check never
publishes directly and never approves its own pull request.

Provider OAuth clients, callbacks, credential encryption, and Google Picker are
configured by the deployment operator; see
[Self-hosting: personal connected accounts](self-hosting.md#personal-connected-accounts).
Picker is the one browser-token exception: a no-store endpoint returns a Drive
access token only to the owning signed-in user so Google's browser Picker can
open. Connection lists, repository sources, and other users never receive it.

The GitHub OAuth App flow requests only `repo`. GitHub's `repo`
scope is broad: the resulting token can read and write every public and private
repository available to that user even though aidocs only performs read calls.
GitHub OAuth Apps cannot make private source-code access read-only or let the
user select individual repositories. Do not enable GitHub personal connections
if this permission is unacceptable. See
[GitHub's OAuth scope reference](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/scopes-for-oauth-apps).

## Configure GitHub webhooks

Set these server variables:

```text
AIDOCS_GITHUB_TOKEN=<fine-grained token or installation token>
AIDOCS_GITHUB_WEBHOOK_SECRET=<random shared secret>
```

`AIDOCS_GITHUB_TOKEN` is retained for legacy GitHub sources without a personal
connection. Use a fine-grained token with read-only Contents access, scoped
only to repositories aidocs may index. A source linked to a personal connection
never falls back to this global token. Configure the GitHub webhook URL as:

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

For manual **Sync now**, connect your Slack user under **Settings > Connections**
and create a Slack source whose `external_repository` is
`TEAM_ID/CHANNEL_ID/THREAD_TS`. The OAuth flow requests the user scopes
`channels:history` and `groups:history`; the thread must be visible to that
Slack user.

Event-driven webhooks remain available independently. Create a Slack source
with `provider=slack`, `trigger_mode=event`, and an `external_repository` in one
of these forms:

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

## Connect a Google Drive file

Connect Google Drive under **Settings > Connections**, then use **Choose from
Drive** while creating a source. Picker grants aidocs access to the selected
file under Google's narrow `drive.file` scope; pasting an arbitrary Drive URL
works only if that personal connection already has access to the file.

Checks read metadata, export Google Docs/Sheets/Slides to text where supported,
or download normal text files. There is no Drive webhook; use **Sync now** for
an immediate check or leave the source on a non-manual trigger for interval
polling. Deployment setup requires the Drive API, Picker API, OAuth web client,
browser API key, and numeric Google Cloud project number; see
[self-hosting](self-hosting.md#google-drive-picker).

## Connect DevRev tickets

Each user connects DevRev by pasting their PAT under **Settings > Connections**.
aidocs validates it, encrypts it with the deployment's connection-encryption
key, and never returns it through the API. Set a source's
`external_repository` to a work DON or display ID such as `TKT-42`; **Sync now**
reads that item with `works.get`.

For event-driven updates, create a DevRev source with `provider=devrev`,
`trigger_mode=event`, and set `external_repository` to the DevRev webhook ID.
This tracks every ticket event delivered by that webhook. Set it to one ticket's
DON or display ID instead to track only that ticket.

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

- Active linked sources whose trigger is not `manual` are polled on the
  configured interval; each source/window is deduplicated across replicas.
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
