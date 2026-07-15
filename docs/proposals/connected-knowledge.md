# Connected knowledge repositories

## Goal

An aidocs repository can treat one or more external systems as sources of
context. A source change is never published directly: aidocs evaluates the
change, records provenance, and proposes any meaningful documentation update
through the repository's normal pull-request and approval rules.

GitHub provides remote comparison, while Slack thread and DevRev ticket events
use signed, replay-protected webhooks. Google Drive, Confluence, Notion, and
manual sources share the same connection and sync-run model without changing
the review workflow.

## Product model

- A **source** belongs to one aidocs repository and records its provider,
  external identity, tracked branch or revision, include/exclude paths, target
  document or folder, trigger mode, direction, and freshness state.
- A repository can have many sources. Several sources may contribute to one
  knowledge base, but updates to the same document are serialized.
- A **sync run** is an immutable attempt to evaluate one external revision. It
  records the delivery id, before/after revisions, status, source links,
  affected paths, summary, error, and resulting aidocs pull request.
- A **proposal** is created only when rendered document text changes. No-op
  evaluations remain visible in activity so users can distinguish a healthy
  integration from one that has stopped receiving events.
- Source-generated pull requests are authored by an integration principal and
  obey the same review rules as human changes. Human approval remains the
  default; external sources never publish documents silently.
- Every generated change carries provenance pinned to an immutable external
  revision. Published documents expose their freshness relative to each source.

## Repository experience

Repositories expose these product areas:

1. **Documents** and **Pull requests** for published content and review.
2. **Sources** for provider connections, path rules, checks, exports, and sync
   history.
3. **Activity** for audit events, source runs, review actions, and settings
   changes.
4. **Insights** for repository search, cited questions, backlinks, and the
   document dependency graph.
5. **Settings** for review rules, teams, portal visibility, freshness policy,
   archive/restore, transfer, and deletion.

Global pages provide a review inbox, notifications, and a document-template
gallery.

## GitHub processing

GitHub is connected with a server-side read-only token. A GitHub App
installation token is recommended for production; a fine-grained token is also
supported. The credential must be scoped to only the repositories enabled in
aidocs. The webhook endpoint:

1. reads the unmodified request body;
2. verifies `X-Hub-Signature-256` with the configured secret;
3. deduplicates `X-GitHub-Delivery` durably;
4. records a pending sync run and acknowledges the delivery quickly;
5. processes the run asynchronously;
6. compares the recorded before/after revisions and applies path filters;
7. creates a no-op result or a reviewable aidocs proposal.

Explicit source checks compare the source's recorded revisions, covering manual
testing and missed deliveries. The durable queue recovers interrupted work.
Scheduled provider reconciliation and installation-token minting remain
provider-operation follow-ups; failures and rate limits already produce
explicit sync states rather than silently advancing freshness.

## Pull-request requirements

A connected knowledge base can span several documents. Pull requests therefore
support a list of changes instead of only one HTML version. A change can create,
update, rename, move, or delete a document. Merge validates and publishes the
whole set atomically.

Open pull requests can receive successive patchsets. Reviews bind to the current
patchset and are dismissed when its content changes. Rebase refreshes base
versions only when the proposed operations can be replayed without losing
published or human-authored content.

## Trust and safety

- The access level of an aidocs repository must not be broader than a connected
  private source without an explicit owner acknowledgement.
- Source files are untrusted data. Repository instructions cannot grant tools,
  change provider credentials, bypass path filters, or alter review policy.
- Generated text includes source citations and never claims freshness beyond
  the last successful revision.
- Secrets, binaries, generated files, vendor directories, and lock files are
  excluded by default and can be overridden only by repository owners.
- Provider credentials and webhook payloads use bounded retention and must not
  appear in audit summaries, generated documents, or application logs.

## Acceptance ledger

| Scenario                                   | Expected result                                                                                 |
| ------------------------------------------ | ----------------------------------------------------------------------------------------------- |
| Connect several GitHub repositories        | Every source is independently configurable and visible in one aidocs repository                 |
| Receive the same delivery twice            | One sync run is processed; the duplicate is acknowledged idempotently                           |
| Merge unrelated code                       | Sync run is recorded as no documentation change; no pull request is opened                      |
| Merge documentation-relevant code          | A source-authored pull request contains readable text changes and immutable provenance          |
| Several documents are affected             | One change set proposes every operation and merges atomically                                   |
| A later merge arrives while review is open | The existing proposal receives a new patchset or the event is queued; approvals are dismissed   |
| A source update races with a human edit    | Rebase preserves the human version or reports a conflict; it never overwrites silently          |
| Ask a repository question                  | The answer includes accessible document/version citations and does not use hidden repositories  |
| Enable a public portal                     | Only explicitly published repository content is accessible without authentication               |
| Export to a Git provider                   | aidocs produces a reviewable provider proposal; it does not push directly to the default branch |
| Archive a repository                       | Content becomes read-only and can be restored by an owner                                       |
| Transfer a repository                      | Ownership changes only after the target accepts                                                 |

## Implementation status

Implemented: multi-document change sets, patchsets and rebase; settings, teams,
notifications, templates and audit; provider-neutral sources and sync runs;
signed GitHub webhooks and remote comparison; search, cited questions,
backlinks, graph, freshness, portals; and reviewable export proposals.

Remaining provider depth: scheduled reconciliation, GitHub App installation
flows, richer source-to-multi-document generation, additional remote provider
clients, and policy-controlled automatic merging. These extend the same source
and pull-request contracts without bypassing review.
