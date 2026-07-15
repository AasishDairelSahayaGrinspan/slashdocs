---
name: aidocs
description: Use whenever the user mentions aidocs, an aidocs URL, an aidocs document/folder/repository/pull request/comment/share/version, or asks to publish, review, open, or update a long-form artifact in aidocs.
---

# aidocs

Use this skill for **any aidocs-related task**. If the user mentions aidocs or
points at an aidocs asset, use the `aidocs` CLI to inspect or act on it. If given an aidocs document URL, don't try to curl or web fetch it. Use the aidocs cli.

## First rule: read the guidelines before authoring

Before writing or publishing an aidocs document in a session, run:

```bash
aidocs guidelines
```

Treat that command as the source of truth for authoring constraints (single-file
HTML, images/assets, theme integration, and publishing examples). Read the CLI
output and follow it.

## Common workflows

### Existing aidocs asset

When the user asks about an existing aidocs document, folder, repository, pull
request, comment, share, version, or URL:

1. Use `aidocs --help` and the relevant subcommand help to discover exact syntax.
2. Use the CLI to inspect the asset (`docs show`, `docs versions`, `docs pull`,
   `docs comments`, `folders`, `repos`, `repos prs`, `open`, etc. as
   appropriate).
3. Perform the requested action through the CLI.
4. Return the important CLI result or URL to the user.

### New long-form artifact

When the user wants a report, RCA, postmortem, spec, review, analysis, launch
plan, findings doc, or other long-form artifact and has not specified another
format:

1. Run `aidocs guidelines` if you have not already done so in this session.
2. Write the artifact as a single HTML file that follows those guidelines.
3. Save it locally (for example `report.html`).
4. Use `aidocs docs --help` and `aidocs docs create --help` to confirm current
   syntax.
5. Publish with the CLI.
6. Return the printed aidocs document URL.

### Updating an existing document

If the user is iterating on an existing aidocs document, pull its current
version first and then push a new version instead of creating a duplicate. Use
the CLI help to find the current syntax and include a short summary of what
changed.

If `aidocs docs push` reports that a pull request is required, the document is
inside a protected repository. Inspect `aidocs repos --help` and `aidocs repos
prs --help`, identify the repository, and open a pull request with the edited
HTML instead of trying to bypass protection. Return the pull-request URL so a
different authorized reviewer can approve it. Never approve your own change.

### Repositories, change sets, and connected sources

Use `aidocs repos` for repository-scoped work. Protected changes may span
several documents and tree operations; inspect `aidocs repos prs change-set
--help`, prepare the requested JSON change set, and submit one atomic pull
request. Use patchsets for revisions and rebase only when the CLI reports a
stale base. A new patchset dismisses approvals from the previous one.

When the task concerns keeping documentation aligned with code or another
knowledge system, inspect `aidocs repos sources --help`. Prefer `simulate` for a
read-only impact check. Use `check` only when the user wants a durable sync run
and a reviewable documentation PR. Never push source-generated content directly
to the published document or automatically approve the integration's PR.

Repository search, cited questions, backlinks, the dependency graph, activity,
review inbox, notifications, teams, templates, and lifecycle settings are
available through their corresponding `aidocs repos` or top-level commands.
Use the CLI help as the exact contract rather than guessing flags.

## MCP server (optional)

The aidocs application server exposes MCP directly over Streamable HTTP at:

```bash
https://<your-aidocs-host>/mcp
```

Connect the URL from the MCP client; no aidocs CLI installation or local MCP
process is required. Interactive clients authenticate through the server's
browser flow. Headless clients can use a service-account bearer key created in
the aidocs web application. If the remote aidocs MCP tools are already available
in your session, prefer them; otherwise use the CLI commands described above.

## Authentication and setup

Assume the user installed aidocs and the bundled skill via the server's
`curl ... | bash` installer. If the CLI reports that auth is missing or expired,
run `aidocs auth login` (include the server URL if the current context requires
it), then retry the requested CLI action. If login cannot be completed in the
environment, explain that clearly and ask the user to authenticate.

## When not to publish

Do not publish to aidocs if the user explicitly asks for Markdown, plain text, a
PDF, inline chat only, or another destination. If they mention aidocs while asking
for one of those formats, still use the CLI when relevant to inspect or reference
the aidocs asset, but respect the requested output format.
