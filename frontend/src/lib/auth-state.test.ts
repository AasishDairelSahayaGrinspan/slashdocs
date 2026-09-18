import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { isFrontendSignedOut, signOut } from "./auth-state";

// The test environment has no Storage implementation, so the flag needs a
// stand-in that behaves like one.
function installLocalStorage() {
  const entries = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => entries.get(key) ?? null,
    setItem: (key: string, value: string) => entries.set(key, value),
    removeItem: (key: string) => entries.delete(key),
  });
}

function installFetch(response: Response | Error) {
  const fetch = vi.fn(async () => {
    if (response instanceof Error) throw response;
    return response;
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

beforeEach(installLocalStorage);

afterEach(() => {
  vi.unstubAllGlobals();
});

it("signs out on the server before marking the app signed out", async () => {
  const fetch = installFetch(new Response(null, { status: 204 }));

  await signOut();

  expect(fetch).toHaveBeenCalledTimes(1);
  const [path, init] = fetch.mock.calls[0] as unknown as [string, RequestInit];
  expect(path).toBe("/v1/auth/logout");
  expect(init.method).toBe("POST");
  expect(init.credentials).toBe("include");
  expect(isFrontendSignedOut()).toBe(true);
});

it("keeps the user signed in when the server rejects the sign-out", async () => {
  installFetch(
    new Response(JSON.stringify({ error: { code: "internal", message: "nope" } }), {
      status: 500,
      headers: { "content-type": "application/json" },
    }),
  );

  await expect(signOut()).rejects.toThrow("nope");
  expect(isFrontendSignedOut()).toBe(false);
});

it("keeps the user signed in when the sign-out request never reaches the server", async () => {
  installFetch(new TypeError("Failed to fetch"));

  await expect(signOut()).rejects.toThrow("Failed to fetch");
  expect(isFrontendSignedOut()).toBe(false);
});
