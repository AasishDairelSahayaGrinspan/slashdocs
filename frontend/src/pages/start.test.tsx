import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StartPage } from "./start";
import { server } from "@/test/msw";
import { renderWithProviders } from "@/test/render";

describe("StartPage", () => {
  it("uses org login command and completes required onboarding steps", async () => {
    window.__AIDOCS_PUBLIC_URL__ = "https://aidocs.example.com";
    server.use(
      http.get("/v1/config", () =>
        HttpResponse.json({
          deployment: "org",
          org_name: "Razorpay",
          everyone_label: "Everyone",
        }),
      ),
    );

    renderWithProviders(<StartPage />, { route: "/app/start" });

    expect(
      await screen.findByText(/curl -fsSL https:\/\/aidocs.example.com/i),
    ).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Mark done/i }));
    expect(
      await screen.findByText(/aidocs auth login https:\/\/aidocs.example.com/i),
    ).toBeInTheDocument();
    await userEvent.click(await screen.findByRole("button", { name: /Mark done/i }));
    await userEvent.click(await screen.findByRole("button", { name: /Mark done/i }));

    await waitFor(() =>
      expect(screen.getByText("You're all set.")).toBeInTheDocument(),
    );
    expect(screen.getAllByRole("link", { name: /Open documents/i })[0]).toHaveAttribute(
      "href",
      "/app/documents",
    );
  });

  it("skips the tutorial", async () => {
    server.use(
      http.get("/v1/config", () =>
        HttpResponse.json({
          deployment: "public",
          org_name: "",
          everyone_label: "Anyone",
        }),
      ),
    );

    renderWithProviders(<StartPage />, { route: "/app/start" });
    await userEvent.click(screen.getByRole("button", { name: /Skip tutorial/i }));
    expect(localStorage.getItem("aidocs.onboarding")).toContain("skipped");
  });
});
