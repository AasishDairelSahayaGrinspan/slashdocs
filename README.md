# aidocs

Give your agents the power to publish beautiful, reviewable documents.

`aidocs` is a Google-Docs-like review layer for AI-generated HTML artifacts. Agents create rich, self-contained HTML reports, specs, dashboards, diagrams, and decision docs; humans review them in the browser with comments, versions, and sharing.

Instead of pasting giant reports into chat, let agents push a document, iterate on feedback, and keep a durable review trail.

## What you can build with it

- Agent-generated architecture reviews
- Incident reports and postmortems
- Product specs and launch plans
- Data analysis reports
- Design proposals with Mermaid/SVG diagrams
- QA findings and browser-test reports
- Internal dashboards captured as portable HTML

## Features

- One self-contained HTML file per document
- Browser review UI with sandboxed rendering
- Anchored comments and resolve/reopen workflow
- Immutable versions with optimistic concurrency
- Google OAuth for human users
- Native service accounts for headless/agent workflows
- Go CLI with compact default output and JSON mode
- Generated OpenAPI/Swagger docs
- Self-hostable Go server with embedded React frontend
- Dockerfile for container deployments

## Why one HTML file?

A single HTML artifact is easy for agents to generate, easy to version, easy to archive, and easy to move through a CLI. Images can be embedded with base64 data URLs, and diagrams can be Mermaid, inline SVG, or rendered HTML/CSS.

```html
<img src="data:image/png;base64,..." alt="embedded screenshot" />
```

No separate image hosting is required for v1.

## Install from source

```bash
git clone https://github.com/razorpay/aidocs.git
cd aidocs
make build
```

Binaries are written to:

```text
bin/aidocs-server
bin/aidocs
```

## Run with Docker

Release images are published to Razorpay Harbor:

```bash
docker pull harbor.razorpay.com/razorpay/aidocs:latest
```

Copy [`.env.example`](.env.example) to `.env`, fill it in, and run (pin an exact
tag or digest in production rather than `:latest`):

```bash
docker run --rm -p 8080:8080 --env-file .env harbor.razorpay.com/razorpay/aidocs:latest
```

See [Self-hosting](docs/self-hosting.md) for production setup and all environment variables. A Kubernetes Helm chart is available in [`charts/aidocs`](charts/aidocs).

## Local development

The fastest way to run the full stack locally — no internal registry access needed.

**Prerequisites:** Docker and Docker Compose.

### 0. Configure credentials

`docker-compose.yml` reads Google OAuth credentials and a session secret from environment variables. Create a `.env` file in the repo root:

```bash
cp .env.example .env
```

Fill in the required values:

```bash
# .env
GOOGLE_OAUTH_CLIENT_ID=<your-client-id>
GOOGLE_OAUTH_CLIENT_SECRET=<your-client-secret>
SESSION_SECRET=$(openssl rand -hex 32)
```

To create a Google OAuth client: go to [Google Cloud Console → Credentials](https://console.cloud.google.com/apis/credentials), create an OAuth 2.0 Client ID (Web application), and set the authorized redirect URI to `http://localhost:8080/v1/auth/google/callback`.

Docker Compose will fail to start if any of these are missing or empty.

### 1. Start dependencies + server

```bash
docker compose up --build
```

This starts:
- **PostgreSQL 16** on `localhost:5432`
- **MinIO** (S3-compatible blob store) on `localhost:9000` (API) and `localhost:9001` (console UI)
- **aidocs server** on `localhost:8080` (built from source via `Dockerfile.local`)

Database migrations run automatically on first boot (`AIDOCS_MIGRATE=true`).

### 2. Open the app

```
http://localhost:8080
```

Sign in with Google. The app is fully functional — you can create documents, invite others, and use the CLI against it.

### 3. Develop frontend with hot reload

Keep the compose stack running (for the API + DB), then in a separate terminal:

```bash
cd frontend
npm install
npm run dev
```

Vite's dev server starts on `http://localhost:5173` and proxies API calls to the Go server at `http://localhost:8080`.

To rebuild the frontend and apply changes to the running Docker container:

```bash
make frontend-docker
```

This runs `make frontend` (compiles the React bundle) then `docker-compose up -d` to restart the container with the new assets baked in. Use this after any frontend change when running via Docker Compose.

### 4. Develop the backend

```bash
# Run the server directly (needs DATABASE_URL and other env vars)
go run ./api/cmd/aidocs-server

# Or rebuild and restart via Docker
docker compose up --build aidocs
```

### 5. MinIO console

Browse uploaded HTML blobs at `http://localhost:9001`.

```
Username: minioadmin
Password: minioadmin
```

### Tear down

```bash
docker compose down          # stop containers, keep volumes
docker compose down -v       # stop containers and delete all data
```

---

## CLI quickstart

```bash
aidocs auth login localhost:8080
aidocs auth whoami

aidocs docs create report.html --title "Agent report"
aidocs docs list
aidocs docs pull doc_123 --out report.html
aidocs docs push doc_123 report.html --summary "Update findings"

aidocs docs comments list doc_123
aidocs docs comments create doc_123 --quote "important text" --body "Please clarify this."
aidocs docs comments resolve doc_123 cmt_123
```

Use `--json` for structured output:

```bash
aidocs --json docs list
```

## Development

```bash
make build
make swagger
go test ./...
npm run build --prefix frontend
```

Useful targets:

```text
make build          build server and CLI
make build-backend  build API server
make build-cli      build CLI
make build-frontend build embedded frontend assets
make swagger        regenerate OpenAPI docs
```

## Project layout

```text
api/       Go API server and embedded frontend assets
cli/       Go CLI
frontend/  React + Vite frontend
docs/      Self-hosting and operator docs
```

## API docs

When the server is running:

```text
/openapi.json
/api-docs
/commit.txt
```

## Security model

Uploaded HTML should be treated as untrusted content. `aidocs` renders documents in a sandboxed frame. For production, deploy with separate app and render origins when possible.

## License

Apache-2.0
