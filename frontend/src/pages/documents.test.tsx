import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DocumentsPage } from "./documents";
import { server } from "@/test/msw";
import { renderWithProviders } from "@/test/render";

function finishOnboarding() {
  localStorage.setItem(
    "aidocs.onboarding",
    JSON.stringify({
      status: "complete",
      steps: {
        cli_installed: true,
        cli_authed: true,
        agent_configured: true,
        first_doc_seen: false,
      },
    }),
  );
}

describe("DocumentsPage", () => {
  it("renders documents, owner details, and search results", async () => {
    finishOnboarding();
    server.use(
      http.get("/v1/me", () =>
        HttpResponse.json({
          principal: { type: "user", id: "usr_1" },
          user: { id: "usr_1", email: "owner@example.com", name: "Owner" },
        }),
      ),
      http.get("/v1/documents", ({ request }) => {
        const q = new URL(request.url).searchParams.get("q");
        return HttpResponse.json({
          items:
            q === "roadmap"
              ? [
                  {
                    id: "doc_2",
                    title: "Roadmap",
                    // nosemgrep: generic.secrets.security.detected-google-gcm-service-account.detected-google-gcm-service-account
                    owner: { type: "service_account", id: "sa_1" },
                  },
                ]
              : [
                  {
                    id: "doc_1",
                    title: "Quarterly Review",
                    owner: {
                      type: "user",
                      id: "usr_1",
                      email: "owner@example.com",
                      name: "Owner",
                    },
                  },
                ],
        });
      }),
    );

    renderWithProviders(<DocumentsPage />, { route: "/app/documents" });
    expect(
      await screen.findByRole("link", { name: /Quarterly Review/i }),
    ).toHaveAttribute("href", "/app/d/doc_1");
    expect(screen.getByText("owner@example.com")).toBeInTheDocument();

    await userEvent.type(screen.getByPlaceholderText(/Search/i), "roadmap");
    expect(await screen.findByRole("link", { name: /Roadmap/i })).toHaveAttribute(
      "href",
      "/app/d/doc_2",
    );
    await waitFor(() =>
      expect(screen.queryByText("Quarterly Review")).not.toBeInTheDocument(),
    );
  });

  it("shows empty and error states", async () => {
    finishOnboarding();
    server.use(
      http.get("/v1/me", () =>
        HttpResponse.json({ principal: { type: "user", id: "usr_1" } }),
      ),
      http.get("/v1/documents", () => HttpResponse.json({ items: [] })),
    );

    const view = renderWithProviders(<DocumentsPage />);
    expect(await screen.findByText("Set up your agent")).toBeInTheDocument();

    server.use(http.get("/v1/documents", () => HttpResponse.json({}, { status: 500 })));
    view.unmount();
    renderWithProviders(<DocumentsPage />);
    expect(await screen.findByText("Could not load documents.")).toBeInTheDocument();
  });
});
