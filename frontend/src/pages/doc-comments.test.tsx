import { http, HttpResponse } from "msw";
import { describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DocProvider, type DocContextValue } from "./doc-context";
import { Comments } from "./doc-comments";
import { server } from "@/test/msw";
import { renderWithProviders } from "@/test/render";

function renderComments(overrides: Partial<DocContextValue> = {}) {
  const value: DocContextValue = {
    docId: "doc_1",
    version: "ver_1",
    commentsLoading: false,
    commentsError: false,
    principal: { type: "user", id: "usr_1", name: "Owner" },
    selection: null,
    setSelection: vi.fn(),
    activeComment: undefined,
    setActiveComment: vi.fn(),
    comments: [
      {
        id: "cmt_1",
        body: "Needs detail",
        status: "open",
        selected_text: "important text",
        created_on_version_id: "ver_1",
        author: { type: "user", id: "usr_1", name: "Owner" },
        replies: [
          {
            id: "cmt_2",
            body: "Added",
            status: "open",
            created_on_version_id: "ver_1",
            author: { type: "user", id: "usr_2", name: "Reviewer" },
          },
        ],
      },
    ],
    ...overrides,
  };
  return {
    value,
    ...renderWithProviders(
      <DocProvider value={value}>
        <Comments />
      </DocProvider>,
    ),
  };
}

describe("Comments", () => {
  it("creates comments from a selection, replies, and resolves threads", async () => {
    const writes: unknown[] = [];
    server.use(
      http.post("/v1/documents/doc_1/comments", async ({ request }) => {
        writes.push(await request.json());
        return HttpResponse.json({ id: "cmt_new", status: "open" });
      }),
      http.post("/v1/documents/doc_1/comments/cmt_1/replies", async ({ request }) => {
        writes.push(await request.json());
        return HttpResponse.json({ id: "cmt_reply", status: "open" });
      }),
      http.patch("/v1/documents/doc_1/comments/cmt_1", async ({ request }) => {
        writes.push(await request.json());
        return HttpResponse.json({ id: "cmt_1", status: "resolved" });
      }),
    );

    renderComments({
      selection: {
        quote: "selected phrase",
        anchor: { prefix: "before", suffix: "after", dom_path: "p[1]" },
      },
    });

    await userEvent.type(screen.getByPlaceholderText(/Add a comment/i), "Check this");
    await userEvent.click(screen.getByRole("button", { name: /^Comment/i }));

    await userEvent.type(screen.getByPlaceholderText(/Reply/i), "Looks better");
    await userEvent.click(screen.getByRole("button", { name: "Reply" }));

    await userEvent.click(screen.getByRole("button", { name: "Resolve" }));

    await waitFor(() =>
      expect(writes).toEqual([
        {
          version_id: "ver_1",
          body: "Check this",
          anchor: {
            quote: "selected phrase",
            prefix: "before",
            suffix: "after",
            dom_path: "p[1]",
            start_offset: 0,
            end_offset: 15,
          },
        },
        { body: "Looks better" },
        { body: "Needs detail", status: "resolved" },
      ]),
    );
  });

  it("shows loading, error, filtered empty, and resolved comment states", async () => {
    const loading = renderComments({ commentsLoading: true, comments: [] });
    expect(document.querySelectorAll(".animate-pulse").length).toBeGreaterThan(0);
    loading.unmount();

    const error = renderComments({ commentsError: true, comments: [] });
    expect(screen.getByText("Could not load comments.")).toBeInTheDocument();
    error.unmount();

    renderComments({
      comments: [
        {
          id: "cmt_resolved",
          body: "Done",
          status: "resolved",
          created_on_version_id: "ver_1",
          author: { type: "user", id: "usr_2", name: "Reviewer" },
        },
      ],
    });
    expect(screen.getByText("No open comments")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "resolved" }));
    expect(screen.getByText("Done")).toBeInTheDocument();
    expect(screen.getAllByText("resolved").length).toBeGreaterThan(1);
  });
});
