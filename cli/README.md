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

## Local source development

The source checkout has a separate local launcher, keeping the installed
`aidocs` CLI unchanged:

```bash
make install-dev # one-time PATH setup
aidocs-dev       # start from the repository root
aidocs-dev test
```

The local app binds to `127.0.0.1`, uses seeded owner and reviewer identities,
and does not require OAuth, Postgres, S3, or Docker. The `aidocs` command remains
the normal document-service CLI.

By default, login stores the secret token in the OS keychain when available and keeps only metadata plus a `token_ref` in `~/.config/aidocs/config.json`. If the keychain is unavailable, it falls back to storing the token in the config file with `0600` permissions. Set `AIDOCS_NO_KEYCHAIN=1` to force file-only storage.

For CI/bots:

```bash
export AIDOCS_SERVER=https://aidocs.example.com
export AIDOCS_TOKEN=aidocs_sa_...
```

## Core agent loop

```bash
aidocs docs list
aidocs docs pull doc_123 --out report.html
aidocs docs comments list doc_123 --json > review.json
# edit report.html
aidocs docs push doc_123 report.html --summary "Address review comments"
aidocs docs comments resolve doc_123 cmt_1 cmt_2
```

For a document inside a protected repository, propose the edited HTML and let a
different contributor review it:

```bash
aidocs repos list
aidocs repos prs create repo_123 doc_123 report.html \
  --title "Address review comments" \
  --summary "Clarify the rollout plan"

# Run these as an independent reviewer/editor:
aidocs repos prs show repo_123 1
aidocs repos prs review repo_123 1 --approve --body "Ready to merge."
aidocs repos prs merge repo_123 1

# Permanently remove a repository and all of its contents (owner only):
aidocs repos delete repo_123
```

Repositories may start empty or from one owned loose document or root folder,
using `aidocs repos create`, `--document-id ...`, or `--folder-id ...`.

For a multi-document or tree change, put the pull-request body and `changes`
array in JSON and use the change-set commands:

```bash
aidocs repos prs change-set repo_123 changes.json
aidocs repos prs patchset repo_123 7 revised-changes.json
aidocs repos prs rebase repo_123 7
```

## Connected sources and knowledge tools

```bash
aidocs repos sources add repo_123 \
  --provider github \
  --repository razorpay/payments \
  --branch master \
  --include 'api/**,docs/**' \
  --target-document doc_123

# Preview is read-only; check records the run and opens/updates a doc PR.
aidocs repos sources simulate repo_123 source_123 \
  --after local-preview --changed-file api/refunds/handler.go
aidocs repos sources check repo_123 source_123 \
  --after merge-sha --changed-file api/refunds/handler.go

aidocs repos sync-runs repo_123
aidocs repos activity repo_123
aidocs repos search repo_123 "refund ownership"
aidocs repos ask repo_123 "Who owns refund operations?"
aidocs repos graph repo_123
aidocs repos backlinks repo_123 doc_123
```

Repository owners can manage policy and review teams from JSON or flags:

```bash
aidocs repos settings update repo_123 settings.json
aidocs repos teams create repo_123 --name "Payments reviewers" --member user_123
aidocs repos teams members add repo_123 team_123 user_456
aidocs repos settings archive repo_123
aidocs repos settings restore repo_123
aidocs repos settings transfer repo_123 --to new-owner@example.com
aidocs repos settings transfer accept repo_123
```

Global collaboration commands include `aidocs inbox`,
`aidocs notifications list`, and `aidocs templates list`. Applying a template
can be previewed before it opens a protected pull request:

```bash
aidocs templates apply repo_123 runbook --target-document doc_123 --preview
```

See [`docs/connected-sources.md`](../docs/connected-sources.md) for webhook and
local testing details.

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

Refresh the bundled agent skill from the connected aidocs server:

```bash
aidocs skills update
```

The command installs the latest server-served `aidocs` skill into `~/.agents/skills/aidocs/SKILL.md` and `~/.claude/skills/aidocs/SKILL.md`.

## Output

Use `--json` for machine-readable output. Without `--json`, commands print concise human-readable summaries.

## Exit codes

- `0`: success
- `1`: generic error
- `2`: usage error
- `3`: auth/permission error
- `4`: not found
- `5`: version, pull-request, or repository conflict
- `6`: network error
