# aidocs CLI

`aidocs` is the scriptable CLI for the aidocs HTML review service.

## Install from source

```bash
go build -o bin/aidocs ./cli
```

## Auth

```bash
aidocs auth login aidocs.example.com       # log in to your aidocs server
aidocs auth login                          # reuses the active context's server
aidocs auth whoami
aidocs auth logout                         # revokes server credential by default
AIDOCS_NO_KEYCHAIN=1 aidocs auth login ... # force file-only token storage
```

There is no built-in default server. Installing via `curl https://<your-server>/install.sh | bash` preconfigures the CLI to use that server; otherwise pass the server URL to `aidocs auth login`, use `--server`, or set `AIDOCS_SERVER`.

Self-hosted login only needs the server URL. The server owns the OIDC configuration (generic `AIDOCS_OIDC_*` variables or the Google aliases); the CLI uses server-mediated loopback PKCE.

By default, login stores the secret token in the OS keychain when available and keeps only metadata plus a `token_ref` in `~/.config/aidocs/config.json`. If the keychain is unavailable, it falls back to storing the token in the config file with `0600` permissions. Set `AIDOCS_NO_KEYCHAIN=1` to force file-only storage.

For CI/bots:

```bash
export AIDOCS_SERVER=https://aidocs.example.com
export AIDOCS_TOKEN=aidocs_sa_...
```

## Core agent loop

```bash
aidocs docs list
aidocs docs list 'incident author:alice@example.com labels:api,review in:"Product Specs"'
aidocs docs list 'author:/@razorpay\.com$/ labels:/^api-/'
aidocs docs labels set doc_123 api review-ready
aidocs docs labels list doc_123
aidocs docs pull doc_123 --out report.html
aidocs docs comments list doc_123 --json > review.json
# edit report.html
aidocs docs push doc_123 report.html --summary "Address review comments"
aidocs docs comments resolve doc_123 cmt_1 cmt_2
```

## Service accounts

```bash
aidocs sa create report-bot
aidocs sa key create sa_123 --name prod-ci
aidocs sa key revoke sa_123 sak_123
```

Disable all `sa` commands in a packaged environment:

```bash
AIDOCS_DISABLE_SA_COMMANDS=1 aidocs sa list
```

## MCP server

The aidocs application server exposes a remote Streamable HTTP MCP endpoint at:

```bash
https://<your-aidocs-host>/mcp
```

Connect that URL directly from an MCP client—there is no local MCP process or
CLI installation to manage. Interactive clients use the server's browser OAuth
flow. Headless clients can send a service-account key as an `Authorization:
Bearer …` header; create one in the aidocs web application's service-account
settings.

The remote server exposes document, version, comment, sharing, and folder
operations, plus management of existing service accounts. It intentionally
does not expose service-account or bearer-key creation; use the web application
or CLI to mint credentials.

## Skills

Refresh the bundled agent skill from the logged-in aidocs server:

```bash
aidocs skills update
```

The command installs the server-served `aidocs` skill into `~/.agents/skills/aidocs/SKILL.md` and `~/.claude/skills/aidocs/SKILL.md`. Files whose contents already match are left untouched, so local mtimes are stable.

## Updates

After most commands the CLI asks its server for the current build and refreshes the bundled skill from it. Both run only against a server you have logged in to with `aidocs auth login`. A one-off `--server`, `AIDOCS_SERVER`, or `--token` is used for that command's API calls and nothing else: it never replaces the binary and never rewrites the `SKILL.md` files your coding agent loads automatically. A context that the installer wrote but nobody has logged in to yet is not trusted either.

```bash
aidocs update                                  # update from the logged-in server
aidocs update --force                          # reinstall the current build from that server
AIDOCS_DISABLE_AUTO_UPDATE=1 aidocs docs list  # skip the automatic check
```

`aidocs update` and `aidocs skills update` refuse with an explanatory error when the server they would use is not one you are logged in to; run `aidocs auth login <server>` first. `--force` only re-installs a matching build — it is not a way to trust another server.

Build identifiers are commit SHAs and cannot be ordered, so the CLI installs whatever build its logged-in server currently advertises, including an older one after that server is rolled back. That is the point: the install tracks your server. Downloads must come from that server's own origin and must match the sha256 it publishes.

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
