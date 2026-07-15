# Production verification

`make verify-production` runs aidocs through its real production entrypoint and
infrastructure adapters. It is intentionally broader than a smoke test: the
script builds the embedded frontend and `aidocs-server`, provisions fresh
PostgreSQL and S3-compatible MinIO instances, applies every migration, and then
exercises state-changing workflows over HTTP.

The default gate builds the production command directly on the host for a fast,
repeatable contributor workflow. `make verify-production-image` runs the same
gate and additionally builds and boots the exact Dockerfile artifact from its
public Node, Go, and Alpine base images.

## Run it

From the repository root:

```bash
make verify-production
```

To additionally verify the release container:

```bash
make verify-production-image
```

The image target needs the Docker Buildx plugin plus ordinary network access to
the Dockerfile's public base images, or those images already present in the
local Docker cache. It is a hard gate when selected: a missing Buildx plugin,
an inaccessible base image, failed build, or failed restricted-container
assertion fails the run.

Required local tools are Docker, Go, Node/npm, Helm, `curl`, `jq`, `psql`,
OpenSSL, `shasum`, `od`, and Python 3. The script fails before changing anything
if a dependency is missing or Docker is unavailable. Temporary containers,
network, database, objects, binary, and logs are removed on exit.

The PostgreSQL, MinIO, MinIO client, and local Dex identity-provider images are
pinned by digest so a passing run is reproducible. The verifier uses Dex to
complete a real local OIDC authorization-code login; it never depends on a
Google or corporate identity endpoint. Override an image only when deliberately
testing a different runtime:

```bash
AIDOCS_VERIFY_POSTGRES_IMAGE=postgres:16 \
AIDOCS_VERIFY_MINIO_IMAGE=minio/minio:RELEASE_TAG \
AIDOCS_VERIFY_MC_IMAGE=minio/mc:RELEASE_TAG \
AIDOCS_VERIFY_DEX_IMAGE=dexidp/dex:v2.41.1 \
make verify-production
```

Set `AIDOCS_VERIFY_KEEP=1` to preserve the isolated containers and artifacts
after a failure for inspection. The script prints their names and temp path.

## What a passing run proves

- The production React build is embedded in the production Go server binary.
- The Helm chart lints, renders, and wires OIDC, database, blob, personal
  connection OAuth/encryption/Picker, source polling, and legacy webhook
  configuration into the Deployment.
- A fresh database reaches migration version 25 or newer.
- Readiness verifies both PostgreSQL and blob storage; it fails closed when
  MinIO is paused and recovers when storage returns.
- A local Dex provider completes OIDC discovery, browser authorization-code
  login, callback, and session authentication without an external dependency.
- Seeded bearer authentication succeeds and anonymous repository access is
  rejected, keeping owner/reviewer approval scenarios deterministic.
- A nested local folder and a loose local HTML document import through the same
  multipart contract used by the browser UI.
- A conflicting multi-file import rolls back every repository, folder,
  document, and version row and leaves the MinIO object count unchanged.
- Imported hierarchy and immutable HTML content survive in PostgreSQL and S3.
- Organization repositories are discoverable by another organization member.
- A protected document change cannot be self-approved or merged early, but can
  be independently approved and merged into a new immutable version.
- Correctly signed Slack and DevRev events are accepted, durably queued, and
  converted into reviewable aidocs pull requests containing the external text.
- Duplicate deliveries are idempotent and stale signed deliveries are rejected.
- A current-shape fake DevRev API validates a per-user PAT with
  `POST /auth-tokens.info` and supplies a ticket through `POST /works.get`.
- The DevRev PAT is absent from API responses, generated documents, pull
  requests, and logs. PostgreSQL stores ciphertext that does not contain the
  plaintext token.
- A linked personal DevRev source creates a text-only documentation pull
  request through **Sync now**. After a full server restart, the account remains
  connected, its ciphertext is unchanged, and the same encrypted PAT produces
  a deterministic no-op sync.
- Repositories, document heads, legacy and personal source connections, sync
  state, and S3 content remain correct after that restart.
- No source run remains stuck in `queued` or `processing` state.

With `make verify-production-image`, a passing run additionally proves:

- The exact Dockerfile artifact starts and serves its embedded frontend while
  running as UID/GID `65532`, with all Linux capabilities dropped, privilege
  escalation disabled, and the root filesystem read-only.
- The image starts with the production connection-encryption and polling
  settings and lists the same persisted personal DevRev account without
  exposing its PAT. Provider validation and post-restart decryption/sync are
  exercised by the host-built production command because the disposable fake
  provider listens on the host loopback interface.
- The container has only the bounded writable `/tmp` used by the Helm chart.
- A two-file multipart import above Go's 8 MiB in-memory parsing threshold
  succeeds, proving production tempfile spill works under those restrictions;
  the temporary repository is then deleted and the S3 object count returns to
  its baseline.

This verifies production behavior locally against disposable dependencies. It
does not replace a post-deploy canary in your real cluster: before promoting a
release, run the same binary or image in staging with the production ingress,
identity provider, secret manager, managed database, and object store, then
repeat the critical import, review, webhook, restart, and readiness assertions.
