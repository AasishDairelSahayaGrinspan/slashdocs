import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "@/api";
import { DEFAULT_COMMENT_FILTER } from "@/lib/constants";
import { DocProvider, type DocContextValue } from "./doc-context";
import { RenderFrame } from "./doc-render-frame";

// The rendered document shares an origin with the bridge, so every message
// below is what a hostile document can send. These tests pin what the app
// shell does with them.
const RENDER_ORIGIN = "https://render.example";

type FetchCall = { url: string; method: string; body?: string };

function installFetch(calls: FetchCall[], options: { grantFails?: boolean } = {}) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      calls.push({
        url,
        method: init?.method || "GET",
        body: typeof init?.body === "string" ? init.body : undefined,
      });
      if (url.endsWith("/render-token")) {
        return jsonResponse({
          token: "render-token",
          url: `${RENDER_ORIGIN}/v/doc_1/ver_1`,
        });
      }
      if (url.endsWith("/github/consents") && (init?.method || "GET") === "GET") {
        return jsonResponse({ items: [] });
      }
      if (url.endsWith("/github/consents")) {
        if (options.grantFails) {
          return jsonResponse(
            { error: { code: "internal", message: "internal server error" } },
            500,
          );
        }
        return jsonResponse({ owner: "acme", repo: "secrets" }, 201);
      }
      if (url.endsWith("/github/raw")) {
        return jsonResponse({
          owner: "acme",
          repo: "secrets",
          path: ".env",
          ref: "main",
          content: "STRIPE_KEY=sk_live_1",
        });
      }
      throw new Error(`unexpected request ${url}`);
    }),
  );
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function docContext(): DocContextValue {
  return {
    docId: "doc_1",
    version: "ver_1",
    comments: [],
    commentFilter: DEFAULT_COMMENT_FILTER,
    setCommentFilter: vi.fn(),
    commentsLoading: false,
    commentsError: false,
    selection: null,
    setSelection: vi.fn(),
    setActiveComment: vi.fn(),
    unanchoredComments: new Set<string>(),
    setUnanchoredComments: vi.fn(),
    commentPositions: new Map(),
    setCommentPositions: vi.fn(),
    commentViewport: { scrollTop: 0, scrollHeight: 1 },
    setCommentViewport: vi.fn(),
    registerDocumentPrinter: vi.fn(),
    printDocument: vi.fn(),
    registerDocumentScroller: vi.fn(),
    scrollDocumentTo: vi.fn(),
    registerCommentRailSync: vi.fn(),
    syncCommentRail: vi.fn(),
    handleDocCommentClick: vi.fn(),
    handleRenderedDocumentClick: vi.fn(),
  };
}

async function mountFrame() {
  // The theme hook resolves "system" against the OS.
  vi.stubGlobal("matchMedia", (media: string) => ({
    media,
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <MemoryRouter>
      <QueryClientProvider client={client}>
        <DocProvider value={docContext()}>
          <RenderFrame />
        </DocProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  );
  const frame = await screen.findByTitle<HTMLIFrameElement>("Rendered document");
  const source = frame.contentWindow!;
  const toFrame = vi.spyOn(source, "postMessage").mockImplementation(() => {});
  const send = async (data: Record<string, unknown>) => {
    await act(async () => {
      window.dispatchEvent(
        new MessageEvent("message", { origin: RENDER_ORIGIN, source, data }),
      );
    });
  };
  return { toFrame, send };
}

function githubRequest(requestId: string, repo = "secrets") {
  return {
    type: "aidocs:github-request",
    requestId,
    operation: "raw",
    owner: "acme",
    repo,
    path: ".env",
    ref: "main",
  };
}

function responses(toFrame: ReturnType<typeof vi.spyOn>) {
  return (toFrame.mock.calls as unknown[][])
    .map((call) => call[0] as Record<string, unknown>)
    .filter((message) => message?.type === "aidocs:github-response");
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

it("reads a repository only after the reader approves it in the shell", async () => {
  const calls: FetchCall[] = [];
  installFetch(calls);
  const { toFrame, send } = await mountFrame();

  await send(githubRequest("req-1"));

  await screen.findByText("Let this document read acme/secrets?");
  expect(calls.some((call) => call.url.endsWith("/github/raw"))).toBe(false);

  await userEvent.click(screen.getByRole("button", { name: "Allow" }));

  await waitFor(() => expect(responses(toFrame)).toHaveLength(1));
  expect(calls.filter((call) => call.url.endsWith("/github/consents"))).toEqual([
    { url: "/v1/documents/doc_1/github/consents", method: "GET", body: undefined },
    {
      url: "/v1/documents/doc_1/github/consents",
      method: "POST",
      body: `{"owner":"acme","repo":"secrets"}`,
    },
  ]);
  expect(responses(toFrame)[0]).toMatchObject({
    requestId: "req-1",
    ok: true,
    result: { content: "STRIPE_KEY=sk_live_1" },
  });
});

it("refuses the request when the reader declines, and stops asking again", async () => {
  const calls: FetchCall[] = [];
  installFetch(calls);
  const { toFrame, send } = await mountFrame();

  await send(githubRequest("req-1"));
  await screen.findByText("Let this document read acme/secrets?");
  await userEvent.click(screen.getByRole("button", { name: "Don’t allow" }));

  await waitFor(() => expect(responses(toFrame)).toHaveLength(1));
  expect(responses(toFrame)[0]).toMatchObject({
    requestId: "req-1",
    ok: false,
    error: { code: "github_consent_required" },
  });

  // A hostile document retries; the refusal holds without another dialog.
  await send(githubRequest("req-2"));
  await waitFor(() => expect(responses(toFrame)).toHaveLength(2));
  expect(responses(toFrame)[1]).toMatchObject({ requestId: "req-2", ok: false });
  expect(screen.queryByText("Let this document read acme/secrets?")).toBeNull();
  expect(calls.some((call) => call.url.endsWith("/github/raw"))).toBe(false);
});

it("shows one dialog at a time however often the document asks", async () => {
  installFetch([]);
  const { send } = await mountFrame();

  await send(githubRequest("req-1"));
  await send(githubRequest("req-2"));
  await send(githubRequest("req-3", "docs"));

  await screen.findByText("Let this document read acme/secrets?");
  expect(screen.getAllByRole("alertdialog")).toHaveLength(1);
  expect(screen.queryByText("Let this document read acme/docs?")).toBeNull();
});

it("reports an approval that could not be saved without reading anything", async () => {
  const calls: FetchCall[] = [];
  installFetch(calls, { grantFails: true });
  const { toFrame, send } = await mountFrame();

  await send(githubRequest("req-1"));
  await screen.findByText("Let this document read acme/secrets?");
  await userEvent.click(screen.getByRole("button", { name: "Allow" }));

  await waitFor(() => expect(responses(toFrame)).toHaveLength(1));
  expect(responses(toFrame)[0]).toMatchObject({ requestId: "req-1", ok: false });
  expect(calls.some((call) => call.url.endsWith("/github/raw"))).toBe(false);

  // The approval was never recorded, so the reader is asked again rather than
  // being stuck with a refusal they did not make.
  await send(githubRequest("req-2"));
  await screen.findByText("Let this document read acme/secrets?");
});

it("rejects a malformed request without prompting", async () => {
  const calls: FetchCall[] = [];
  installFetch(calls);
  const { toFrame, send } = await mountFrame();

  await send({
    type: "aidocs:github-request",
    requestId: "req-1",
    operation: "raw",
    owner: "acme/../evil",
    repo: { toString: () => "secrets" },
    path: "../../etc/passwd",
    ref: "main",
  });

  await waitFor(() => expect(responses(toFrame)).toHaveLength(1));
  expect(responses(toFrame)[0]).toMatchObject({ requestId: "req-1", ok: false });
  expect(screen.queryByRole("alertdialog")).toBeNull();
  expect(calls.some((call) => call.url.includes("/github/"))).toBe(false);
});

it("confirms before sending the reader to GitHub", async () => {
  installFetch([]);
  // jsdom only performs same-document navigation, so the connect URL is
  // redirected to a fragment and the hash records whether it was followed.
  const connectURL = vi
    .spyOn(api, "githubConnectURL")
    .mockReturnValue("#github-connect");
  const { send } = await mountFrame();

  await send({ type: "aidocs:github-connect" });

  await screen.findByText("Connect your GitHub account?");
  expect(connectURL).not.toHaveBeenCalled();
  expect(window.location.hash).toBe("");

  await userEvent.click(screen.getByRole("button", { name: "Continue to GitHub" }));
  expect(connectURL).toHaveBeenCalledOnce();
  expect(window.location.hash).toBe("#github-connect");

  // Once dismissed, the document cannot reopen the prompt this page load.
  await send({ type: "aidocs:github-connect" });
  expect(screen.queryByText("Connect your GitHub account?")).toBeNull();
});
