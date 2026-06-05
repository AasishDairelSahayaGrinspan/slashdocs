import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DevelopersPage } from "./developers";
import { LoginPage } from "./login";
import { ProfilePage } from "./profile";
import { server } from "@/test/msw";
import { renderWithProviders } from "@/test/render";

describe("LoginPage", () => {
  it("builds a login URL from the next parameter", async () => {
    server.use(
      http.get("/v1/me", () =>
        HttpResponse.json({ error: { code: "unauthorized" } }, { status: 401 }),
      ),
    );

    renderWithProviders(<LoginPage />, { route: "/login?next=/app/d/doc_1" });
    const link = await screen.findByRole("link", { name: /Continue with Google/i });
    expect(link).toHaveAttribute(
      "href",
      "/v1/auth/google/start?mode=web&redirect=%2Fapp%2Fd%2Fdoc_1",
    );
  });
});

describe("ProfilePage", () => {
  it("renders user details, switches theme, and signs out", async () => {
    server.use(
      http.get("/v1/me", () =>
        HttpResponse.json({
          principal: { type: "user", id: "usr_1", email: "me@example.com", name: "Me" },
          user: { type: "user", id: "usr_1", email: "me@example.com", name: "Me" },
        }),
      ),
    );

    renderWithProviders(<ProfilePage />, { route: "/app/settings/profile" });
    expect(await screen.findByText("me@example.com")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /Dark/i }));
    await waitFor(() =>
      expect(document.documentElement.classList.contains("dark")).toBe(true),
    );

    await userEvent.click(screen.getByRole("button", { name: /Sign out/i }));
    expect(localStorage.getItem("aidocs.frontend_signed_out")).toBe("true");
  });

  it("shows the profile error state", async () => {
    server.use(http.get("/v1/me", () => HttpResponse.json({}, { status: 500 })));

    renderWithProviders(<ProfilePage />);
    expect(await screen.findByText("Could not load your profile.")).toBeInTheDocument();
  });
});

describe("DevelopersPage", () => {
  it("renders developer entry points and raw HTTP example", () => {
    window.__AIDOCS_PUBLIC_URL__ = "https://aidocs.example.com";
    renderWithProviders(<DevelopersPage />);

    expect(screen.getByRole("heading", { name: "Developers" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Open setup guide/i })).toHaveAttribute(
      "href",
      "/app/start",
    );
    expect(screen.getByText(/curl -H/)).toHaveTextContent("https://aidocs.example.com");
  });
});
