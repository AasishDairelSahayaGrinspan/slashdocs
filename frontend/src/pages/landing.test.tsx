import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { screen } from "@testing-library/react";
import { LandingPage } from "./landing";
import { server } from "@/test/msw";
import { renderWithProviders } from "@/test/render";

describe("LandingPage", () => {
  it("renders public marketing and install content for signed-out users", async () => {
    server.use(
      http.get("/v1/me", () =>
        HttpResponse.json(
          { error: { code: "unauthorized", message: "authentication required" } },
          { status: 401 },
        ),
      ),
    );

    renderWithProviders(<LandingPage />);
    expect(
      await screen.findByText(/Give your agents the power to publish/i),
    ).toBeInTheDocument();
    expect(screen.getByText("Install in seconds")).toBeInTheDocument();
    expect(screen.getByText("Incident reports & RCAs")).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: /Sign in/i })[0]).toHaveAttribute(
      "href",
      "/login",
    );
  });
});
