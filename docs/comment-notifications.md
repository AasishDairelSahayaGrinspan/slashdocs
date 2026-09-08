# Comment notifications and links

Comments stay beside their highlighted text. The sidebar shows the total thread
count, including threads whose text is currently hidden or missing, with All,
Open, and Resolved filters. Opening a comment link selects its thread even if it
is resolved. Each comment and reply has a **Copy comment link** action.

## Personal notification controls

Use the bell beside the comment count to mute or enable notifications from that
document. In **Profile → Slack notifications**, switch notifications off globally.
The global switch overrides document settings. Muting affects only your account;
it does not change document access or anyone else's preferences. Re-enabling
starts with future activity, without replaying the muted backlog.

With Slack configured, each new comment or reply notifies the document's human
owner and explicitly mentioned human users. The author is excluded and recipients
are deduplicated; an owner who is also mentioned receives one mention notification.
Edits, reactions, resolving, and reopening do not notify. Service accounts can
write comments and receive structured mentions, but have no Slack inbox.

Choose a person from the `@` suggestions to create a mention. The composer shows
accepted mentions below the text field. Pasting or typing around a tag preserves
it; editing the tag itself removes its association. Plain `@text` is not a tag.

## CLI workflows

Every command uses the current authenticated user or service account. Preferences
cannot be changed on someone else's behalf. Add `--json` for structured output.

```bash
# Global preference and effective per-document preference.
aidocs notifications status --json
aidocs notifications disable
aidocs notifications enable
aidocs docs notifications disable doc_123
aidocs docs notifications status doc_123 --json
aidocs docs notifications enable doc_123

# Repeated --mention flags create real tags, appending @email if absent.
aidocs docs comments create doc_123 --version ver_123 \
  --quote 'Launch checklist' --body 'Please review' \
  --mention reviewer@example.com --mention owner@example.com

# File and stdin input avoid shell escaping for longer comments.
aidocs docs comments reply doc_123 cmt_123 --body-file - \
  --mention reviewer@example.com < review.txt

# Stable principal IDs also work, including agents.
aidocs docs comments reply doc_123 cmt_123 --body 'Follow up' \
  --mention sa:sa_123

# Mention changes require the updated body; editing does not send notifications.
aidocs docs comments update doc_123 cmt_123 --body 'Updated request' \
  --mention user:usr_123
aidocs docs comments update doc_123 cmt_123 --clear-mentions

# Discover headings; get a URL without launching a browser.
aidocs docs headings doc_123 --json
aidocs docs link doc_123 --heading launch-checklist
aidocs docs link doc_123 --heading launch-checklist --version ver_123
aidocs docs link doc_123 --comment cmt_123

# Resolved threads remain available to agents.
aidocs docs comments list doc_123 --status resolved --json
```

The CLI resolves an email to an exact platform user and calculates structured
mention offsets, including Unicode. For `user:<id>` and `sa:<id>`, it uses `@<id>`
as the label. A matching standalone label already in the body is reused; otherwise
it is appended. Without `--mention`, the CLI never guesses recipients from prose.

## Heading links

Hover or focus a heading to reveal its **#** copy-link button (always visible on
touch devices). Existing HTML IDs are preserved. Headings without an ID receive
one derived from their text, with numeric suffixes to avoid collisions. For links
that should survive title edits or reordered headings, authors should supply
explicit stable IDs such as `<h2 id="launch-checklist">Launch checklist</h2>`.

Links follow the latest version unless `--version` is supplied. The headings API
lists stored HTML headings; headings generated only by JavaScript become available
in the viewer when created. Agents can link to a known runtime ID with
`docs link --heading`. Following a link opens enclosing `<details>` elements;
other custom interactions remain controlled by the document. Source HTML is not
rewritten by these viewer conveniences.

## Slack setup

This first iteration uses one internal Slack app for the deployment. Platform and
Slack email addresses must match. Install the app with bot scopes
[`users:read`](https://docs.slack.dev/reference/scopes/users.read/) (required by Slack alongside email lookup),
[`users:read.email`](https://docs.slack.dev/reference/methods/users.lookupByEmail/),
[`im:write`](https://docs.slack.dev/reference/methods/conversations.open/), and
[`chat:write`](https://docs.slack.dev/reference/methods/chat.postMessage/).
No event subscriptions or inbound Slack webhook are required.
Enable **App Home → Messages Tab** so the bot can send DMs. It can be read-only;
this integration does not handle messages sent back to the bot. In an app
manifest, set `features.app_home.messages_tab_enabled` and
`features.app_home.messages_tab_read_only_enabled` to `true`.

The server reads the bot token from `AIDOCS_SLACK_BOT_TOKEN` at startup. On
Razorpay deployments, Credstash is the internal UI for managing Kubernetes
Secrets; the Helm chart injects the secret value as an environment variable.
The application does not invoke a Credstash client or access DynamoDB/KMS for
this token.

For devstack, put the bot token at
`aidocs/aidocs-secrets/AIDOCS_SLACK_BOT_TOKEN` in the Credstash UI. The AIDocs chart
supports this through its existing `extraEnv` setting:

```yaml
extraEnv:
  - name: AIDOCS_SLACK_BOT_TOKEN
    valueFrom:
      secretKeyRef:
        name: aidocs-secrets
        key: AIDOCS_SLACK_BOT_TOKEN
```

Keep `AIDOCS_APP_ORIGIN` set to the public application origin so notification
buttons point to the right deployment. An unset or empty token disables
delivery and enqueueing. With the required `secretKeyRef` above, Kubernetes
prevents a pod from starting if the Secret or key is missing. Rotate by updating
the Kubernetes Secret through Credstash and restarting the workload; revoke the
old token after rollout. For offboarding, disable the environment injection,
revoke the Slack token, and remove its Kubernetes Secret key through Credstash.

## Delivery behavior

The comment and outbox recipients are saved in one database transaction. A worker
rechecks the recipient's current document access and notification settings before
sending a DM. Slack receives the document title, highlighted text, comment or
reply content, and a button linking to that exact comment. Replies include the
root thread's highlight. The activity leads the message, followed by
`Document: <title>` as a link, an italic Slack
blockquote, the comment, and a prominent **View comment** button. Regular Block
Kit blocks avoid the extra attribution footer Slack adds to attachments.
Text is rendered literally, without Slack mention
parsing or link unfurling. Titles are limited to 150 characters, highlights to
600, and comments to 2,200, with an ellipsis for longer content; the button opens
the full thread. Recognizable labeled passwords, API keys, access tokens, OTPs,
and card numbers are masked before sending, including in push-notification
fallback text. This masking is not a general-purpose sensitive-data classifier.

Successful delivery deletes the outbox row immediately. Missing or ineligible
Slack users are discarded. Temporary failures retry with backoff and Slack's
`Retry-After`; pending entries expire after 24 hours. A lease recovers jobs after
process crashes, and acknowledgements cannot delete a newer worker's claim.
Delivery is at least once: a crash after Slack accepts a message but before the
row is deleted can produce a duplicate. The first version processes one item per
second per worker and limits each recipient to one delivery per six seconds
across replicas. Preference/link APIs have a bounded per-principal request budget.

Tests exercise the queue against PostgreSQL and the Slack HTTP contract against
a local TLS mock. Enabling a real deployment requires its Slack app and bot token.
Local Compose accepts `AIDOCS_SLACK_BOT_TOKEN` from the launching process's
environment; when absent, it does not send Slack messages. The RAW workspace
integration check uses this same environment-variable contract and real Slack
delivery.

Applied Seezo guidance: `validation/notifications`,
`validation/third-party-services`, `secrets/credstash`, `secrets/handling`,
`authorization/access-control`, `authorization/state-changing-operations`,
`data-protection/sensitive-data`, `validation/untrusted-input`, `api/security`.
