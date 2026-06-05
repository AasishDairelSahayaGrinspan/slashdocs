import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ServiceAccountsPage } from "./service-accounts";
import { server } from "@/test/msw";
import { renderWithProviders } from "@/test/render";

describe("ServiceAccountsPage", () => {
  it("lists bots and creates a new key", async () => {
    const requests: unknown[] = [];
    server.use(
      http.get("/v1/service-accounts", () =>
        HttpResponse.json({
          items: [{ id: "sa_1", name: "release-bot@example.bot" }],
        }),
      ),
      http.get("/v1/service-accounts/sa_1/keys", () =>
        HttpResponse.json({ items: [{ id: "sak_1", name: "default" }] }),
      ),
      http.post("/v1/service-accounts/sa_1/keys", async ({ request }) => {
        requests.push(await request.json());
        return HttpResponse.json({ id: "sak_2", token: "aidocs_sa_secret" });
      }),
    );

    renderWithProviders(<ServiceAccountsPage />, {
      route: "/app/settings/service-accounts",
    });

    expect(
      (await screen.findAllByText("release-bot@example.bot")).length,
    ).toBeGreaterThan(0);
    expect(await screen.findByText("sak_1")).toBeInTheDocument();

    const keyName = screen.getByPlaceholderText("Key name");
    await userEvent.clear(keyName);
    await userEvent.type(keyName, "ci");
    await userEvent.click(screen.getByRole("button", { name: /New key/i }));

    expect(await screen.findByText("aidocs_sa_secret")).toBeInTheDocument();
    expect(requests).toEqual([{ name: "ci" }]);
  });

  it("creates a bot and validates bot domains before submit", async () => {
    const payloads: unknown[] = [];
    server.use(
      http.get("/v1/service-accounts", () => HttpResponse.json({ items: [] })),
      http.post("/v1/service-accounts", async ({ request }) => {
        payloads.push(await request.json());
        return HttpResponse.json({
          id: "sa_2",
          label: "n8n",
          name: "n8n@ops.bot",
          key: { id: "sak_1", token: "aidocs_sa_new" },
        });
      }),
    );

    renderWithProviders(<ServiceAccountsPage />);
    await userEvent.click(screen.getByRole("button", { name: /New bot/i }));
    await userEvent.type(screen.getByPlaceholderText(/n8n-prod/i), "n8n@ops.com");

    expect(screen.getByText(/Addresses must end in .bot/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Create bot/i })).toBeDisabled();

    await userEvent.clear(screen.getByPlaceholderText(/n8n-prod/i));
    await userEvent.type(screen.getByPlaceholderText(/n8n-prod/i), "n8n@ops.bot");
    await userEvent.click(screen.getByRole("button", { name: /Create bot/i }));

    expect(await screen.findByText("n8n@ops.bot")).toBeInTheDocument();
    expect(await screen.findByText("aidocs_sa_new")).toBeInTheDocument();
    await waitFor(() =>
      expect(payloads).toEqual([{ label: "n8n", domain: "ops.bot" }]),
    );
  });

  it("blocks bot labels that the backend rejects", async () => {
    const payloads: unknown[] = [];
    server.use(
      http.get("/v1/service-accounts", () => HttpResponse.json({ items: [] })),
      http.post("/v1/service-accounts", async ({ request }) => {
        payloads.push(await request.json());
        return HttpResponse.json(
          { error: { code: "bad_request", message: "invalid label" } },
          { status: 400 },
        );
      }),
    );

    renderWithProviders(<ServiceAccountsPage />);
    await userEvent.click(screen.getByRole("button", { name: /New bot/i }));
    await userEvent.type(screen.getByPlaceholderText(/n8n-prod/i), "DeployBot");

    expect(screen.getByRole("button", { name: /Create bot/i })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: /Create bot/i }));
    expect(payloads).toEqual([]);
  });
});
