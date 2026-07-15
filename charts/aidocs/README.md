# aidocs Helm chart

Deploy `aidocs` to Kubernetes as one application Deployment and Service. You bring Postgres, S3-compatible blob storage, and an OIDC identity provider (Google, Okta, Entra ID, Keycloak, Dex, ...).

## Two origins, on purpose

aidocs serves uploaded document HTML — untrusted content — in a sandboxed frame on a dedicated render origin (`origins.render`). Keep it on a **different host** from the app origin (`origins.app`): if documents rendered on the app origin, an uploaded script could run same-origin with the authenticated application and its session cookies. The chart defaults reflect this (`aidocs.example.com` vs `render.aidocs.example.com`), and when the Traefik `ingressRoute` is enabled, template rendering fails unless `ingressRoute.renderHost` is set and differs from `ingressRoute.host`. Route both hosts to the same Service.

## Install

```bash
helm upgrade --install aidocs ./charts/aidocs \
  --namespace aidocs \
  --create-namespace \
  -f values.yaml
```

## Minimal values

```yaml
image:
  repository: razorpay/aidocs
  tag: "0.1.0"

# `public` (the chart default) accepts any account your OIDC provider allows.
deployment:
  mode: public

origins:
  app: https://aidocs.example.com
  render: https://render.aidocs.example.com

googleOAuth:
  clientID: "..."
  clientSecret: "..."

# Or any other OIDC provider instead of googleOAuth (setting oidc.issuerURL
# switches the chart over):
# oidc:
#   issuerURL: https://sso.example.com
#   providerName: Okta
#   clientID: aidocs
#   clientSecret: "..."

session:
  secret: "replace-with-at-least-32-random-bytes"

connectedKnowledge:
  pollInterval: 15m # 0 disables linked-source polling; minimum nonzero value 1m
  githubToken: "..." # optional, read-only Contents access
  githubWebhookSecret: "..." # optional; POST /v1/webhooks/github
  slackSigningSecret: "..." # optional; POST /v1/webhooks/slack
  devRevWebhookSecret: "..." # optional, 8-32 bytes; POST /v1/webhooks/devrev

database:
  host: postgres
  user: aidocs
  password: replace-me
  name: aidocs
  sslMode: require

blob:
  bucket: aidocs
  region: us-east-1
  accessKeyID: "..."
  secretAccessKey: "..."

# Bounded writable /tmp for multipart imports while the image stays read-only.
temporaryStorage:
  sizeLimit: 128Mi

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 5
  targetCPUUtilizationPercentage: 70
  targetMemoryUtilizationPercentage: 70

ingress:
  enabled: true
  className: nginx
  hosts:
    - host: aidocs.example.com
      paths:
        - path: /
          pathType: Prefix
    - host: render.aidocs.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: aidocs-tls
      hosts:
        - aidocs.example.com
        - render.aidocs.example.com
```

## Restricting sign-in to your organization

The chart defaults to `deployment.mode: public`, which accepts any account
the configured OIDC provider allows. To gate login to a company domain,
switch to org mode and set the domains explicitly:

```yaml
deployment:
  mode: org
  orgDomains: "acme.com" # comma-separated; required when mode=org
  orgName: "Acme" # share UI copy, e.g. "Anyone in Acme"
```

In public mode you can still apply a lighter email-domain allowlist with
`allowedOAuthDomains: "acme.com"` (empty by default, meaning no gate).

## Personal connected accounts

Personal provider connections are optional and separate from the OAuth/OIDC
provider used to sign in to aidocs. Each user connects their own account from
**Settings > Connections**; aidocs encrypts the resulting access and refresh
tokens before storing them in Postgres.

```yaml
personalConnections:
  encryption:
    # Base64 for exactly 32 random bytes: openssl rand -base64 32
    key: "..."
  github:
    clientID: "..."
    clientSecret: "..."
  slack:
    clientID: "..."
    clientSecret: "..."
  googleDrive:
    clientID: "..."
    clientSecret: "..."
  googlePicker:
    apiKey: "..."
    appID: "123456789012" # numeric Google Cloud project number
  devRev:
    # Optional trusted-provider-proxy/testing override; empty uses api.devrev.ai.
    apiBaseURL: ""
```

Keep the encryption key stable across all pods and restarts. Changing it makes
existing connected-account credentials unreadable until every user reconnects.
Setting only the encryption key is enough to enable per-user DevRev PATs;
DevRev does not use a deployment-wide OAuth client. Leave
`personalConnections.devRev.apiBaseURL` empty for the official DevRev API; the
override exists for a trusted provider proxy or contract testing.

Register these exact callbacks under the public `origins.app` host:

```text
https://aidocs.example.com/v1/connections/github/callback
https://aidocs.example.com/v1/connections/slack/callback
https://aidocs.example.com/v1/connections/google_drive/callback
```

The GitHub OAuth App flow currently requests only `repo`.
GitHub documents that `repo` grants broad read/write access to every public and
private repository available to that user; OAuth Apps cannot make private code
access read-only. aidocs uses the token only for reads, but do not enable this
flow if the token's broad capability is unacceptable. See
[GitHub's OAuth scope reference](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/scopes-for-oauth-apps).

For Slack, create an OAuth v2 app with the user token scopes
`channels:history` and `groups:history`. Each aidocs user authorizes their own
Slack identity. See [Slack OAuth v2](https://api.slack.com/authentication/oauth-v2).

For Google Drive, enable both the Google Drive API and Google Picker API, add
`origins.app` as an authorized JavaScript origin, restrict the browser API key
to that origin and Picker API, and use the Cloud project number as `appID`.
aidocs requests the narrow `drive.file` scope, so users explicitly select the
files the app may read. See Google's
[Picker setup](https://developers.google.com/workspace/drive/picker/guides/web-picker)
and [`drive.file` guidance](https://developers.google.com/workspace/drive/api/guides/api-specific-auth).

Connected accounts enable the repository's **Sync now** action and background
reconciliation. `connectedKnowledge.pollInterval` defaults to `15m`; set it to
`0` to disable polling or another Go duration of at least `1m`. Polling covers
active linked sources whose trigger is not `manual`. A persisted per-window
delivery key deduplicates discovery across replicas, so each source queues at
most once per interval. Provider limits still apply—especially when many Slack
threads share one user token—so increase the interval or use manual triggers as
needed.

Signed GitHub, Slack, and DevRev webhooks under `connectedKnowledge` remain
available independently for event-driven updates, and `AIDOCS_GITHUB_TOKEN`
remains a legacy fallback only for unlinked GitHub sources.

## One-time structured-mentions upgrade

When upgrading an installation from an image that stored comment mentions as
U+2063 markers, do not use the normal rolling strategy. Pause comment writes
and perform this release with no old/new pod overlap:

```bash
helm upgrade aidocs ./charts/aidocs \
  --namespace aidocs \
  -f values.yaml \
  --set updateStrategy.type=Recreate
```

After every pod runs the new image and browser caches/tabs have been refreshed,
restore normal rolling updates (this changes only the Deployment strategy):

```bash
helm upgrade aidocs ./charts/aidocs \
  --namespace aidocs \
  --reuse-values \
  --set updateStrategy.type=RollingUpdate
```

See the coordinated cutover and irreversible rollback details in
[`docs/self-hosting.md`](../../docs/self-hosting.md#upgrading).

## Realtime KV events

Document KV streams default to `AIDOCS_KV_EVENTS_MODE=auto`, which uses
coalesced polling unless a dedicated LISTEN connection URL is provided.
The `kvEvents` block wires the two server env vars; both are rendered only
when set:

```yaml
kvEvents:
  mode: notify # auto | notify | poll (empty = server default, auto)
  # Direct or session-pooled URL to the same database for the dedicated
  # LISTEN connection. Never a transaction-pooling endpoint. Stored in the
  # chart-managed Secret.
  listenerDatabaseURL: "postgres://aidocs:pass@postgres:5432/aidocs?sslmode=require"
```

Or keep the URL out of Helm values with a pre-created Secret:

```yaml
kvEvents:
  mode: notify
  existingSecret: aidocs-kv-events
  listenerDatabaseURLKey: AIDOCS_KV_EVENTS_LISTENER_DATABASE_URL
```

If your only database route is PgBouncer in transaction-pooling mode, set
`kvEvents.mode: poll` and leave the listener URL unset.

## Existing secrets

You can keep secrets outside Helm values:

```yaml
googleOAuth:
  existingSecret: aidocs-google-oauth
  clientIDKey: GOOGLE_OAUTH_CLIENT_ID
  clientSecretKey: GOOGLE_OAUTH_CLIENT_SECRET

session:
  existingSecret: aidocs-session
  secretKey: SESSION_SECRET

personalConnections:
  encryption:
    existingSecret: aidocs-connection-encryption
    secretKey: AIDOCS_CONNECTION_ENCRYPTION_KEY
  github:
    existingSecret: aidocs-github-oauth
    clientIDKey: AIDOCS_GITHUB_OAUTH_CLIENT_ID
    clientSecretKey: AIDOCS_GITHUB_OAUTH_CLIENT_SECRET
  slack:
    existingSecret: aidocs-slack-oauth
    clientIDKey: AIDOCS_SLACK_OAUTH_CLIENT_ID
    clientSecretKey: AIDOCS_SLACK_OAUTH_CLIENT_SECRET
  googleDrive:
    existingSecret: aidocs-google-drive-oauth
    clientIDKey: AIDOCS_GOOGLE_DRIVE_OAUTH_CLIENT_ID
    clientSecretKey: AIDOCS_GOOGLE_DRIVE_OAUTH_CLIENT_SECRET
  googlePicker:
    existingSecret: aidocs-google-picker
    apiKeyKey: AIDOCS_GOOGLE_PICKER_API_KEY
    appIDKey: AIDOCS_GOOGLE_PICKER_APP_ID

database:
  existingSecret: aidocs-database
  hostKey: DATABASE_HOST
  userKey: DATABASE_USER
  passwordKey: DATABASE_PASSWORD
  nameKey: DATABASE_NAME

blob:
  existingSecret: aidocs-blob
  accessKeyIDKey: access-key-id
  secretAccessKeyKey: secret-access-key

connectedKnowledge:
  existingSecret: aidocs-connected-knowledge
  githubTokenKey: AIDOCS_GITHUB_TOKEN
  githubWebhookSecretKey: AIDOCS_GITHUB_WEBHOOK_SECRET
  slackExistingSecret: aidocs-slack
  slackSigningSecretKey: AIDOCS_SLACK_SIGNING_SECRET
  devRevExistingSecret: aidocs-devrev
  devRevWebhookSecretKey: AIDOCS_DEVREV_WEBHOOK_SECRET
```

## Redirect URIs

Configure this redirect URI in Google Cloud:

```text
https://aidocs.example.com/v1/auth/google/callback
```

or, for a generic OIDC provider:

```text
https://aidocs.example.com/v1/auth/oidc/callback
```

These sign-in callbacks are separate from the personal connected-account
callbacks. Register whichever provider connections you enable:

```text
https://aidocs.example.com/v1/connections/github/callback
https://aidocs.example.com/v1/connections/slack/callback
https://aidocs.example.com/v1/connections/google_drive/callback
```
