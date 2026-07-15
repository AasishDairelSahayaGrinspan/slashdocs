import assert from "node:assert/strict";
import test from "node:test";

import type { ConnectedAccount } from "../src/api.ts";
import {
  connectedAccountContext,
  connectedAccountLabel,
  connectedAccountsForProvider,
  isConnectedAccountProvider,
  isUsableConnectedAccount,
} from "../src/lib/connected-accounts.ts";

function connection(
  id: string,
  provider: ConnectedAccount["provider"],
  status: string,
): ConnectedAccount {
  return {
    id,
    provider,
    status,
    external_account_id: `${provider}-${id}`,
    display_name: `${provider} ${id}`,
  };
}

test("only providers with personal connection flows require an account", () => {
  for (const provider of ["github", "slack", "devrev", "google_drive"] as const) {
    assert.equal(isConnectedAccountProvider(provider), true);
  }
  for (const provider of ["confluence", "notion", "manual"] as const) {
    assert.equal(isConnectedAccountProvider(provider), false);
  }
});

test("active and demo accounts are usable while reconnecting accounts are not", () => {
  assert.equal(
    isUsableConnectedAccount(connection("active", "github", "active")),
    true,
  );
  assert.equal(isUsableConnectedAccount(connection("demo", "slack", "demo")), true);
  assert.equal(
    isUsableConnectedAccount(connection("expired", "google_drive", "needs_reconnect")),
    false,
  );
});

test("provider filtering preserves multiple accounts and account labels", () => {
  const github = connection("personal", "github", "active");
  const work = {
    ...connection("work", "github", "active"),
    workspace_name: "Razorpay",
  };
  const slack = connection("slack", "slack", "active");

  assert.deepEqual(connectedAccountsForProvider([github, slack, work], "github"), [
    github,
    work,
  ]);
  assert.equal(connectedAccountLabel(work), "github work");
  assert.equal(connectedAccountContext(work), "Razorpay");
});
