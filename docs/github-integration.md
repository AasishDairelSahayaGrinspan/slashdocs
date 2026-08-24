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

If a fetch fails because the reader has not connected GitHub, let the document
offer the connection action directly. The trusted aidocs shell starts OAuth and
returns the reader to the same document and heading:

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
frames may be served from separate origins.

The document decides how to render the returned raw content. DOM inserted
after the fetch is monitored by the existing annotation bridge, so readers can
select it and attach comments normally. Files are limited to 10 MiB.

The first implementation uses a GitHub OAuth App and requests the `repo` scope,
because OAuth Apps cannot request read-only access to private repository
contents. A future GitHub App implementation can narrow this to read-only
Contents permission and short-lived installation tokens without changing the
`window.aidocs.github` API.
