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

Org installs can also pick the sharing default the server applies to every
created document (`viewer`, `commenter`, `editor`, or `private`, the
default):

```yaml
deployment:
  mode: org
  orgDomains: "acme.com"
  defaultDocPermission: "commenter" # new documents are org-commentable
```

Creators can override per document with the `default_permission` field on
create, and every applied default is reported back with its grant id so it
can be revoked. Non-private defaults require org mode: a public server
would share every new document with anyone holding the link, so the server
refuses to start rather than apply that silently.

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
