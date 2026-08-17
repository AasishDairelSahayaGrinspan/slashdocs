import { useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DocumentSearchInput } from "./document-search-input";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function Harness() {
  const [value, setValue] = useState("");
  return <DocumentSearchInput value={value} onChange={setValue} />;
}

describe("DocumentSearchInput", () => {
  it("autocompletes an author token with keyboard navigation", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/v1/users?q=ali") {
        return jsonResponse({
          items: [
            {
              type: "user",
              id: "usr_alice",
              email: "alice@example.com",
              name: "Alice",
            },
          ],
        });
      }
      return jsonResponse({ items: [] });
    });
    vi.stubGlobal("fetch", fetchMock);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const user = userEvent.setup();
    render(
      <QueryClientProvider client={client}>
        <Harness />
      </QueryClientProvider>,
    );

    const input = screen.getByRole("combobox", { name: "Search documents" });
    await user.type(input, "incident author:ali");
    expect(
      await screen.findByRole("option", { name: /alice@example\.com/i }),
    ).toBeVisible();
    await user.keyboard("{Enter}");

    await waitFor(() =>
      expect(input).toHaveValue("incident author:alice@example.com "),
    );
    expect(fetchMock).toHaveBeenCalledWith("/v1/users?q=ali", expect.anything());
  });
});
