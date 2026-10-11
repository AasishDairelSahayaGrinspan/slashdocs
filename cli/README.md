# slashdocs CLI

`slashdocs` is the scriptable CLI for the slashdocs HTML review service.

## Install from source

```bash
go build -o bin/slashdocs ./cli
```

## Auth

```bash
slashdocs auth login slashdocs.example.com       # log in to your slashdocs server
slashdocs auth login                          # reuses the active context's server
slashdocs auth whoami
slashdocs auth logout                         # revokes server credential by default
SLASHDOCS_NO_KEYCHAIN=1 slashdocs auth login ... # force file-only token storage
```

There is no built-in default server. Installing via `curl https://<your-server>/install.sh | sh` preconfigures the CLI to use that server; otherwise pass the server URL to `slashdocs auth login`, use `--server`, or set `SLASHDOCS_SERVER`.

Self-hosted login only needs the server URL. The server owns the OIDC configuration (generic `SLASHDOCS_OIDC_*` variables or the Google aliases); the CLI uses server-mediated loopback PKCE.

By default, login stores the secret token in the OS keychain when available and keeps only metadata plus a `token_ref` in `~/.config/slashdocs/config.json`. If the keychain is unavailable, it falls back to storing the token in the config file with `0600` permissions. Set `SLASHDOCS_NO_KEYCHAIN=1` to force file-only storage.

For CI/bots:

```bash
export SLASHDOCS_SERVER=https://slashdocs.example.com
export SLASHDOCS_TOKEN=slashdocs_sa_...
```

## Core agent loop

```bash
slashdocs docs list
slashdocs docs list 'incident author:alice@example.com labels:api,review in:"Product Specs"'
slashdocs docs list 'author:/@example\.com$/ labels:/^api-/'
slashdocs docs labels set doc_123 api review-ready
slashdocs docs labels list doc_123
slashdocs docs pull doc_123 --out report.html
slashdocs docs comments list doc_123 --json > review.json
# edit report.html
slashdocs docs push doc_123 report.html --summary "Address review comments"
slashdocs docs comments resolve doc_123 cmt_1 cmt_2
```

## Service accounts

```bash
slashdocs sa create report-bot
slashdocs sa key create sa_123 --name prod-ci
slashdocs sa key revoke sa_123 sak_123
```

Disable all `sa` commands in a packaged environment:

```bash
SLASHDOCS_DISABLE_SA_COMMANDS=1 slashdocs sa list
```

## MCP server

The slashdocs application server exposes a remote Streamable HTTP MCP endpoint at:

```bash
https://<your-slashdocs-host>/mcp
```

Connect that URL directly from an MCP client—there is no local MCP process or
CLI installation to manage. Interactive clients use the server's browser OAuth
flow. Headless clients can send a service-account key as an `Authorization:
Bearer …` header; create one in the slashdocs web application's service-account
settings.

The remote server exposes document, version, comment, sharing, and folder
operations, plus management of existing service accounts. It intentionally
does not expose service-account or bearer-key creation; use the web application
or CLI to mint credentials.

## Skills

Refresh the bundled agent skill from the connected slashdocs server:

```bash
slashdocs skills update
```

The command installs the latest server-served `slashdocs` skill into `~/.agents/skills/slashdocs/SKILL.md` and `~/.claude/skills/slashdocs/SKILL.md`.

## Output

Use `--json` for machine-readable output. Without `--json`, commands print concise human-readable summaries.

## Exit codes

- `0`: success
- `1`: generic error
- `2`: usage error
- `3`: auth/permission error
- `4`: not found
- `5`: version conflict
- `6`: network error
