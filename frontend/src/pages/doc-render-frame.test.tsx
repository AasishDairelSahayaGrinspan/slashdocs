import { http, HttpResponse } from "msw";
import { describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import { DocProvider, type DocContextValue } from "./doc-context";
import { RenderFrame } from "./doc-render-frame";
import { server } from "@/test/msw";
import { renderWithProviders } from "@/test/render";

describe("RenderFrame", () => {
  it("creates a render token and renders a themed iframe", async () => {
    server.use(
      http.post("/v1/documents/doc_1/versions/ver_1/render-token", () =>
        HttpResponse.json({
          token: "render-token",
          url: "https://render.example.com/v1/documents/doc_1/render",
        }),
      ),
    );
    const value: DocContextValue = {
      docId: "doc_1",
      version: "ver_1",
      commentsLoading: false,
      commentsError: false,
      comments: [
        {
          id: "cmt_1",
          body: "Open",
          status: "open",
          selected_text: "quote",
          created_on_version_id: "ver_1",
          author: { type: "user", id: "usr_1" },
        },
        {
          id: "cmt_2",
          body: "Resolved",
          status: "resolved",
          selected_text: "resolved",
          created_on_version_id: "ver_1",
          author: { type: "user", id: "usr_1" },
        },
      ],
      principal: { type: "user", id: "usr_1" },
      selection: null,
      setSelection: vi.fn(),
      activeComment: "cmt_1",
      setActiveComment: vi.fn(),
    };

    renderWithProviders(
      <DocProvider value={value}>
        <RenderFrame />
      </DocProvider>,
    );

    expect(await screen.findByTitle("Rendered document")).toHaveAttribute(
      "src",
      "https://render.example.com/v1/documents/doc_1/render?aidocs_theme=light",
    );
  });
});
