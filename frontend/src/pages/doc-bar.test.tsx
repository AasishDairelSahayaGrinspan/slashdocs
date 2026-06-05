import { describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DocBar } from "./doc-bar";
import { renderWithProviders } from "@/test/render";

describe("DocBar", () => {
  it("shows older-version state and selects versions", async () => {
    const onSelectVersion = vi.fn();
    renderWithProviders(
      <DocBar
        title="Architecture Review"
        docId="doc_1"
        owner={{ type: "user", id: "usr_1", name: "Owner" }}
        principal={{ type: "user", id: "usr_1", name: "Owner" }}
        current="ver_1"
        latest="ver_2"
        versions={[
          {
            id: "ver_1",
            number: 1,
            change_summary: "Initial",
            created_by: { type: "user", id: "usr_1", name: "Owner" },
          },
          {
            id: "ver_2",
            number: 2,
            change_summary: "Updated",
            created_by: { type: "service_account", id: "sa_1" },
          },
        ]}
        onSelectVersion={onSelectVersion}
      />,
      { route: "/app/d/doc_1" },
    );

    expect(screen.getByText("Architecture Review")).toBeInTheDocument();
    expect(screen.getByText(/Viewing older version/i)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Back to latest/i }));
    expect(onSelectVersion).toHaveBeenCalledWith("ver_2");

    await userEvent.click(screen.getByRole("button", { name: /v1/i }));
    expect(await screen.findByText("Version history")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Version 2/i }));
    expect(onSelectVersion).toHaveBeenCalledWith("ver_2");
  });

  it("hides management controls for non-owners", () => {
    renderWithProviders(
      <DocBar
        title="Shared"
        docId="doc_1"
        owner={{ type: "user", id: "owner" }}
        principal={{ type: "user", id: "viewer" }}
        current="ver_1"
        latest="ver_1"
        versions={[]}
        onSelectVersion={vi.fn()}
      />,
    );

    expect(screen.queryByRole("button", { name: /Share/i })).not.toBeInTheDocument();
  });
});
