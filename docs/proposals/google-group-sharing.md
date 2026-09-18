# Google Group sharing for aidocs

Status: Proposed

## Summary

Extend the existing aidocs ACL model with a `google_group` principal so that
documents and folders can be shared with verified, organization-owned Google
Groups.

The first release will:

- Support document and folder grants, including inherited folder access.
- Support direct and nested Google Group membership.
- Refresh memberships during authenticated use with a one-minute Postgres
  cache.
- Ignore stale membership data when Google is unavailable.
- Return `503 dependency_unavailable` for group-dependent access and aggregate
  workspace or search requests when membership refresh fails.
- Enable group sharing only for `org` deployments and groups owned by the
  configured Google Workspace customer.
- Provide group autocomplete in the web UI and explicit `--group` CLI syntax.
- Allow owner-level group grants while retaining the creator as the canonical
  owner.
- Allow only the canonical owner to create, promote, demote, or revoke an
  owner-level group grant.
- Avoid sending sharing or notification emails as part of this capability.
- Target deployments with up to 1,000 daily active users.

## Google integration

Introduce a `GoogleGroupDirectory` abstraction with the following operations:

```go
type GoogleGroupDirectory interface {
	SearchGroups(ctx context.Context, query string, limit int) ([]GoogleGroup, error)
	LookupGroup(ctx context.Context, email string) (GoogleGroup, error)
	TransitiveGroupsForMember(ctx context.Context, userEmail string) ([]GoogleGroup, error)
}
```

The production implementation will use the Google Cloud Identity API:

- `groups.search` for autocomplete, restricted to the configured Workspace
  customer.
- `groups.lookup` and `groups.get` to verify exact group addresses.
- `groups.memberships.searchTransitiveGroups` for direct and nested
  memberships.
- `https://www.googleapis.com/auth/cloud-identity.groups.readonly` as the only
  required OAuth scope.

Persist the immutable `groups/{id}` resource name as the ACL principal ID.
Group email and display name are metadata because a group address can change.
All requests must be restricted to `parent == customers/{customerID}` and must
accept only Google-managed discussion or security groups.

The transitive-membership API requires Google Workspace Enterprise Standard,
Enterprise Plus, Enterprise for Education, or Cloud Identity Premium. See the
[Google transitive-membership documentation](https://docs.cloud.google.com/identity/docs/reference/rest/v1/groups.memberships/searchTransitiveGroups).

## Persistence

Add the following tables through a Goose migration.

### `google_groups`

| Column | Purpose |
| --- | --- |
| `id` | Immutable `groups/{id}` resource name and primary key. |
| `customer_id` | Owning Google Workspace customer. |
| `email` | Canonical group email. |
| `display_name` | Current Google display name. |
| `last_verified_at` | Last successful Google verification. |
| `created_at` / `updated_at` | Local lifecycle timestamps. |

Add a case-insensitive unique index on canonical email.

### `google_group_memberships`

| Column | Purpose |
| --- | --- |
| `user_id` | aidocs user receiving derived group access. |
| `group_id` | Referenced immutable Google Group ID. |
| `synced_at` | Time this membership was observed. |

Use `(user_id, group_id)` as the primary key and cascading foreign keys to the
user and group records.

### `google_group_sync_state`

| Column | Purpose |
| --- | --- |
| `user_id` | aidocs user whose membership was refreshed. |
| `last_attempt_at` | Most recent refresh attempt. |
| `last_success_at` | Most recent complete, successful snapshot. |
| `last_error_code` | Low-cardinality failure category for diagnostics. |

A successful refresh transaction must:

1. Upsert the group metadata returned by Google.
2. Atomically replace the user's complete membership snapshot.
3. Update `last_success_at` only after the snapshot is committed.

A failed refresh must preserve the old rows for diagnostics, but rows older
than the one-minute freshness window must not be used for authorization.

## Authentication and authorization

Add `PrincipalGoogleGroup = "google_group"`. Human principals will also carry
internal, non-serialized fields for resolved group IDs and membership-resolution
status. These fields must never appear in API responses or logs.

After normal cookie or bearer authentication:

1. Skip group resolution for aidocs service accounts.
2. Read the user's shared membership snapshot from Postgres.
3. Attach the cached group IDs when `last_success_at` is less than one minute
   old.
4. Otherwise, synchronously refresh memberships using
   `searchTransitiveGroups`.
5. Coalesce concurrent refreshes for the same user within each server process.
6. Apply a short retry cooldown after Google failures to avoid request storms.
7. Never attach or authorize against stale group IDs.

Update every authorization and discovery path that currently matches only the
authenticated principal:

- Direct document and folder role checks.
- Folder-ancestor permission inheritance.
- Workspace document and folder listings.
- Document and folder title search.
- Folder content and document listings.
- Effective-access reporting.

The effective role remains the highest role derived from canonical ownership,
direct user or service-account grants, Google Group grants, inherited folder
grants, and the `anyone` principal, with one exception: an owner-level grant
inherited from a folder owned by somebody else is capped at `editor` (see
[Ownership semantics](#ownership-semantics)).

### Failure behavior

- Exact-resource access continues when canonical ownership, a direct user or
  service-account grant, or `anyone` independently satisfies the required role.
- If access could depend on unresolved group membership, deny access and return
  HTTP `503` with error code `group_membership_unavailable`.
- Workspace, folder-listing, and search endpoints return `503` instead of
  silently returning incomplete results.
- Group search and group-grant creation return `503` while Google is
  unavailable.
- Successfully refreshed membership removal produces the ordinary `403`
  response when the user has no remaining access.

## Ownership semantics

Google Groups may receive `viewer`, `commenter`, `editor`, or `owner` roles.

An `owner` Google Group grant provides owner-level co-access. It does not
replace the canonical owner stored on the document or folder. Group-derived
co-owners may perform normal owner operations, including deletion and managing
non-owner grants.

Owner-level access only ever follows the canonical owner's own resources. An
owner-level grant on a folder confers `owner` on the resources below it whose
canonical owner is that folder's canonical owner, and at most `editor` on
anything below it that belongs to somebody else — so moving another person's
document into the folder, or re-parenting a folder around it, never promotes the
folder's sharers over that person.

Owner-level grant invariants:

- Only the canonical owner may create an owner-level group grant.
- Only the canonical owner may promote a group to owner.
- Only the canonical owner may demote or revoke an existing owner-level group
  grant.
- Group-derived co-owners cannot alter owner-level grants.
- An owner-level grant inherited from an ancestor folder confers `owner` only
  when that folder's canonical owner is also the canonical owner of the
  resource below; otherwise it confers at most `editor`, everywhere an
  effective role is computed or reported.
- Owner-level grants remain unsupported for individual users and aidocs
  service accounts.
- The canonical owner continues to appear separately in effective-access and
  UI representations.

These rules need explicit comments near the authorization checks because they
are security invariants rather than ordinary role validation.

## Public API

Add an authenticated search endpoint:

```http
GET /v1/google-groups?q=<query>
```

Response:

```json
{
  "items": [
    {
      "id": "groups/012345",
      "email": "tech-infra-common-platforms@razorpay.com",
      "name": "Common Platform"
    }
  ]
}
```

The endpoint must:

- Require at least two query characters.
- Return at most 20 entries.
- Search canonical email and display name.
- Return only groups belonging to the configured Workspace customer.
- Return no membership details or member counts.

Extend the existing document and folder grant request:

```json
{
  "principal": {
    "type": "google_group",
    "email": "tech-infra-common-platforms@razorpay.com"
  },
  "role": "commenter"
}
```

The server must resolve and verify the group rather than trusting a
caller-supplied ID. Grant responses and effective-access results will include
the immutable ID, canonical email, and display name.

The API change is additive. Existing user, service-account, and `anyone`
payloads remain compatible.

## CLI

Add mutually exclusive group syntax for documents and folders:

```bash
aidocs docs grants add <doc_id> \
  --group tech-infra-common-platforms@razorpay.com \
  --role commenter

aidocs folders grants add <folder_id> \
  --group tech-infra-common-platforms@razorpay.com \
  --role viewer
```

`--group` must be mutually exclusive with `--to`, `--everyone`, and the legacy
`--principal` flag. Update command help, examples, compact output, JSON output,
and usage-error handling.

## Web UI

Update the Share sheet with explicit target modes:

- People and bots.
- Google Groups.

Google Group mode will:

- Debounce autocomplete through `/v1/google-groups`.
- Require selection of a server-verified result.
- Display a group icon, display name, and canonical email.
- Include `owner` in the role selector only when the current user is the
  canonical resource owner.
- Clearly distinguish the canonical owner from group co-owners.
- Display direct and inherited group access sources.
- Preserve confirmation flows when changing or revoking inherited access.
- Surface `group_membership_unavailable` as a temporary dependency problem,
  not as a permanent permission denial.

Expose `google_groups_enabled` from `/v1/config`. Hide group search and sharing
controls when the capability is disabled.

## Deployment configuration

Add the following server settings:

| Variable | Default | Purpose |
| --- | --- | --- |
| `AIDOCS_GOOGLE_GROUPS_ENABLED` | `false` | Enables the capability. |
| `GOOGLE_GROUPS_CUSTOMER_ID` | none | Workspace customer ID beginning with `C`. |
| `GOOGLE_GROUPS_ADMIN_EMAIL` | none | Delegated administrator subject. |
| `GOOGLE_GROUPS_CREDENTIALS_FILE` | none | Mounted service-account JSON file. |
| `GOOGLE_GROUPS_MEMBERSHIP_TTL` | `1m` | Maximum authorization-cache age. |
| `GOOGLE_GROUPS_REQUEST_TIMEOUT` | `5s` | Deadline for Google API calls. |

When enabled, startup must:

- Require `AIDOCS_DEPLOYMENT=org`.
- Validate all group settings.
- Validate that the credential file is present and readable.
- Fail startup for malformed or incomplete configuration.
- Avoid making live Google availability a Kubernetes readiness dependency.

Extend Helm values to mount a dedicated service-account JSON key from an
existing Secret. Document domain-wide delegation, the read-only scope, customer
ID discovery, credential rotation, and staging verification. Credentials and
tokens must not be written to logs or returned through configuration APIs.

## Tests

### Unit tests

- Cloud Identity request construction, CEL escaping, customer filtering, and
  pagination.
- Search, lookup, and transitive-membership response mapping.
- Fresh, stale, empty, and failed membership snapshots using an injected clock.
- Atomic membership replacement and preservation after failed refresh.
- One-minute expiry and retry cooldown.
- Concurrent refresh coalescing.
- External-customer, malformed, and deleted group rejection.
- Group role merging and highest-role precedence.
- Owner-group creation, promotion, demotion, and revocation authorization.
- Service accounts never receiving Google-derived memberships.
- Credentials, tokens, user emails, and group membership lists not leaking into
  errors, logs, or metrics.

### Repository and Postgres integration tests

Add a Postgres service to CI and cover:

- Migration up/down behavior and schema constraints.
- Group grant persistence and metadata joins.
- Direct and nested group access to documents.
- Group access inherited from parent folders.
- Workspace, search, and folder-listing visibility.
- Membership additions and removals after refresh.
- Empty membership snapshots removing previous access.
- Group renames preserving authorization through the immutable ID.
- Effective-access output for direct and inherited group sources.
- Owner-group permissions without canonical-owner replacement.
- Existing user, service-account, and `anyone` behavior remaining unchanged.

### API and CLI tests

- Group autocomplete success, pagination, disabled feature, invalid query, and
  Google outage.
- Document and folder group-grant create, update, and delete.
- Unknown, external, and spoofed group rejection.
- `503` for group-dependent access with stale or unavailable membership.
- Direct exact-resource access remaining usable during Google failure.
- Aggregate workspace and search endpoints failing instead of returning partial
  data.
- Canonical-owner-only owner-grant operations.
- CLI `--group` payload, human output, JSON output, and mutual-exclusion errors.
- Backward compatibility for existing API and CLI callers.

### Frontend component tests

Introduce Vitest and React Testing Library for:

- People/group mode switching.
- Debounced group search and verified selection.
- Empty, loading, and dependency-error states.
- Group roles and conditional owner visibility.
- Direct versus inherited group display.
- Owner-grant confirmation and authorization errors.

### Browser tests

Introduce Playwright with a fake Google directory and verify:

- A canonical owner shares a document and folder with a group.
- Direct and nested members discover and open the resource.
- Removed members lose access after cache expiry.
- Folder inheritance works.
- Google failure produces the expected temporary-error UI without exposing
  stale content.
- Existing user and service-account sharing still works.

CI must remain hermetic and must not use live Google credentials. Run a gated
staging smoke test with a dedicated real Workspace group for search, direct
membership, nested membership, addition, removal, and the one-minute
revocation target.

## Observability

Add low-cardinality metrics for:

- Google requests by operation and outcome.
- Google request latency.
- Membership-cache hits, misses, and stale snapshots.
- Refresh success and failure.
- Group-dependent authorization failures.
- Search and grant-resolution failures.

Do not label metrics with user emails, group emails, or group IDs. Alert on
sustained Google `401` or `403` responses, quota failures, membership-refresh
failure rate, and elevated `group_membership_unavailable` responses.

## Rollout

Although the capability will launch to all organization users, use two
technical deployments:

1. Deploy migrations and group-aware backend/UI code with the feature disabled.
2. Verify all existing ACL behavior and complete the staging live tests.
3. Ensure the first rollout has completed across every pod.
4. Mount Google credentials and enable the capability in a second rollout.
5. Monitor dependency errors, quota use, cache performance, and authorization
   denials.

Do not create group grants while old and new server versions are mixed. Roll
back by disabling the feature; retain group and grant rows so re-enabling is
non-destructive.

## Acceptance criteria

- A canonical owner can share documents and folders with a verified,
  organization-owned Google Group.
- Direct and nested group members receive the assigned role and inherited
  folder access.
- Membership additions and removals take effect within one minute during
  authenticated use.
- Stale membership is never used for authorization.
- Google failures produce explicit temporary errors without breaking
  independently sufficient exact-resource access.
- Group owner grants provide owner-level co-access without changing canonical
  ownership.
- Only the canonical owner can manage owner-level group grants.
- Existing sharing behavior remains backward compatible.
- No email is sent when sharing with a group.

## Commenting and documentation expectations

Implementation comments should explain only non-obvious behavior and security
invariants, particularly:

- Why ACLs store immutable Google Group IDs instead of email addresses.
- Why stale memberships are retained for diagnostics but ignored for access.
- Why group membership fields on a principal are internal-only.
- Why group owner access does not replace canonical ownership.
- Why only canonical owners may alter owner-level group grants.
- Why an inherited owner-level grant is capped at `editor` for resources owned
  by somebody else.

Update the self-hosting guide, Helm chart documentation, CLI help, generated
OpenAPI specification, and operator runbooks alongside the implementation.
