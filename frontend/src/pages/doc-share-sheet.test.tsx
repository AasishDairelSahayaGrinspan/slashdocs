import { Sheet } from "@/components/ui/sheet";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ShareSheet } from "./doc-share-sheet";
import { server } from "@/test/msw";
import { renderWithProviders } from "@/test/render";

describe("ShareSheet", () => {
  it("searches users, creates grants, updates access, and copies links", async () => {
    const writes: unknown[] = [];
    server.use(
      http.get("/v1/config", () =>
        HttpResponse.json({
          deployment: "org",
          org_name: "Razorpay",
          everyone_label: "Everyone at Razorpay",
        }),
      ),
      http.get("/v1/documents/doc_1/grants", () =>
        HttpResponse.json({
          items: [
            {
              id: "gr_1",
              role: "viewer",
              principal: {
                type: "user",
                id: "usr_2",
                name: "Reviewer",
                email: "reviewer@example.com",
              },
            },
            { id: "gr_any", role: "viewer", principal: { type: "anyone" } },
          ],
        }),
      ),
      http.get("/v1/users", ({ request }) => {
        expect(new URL(request.url).searchParams.get("q")).toBe("review");
        return HttpResponse.json({
          items: [{ id: "usr_3", name: "Review Lead", email: "lead@example.com" }],
        });
      }),
      http.post("/v1/documents/doc_1/grants", async ({ request }) => {
        writes.push(await request.json());
        return HttpResponse.json({ id: "gr_2", role: "editor" });
      }),
      http.patch("/v1/documents/doc_1/grants/gr_1", async ({ request }) => {
        writes.push(await request.json());
        return HttpResponse.json({ id: "gr_1", role: "editor" });
      }),
    );

    renderWithProviders(
      <Sheet open>
        <ShareSheet docId="doc_1" />
      </Sheet>,
    );

    expect(await screen.findByText("Reviewer")).toBeInTheDocument();
    const address = screen.getByPlaceholderText(/user@org.example/i);
    await userEvent.type(address, "review");
    await userEvent.click(await screen.findByText("Review Lead"));
    await userEvent.selectOptions(screen.getAllByLabelText("Access role")[0], "editor");
    await userEvent.click(screen.getByRole("button", { name: "Share" }));

    await userEvent.selectOptions(screen.getAllByLabelText("Access role")[1], "editor");
    await userEvent.click(screen.getByRole("button", { name: /Copy link/i }));

    await waitFor(() =>
      expect(writes).toEqual([
        { address: "lead@example.com", role: "editor" },
        { role: "editor" },
      ]),
    );
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
      `${window.location.origin}/app/d/doc_1`,
    );
  });
});
