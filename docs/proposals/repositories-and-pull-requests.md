# Repositories and pull requests

## Goal

Make aidocs feel like GitHub for documents: a person or configured organization
owns repositories, repositories contain independent documents and nested
folders, access can be granted by email, and protected document changes are
published only after review.

## Initial product model

- A **repository** is a stable boundary around an owned root folder. It can
  start empty, from a loose document, or from a root folder tree. An empty
  repository may bootstrap exactly one initial document directly; later
  protected changes must use pull requests.
- A repository has a personal or organization namespace, a unique slug within
  that namespace, a description, a required approval count, and protection
  enabled by default.
- Repository access is the ACL on its stable root folder. Existing folder ACL
  inheritance gives every descendant folder and document the repository grant;
  direct child grants may add access but cannot subtract inherited access.
- The configured organization deployment is the organization namespace in this
  version. Organization repositories grant the server-wide organization
  audience viewer access by default and are discoverable by all authenticated
  organization users; owners explicitly grant editor access.
- A **pull request** contains one or more immutable document-tree changes. A
  change may create, update, rename, move, or delete a document. Published
  document heads and paths do not move when a proposal is uploaded.
- A reviewer records either `approved` or `changes_requested` against the
  proposed head version. Authors cannot review their own pull requests.
- Merge locks the pull request and affected tree, rechecks every base, enforces
  repository approval and requested-changes state, then publishes the complete
  change set in one transaction.

## Protected and unprotected state

For a protected repository, direct document version uploads and content
metadata mutations are rejected. Repository tree mutations are represented as
reviewable change sets, so rename, move, create, and delete cannot bypass pull
requests.

The sole exception is initializing an empty repository with its first
document. The bootstrap endpoint becomes unavailable as soon as one document
exists, so it cannot bypass protection for later additions or updates.

The following collaboration/runtime state remains outside repository history:

- access grants;
- review comments and replies;
- document KV/runtime state;
- pins and workspace-open history.

Loose documents and folders retain their existing direct-edit behavior.

Repository deletion is an owner-only administrative operation. It permanently
removes the repository boundary and all contained documents, folders, versions,
pull requests, reviews, comments, and grants; it is not a reviewable content
change.

## Acceptance ledger

| Scenario                              | Expected result                                                    | Evidence                      |
| ------------------------------------- | ------------------------------------------------------------------ | ----------------------------- |
| Create a personal repository          | Unique personal namespace/slug and stable root                     | HTTP test                     |
| Create an organization repository     | Allowed only on an org deployment; org viewers inherit access      | HTTP test                     |
| Create an empty repository            | Owned root folder is created automatically                         | Domain + HTTP test            |
| Bootstrap its first document          | Allowed once for editors; a second bootstrap is rejected           | HTTP test                     |
| Seed from one doc or folder tree      | Owned loose content moves into the repository once                 | HTTP test                     |
| Invite by email                       | A placeholder grant survives until that email signs in             | HTTP test                     |
| Push directly to a protected doc      | `409 pull_request_required` for browser, CLI, and service accounts | Domain + HTTP test            |
| Open a pull request                   | Proposed head renders but published current version is unchanged   | HTTP test                     |
| Review a document change              | Diff shows readable added/removed text, not source markup          | Frontend build + bridge tests |
| Review your own change                | Rejected                                                           | HTTP test                     |
| Merge without approval                | Rejected with an actionable reason                                 | HTTP test                     |
| Request changes                       | Merge remains blocked until reviewer state changes                 | HTTP test                     |
| Merge an approved current-base change | Head advances atomically and PR becomes merged                     | HTTP test                     |
| Merge after another change won        | Rejected as stale                                                  | HTTP test                     |
| Change several documents and paths    | Validated first and merged atomically                              | Domain + PostgreSQL test      |
| Upload a later patchset               | Prior approvals no longer count                                    | Domain + HTTP test            |
| Rebase a stale proposal               | Bases refresh only when changes replay safely                      | Domain + HTTP test            |

## Deliberate next steps

1. Named branches and merge queues; successive patchsets and approval dismissal
   are available now.
2. First-class multi-organization membership and richer repository roles;
   repository teams are available now.
3. Pending-invitation status, resend/revoke flows, and email delivery.
4. Optional visual diffs and review threads scoped to a pull request.
5. Richer CODEOWNERS rule syntax; named reviewer/team and path-rule approvals
   are enforced now.
