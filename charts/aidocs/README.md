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
