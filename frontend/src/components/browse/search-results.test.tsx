import { MemoryRouter } from "react-router-dom";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SearchResults } from "./search-results";

describe("SearchResults", () => {
  it("shows where a document is and marks the matching text", () => {
    render(
      <MemoryRouter>
        <SearchResults
          entries={[
            {
              type: "document",
              document: {
                id: "doc_1",
                title: "Q3 incident review",
                owner: { type: "user", id: "usr_1", name: "Alice" },
                parent_folder: { id: "fld_1", title: "Payments" },
                snippet: "The error \u0002budget\u0003 for checkout",
              },
            },
          ]}
        />
      </MemoryRouter>,
    );
    expect(screen.getByRole("link", { name: /Q3 incident review/ })).toHaveAttribute(
      "href",
      "/app/d/doc_1",
    );
    expect(screen.getByText(/Alice · in Payments/)).toBeVisible();
    expect(screen.getByText("budget").tagName).toBe("MARK");
    expect(screen.getByText(/for checkout/).textContent).toBe(
      "The error budget for checkout",
    );
  });
});
