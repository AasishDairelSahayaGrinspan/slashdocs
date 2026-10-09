import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Sheet } from "@/components/ui/sheet";
import { ShareSheet } from "./doc-share-sheet";

// Sharing is owner-only, but a non-owner may still leave: removing their own
// grant. The affordance must exist on their own row only, and must never be a
// route to the "anyone" grant that admits everyone else.
const ME = { type: "user", id: "usr_me", email: "me@example.com", name: "Me" };

function jsonResponse(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function installFetch() {
  const calls: Array<{ url: string; method: string }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      calls.push({ url, method: init?.method ?? "GET" });
      if (url === "/v1/config") {
        return jsonResponse({
          deployment: "org",
          org_name: "Acme",
          everyone_label: "Anyone in Acme",
          google_groups_enabled: false,
        });
      }
      // /v1/me nests the caller under "principal"; a flat principal here would
      // let a wrong accessor in the component pass this test.
      if (url === "/v1/me") return jsonResponse({ principal: ME, user: ME });
      if (url === "/v1/documents/doc_1/access") {
        return jsonResponse({
          current_role: "commenter",
          canonical_owner: false,
          items: [
            {
              principal: {
                type: "user",
                id: "usr_owner",
                email: "owner@example.com",
                name: "Owner",
              },
              effective_role: "owner",
              sources: [
                {
                  resource_type: "document",
                  resource_id: "doc_1",
                  resource_title: "Doc",
                  role: "owner",
                },
              ],
            },
            {
              principal: ME,
              effective_role: "commenter",
              sources: [
                {
                  resource_type: "document",
                  resource_id: "doc_1",
                  resource_title: "Doc",
                  role: "commenter",
                  grant_id: "gr_self",
                },
              ],
            },
            {
              principal: { type: "anyone", id: "" },
              effective_role: "commenter",
              sources: [
                {
                  resource_type: "document",
                  resource_id: "doc_1",
                  resource_title: "Doc",
                  role: "commenter",
                  grant_id: "gr_anyone",
                },
              ],
            },
          ],
        });
      }
      if (url.includes("/grants/")) return new Response(null, { status: 204 });
      throw new Error(`unexpected fetch: ${url}`);
    }),
  );
  return calls;
}

function renderShare() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <Sheet open>
        <ShareSheet docId="doc_1" canManageAccess={false} isCanonicalOwner={false} />
      </Sheet>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("non-owner sharing controls", () => {
  it("revokes only the caller's own grant when they leave", async () => {
    const calls = installFetch();
    renderShare();

    const leave = await screen.findByRole("button", { name: /leave document/i });
    await userEvent.click(leave);

    await waitFor(() => {
      expect(
        calls.some((c) => c.url.endsWith("/grants/gr_self") && c.method === "DELETE"),
      ).toBe(true);
    });
    expect(calls.some((c) => c.url.endsWith("/grants/gr_anyone"))).toBe(false);
  });

  it("offers exactly one leave control, on the caller's row", async () => {
    installFetch();
    renderShare();

    await screen.findByRole("button", { name: /leave document/i });
    expect(screen.getAllByRole("button", { name: /leave document/i })).toHaveLength(1);
  });
});
