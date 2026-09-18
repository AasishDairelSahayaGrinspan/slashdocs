# GitHub-backed documents

aidocs users can connect their own GitHub account from **Profile**. The server
uses a GitHub OAuth App and stores the resulting access token encrypted at
rest. The token is never returned to the browser or to uploaded document code.

Configure the OAuth App with this callback URL:

```text
https://<aidocs-app-origin>/v1/integrations/github/callback
```

Then set `AIDOCS_GITHUB_CLIENT_ID` and `AIDOCS_GITHUB_CLIENT_SECRET` (or use
the `githubOAuth` Helm values).

## Document API

Uploaded HTML can fetch the raw bytes of a repository file at a branch, tag,
or commit through the sandbox bridge:

```html
<main id="content">Loading…</main>
<script>
  const file = await window.aidocs.github.getRawContent({
    owner: "razorpay",
    repo: "example",
    path: "docs/architecture.html",
    ref: "main", // a branch, tag, or commit SHA
  });
  document.querySelector("#content").innerHTML = file.content;
</script>
```

## Per-repository consent

A document chooses its own coordinates and the server answers them with the
reader's token, so reading a repository needs the reader's approval first. The
approval is recorded per **user**, per **document**, and per **repository**:
approving `acme/docs` for one document approves nothing else, and each reader
approves for themselves. Owner and repository names are stored lowercased
because GitHub resolves them case-insensitively.

The first time a document asks for a repository the reader has not approved,
the aidocs shell — not the document — shows a dialog naming the repository and
explaining that the document will read files from it with their GitHub account.
Only a click on **Allow** records the approval and lets the read continue.
Declining answers the document with an error, and that repository stays
declined until the page is loaded again. One dialog is shown at a time, so a
document cannot flood the reader with prompts.

Until the approval exists, `POST /v1/documents/{id}/github/raw` refuses the
read and nothing is requested from GitHub:

```json
{
  "error": {
    "code": "github_consent_required",
    "message": "Approve this document reading acme/docs before it can load repository content",
    "details": { "owner": "acme", "repo": "docs" }
  }
}
```

Document JavaScript sees this as a rejected promise whose message names the
repository, so handle it like any other failure: show the reader what the
document wanted to read and let them retry. Because a decline lasts for the
page load, a retry only re-prompts after a reload.

Approvals are managed under `/v1/documents/{id}/github/consents`: `GET` lists
the ones the caller made for that document, `POST` with `{"owner", "repo"}`
records one, and `DELETE /{owner}/{repo}` withdraws one. They require at least
viewer access to the document and a user principal (a service account cannot
approve, and cannot read repository content). Disconnecting GitHub withdraws
every approval the user made, and deleting a document removes its approvals, so
reconnecting or re-uploading never revives an old one.

## Connecting from a document

If a fetch fails because the reader has not connected GitHub, let the document
offer the connection action directly. The shell asks the reader to confirm and
only then starts OAuth, returning them to the same document and heading:

```html
<button id="connect-github" type="button">Connect GitHub</button>
<script>
  document.querySelector("#connect-github").addEventListener("click", () => {
    window.aidocs.github.connect();
  });
</script>
```

The document can present this as a button or link-like control. It should call
`connect()` rather than constructing an OAuth URL, because the app and render
frames may be served from separate origins. `connect()` only asks the shell to
offer the connection: the reader still has to confirm, and a confirmation they
dismissed is not offered again until the page is loaded again.

The document decides how to render the returned raw content. DOM inserted
after the fetch is monitored by the existing annotation bridge, so readers can
select it and attach comments normally. Files are limited to 50 MiB.

The first implementation uses a GitHub OAuth App and requests the `repo` scope,
because OAuth Apps cannot request read-only access to private repository
contents. A future GitHub App implementation can narrow this to read-only
Contents permission and short-lived installation tokens without changing the
`window.aidocs.github` API.
