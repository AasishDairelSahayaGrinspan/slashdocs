import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { APIError, api, currentVersionID, docID, docTitle, versionID } from "./api";
import { server } from "./test/msw";

describe("api", () => {
  it("reads JSON responses and encodes query parameters", async () => {
    server.use(
      http.get("/v1/documents", ({ request }) => {
        const url = new URL(request.url);
        expect(url.searchParams.get("q")).toBe("q3 review");
        return HttpResponse.json({
          items: [{ id: "doc_1", title: "Q3", current_version_id: "ver_1" }],
        });
      }),
    );

    await expect(api.listDocuments("q3 review")).resolves.toEqual({
      items: [{ id: "doc_1", title: "Q3", current_version_id: "ver_1" }],
    });
  });

  it("sends JSON bodies for comment, grant, and service-account mutations", async () => {
    const seen: Record<string, unknown> = {};
    server.use(
      http.post("/v1/documents/doc_1/comments", async ({ request }) => {
        seen.comment = await request.json();
        return HttpResponse.json({ id: "cmt_1", status: "open" });
      }),
      http.patch("/v1/documents/doc_1/grants/gr_1", async ({ request }) => {
        seen.grant = await request.json();
        return HttpResponse.json({ id: "gr_1", role: "editor" });
      }),
      http.post("/v1/service-accounts", async ({ request }) => {
        seen.serviceAccount = await request.json();
        return HttpResponse.json({
          id: "sa_1",
          label: "bot",
          name: "bot@example.bot",
          key: { id: "sak_1", token: "aidocs_sa_x" },
        });
      }),
    );

    await api.createComment("doc_1", "ver_1", "Fix this", "quoted", {
      prefix: "pre",
      suffix: "post",
      start_offset: 2,
      end_offset: 8,
    });
    await api.updateGrant("doc_1", "gr_1", "editor");
    await api.createServiceAccount("bot", "example.bot");

    expect(seen.comment).toEqual({
      version_id: "ver_1",
      body: "Fix this",
      anchor: {
        quote: "quoted",
        prefix: "pre",
        suffix: "post",
        dom_path: "body",
        start_offset: 2,
        end_offset: 8,
      },
    });
    expect(seen.grant).toEqual({ role: "editor" });
    expect(seen.serviceAccount).toEqual({ label: "bot", domain: "example.bot" });
  });

  it("sends multipart bodies without forcing JSON content type", async () => {
    server.use(
      http.post("/v1/documents", async ({ request }) => {
        expect(request.headers.get("content-type")).toContain("multipart/form-data");
        const body = await request.formData();
        expect(body.get("title")).toBe("Report");
        expect((body.get("file") as File).size).toBeGreaterThan(0);
        return HttpResponse.json({ id: "doc_1", current_version_id: "ver_1" });
      }),
      http.post("/v1/documents/doc_1/versions", async ({ request }) => {
        const body = await request.formData();
        expect(body.get("base_version_id")).toBe("ver_1");
        expect(body.get("change_summary")).toBe("Update copy");
        return HttpResponse.json({ id: "ver_2", number: 2 });
      }),
    );

    const file = new File(["<html></html>"], "report.html", { type: "text/html" });
    await expect(api.createDocument("Report", file)).resolves.toEqual({
      id: "doc_1",
      current_version_id: "ver_1",
    });
    await expect(
      api.createVersion("doc_1", "ver_1", "Update copy", file),
    ).resolves.toEqual({ id: "ver_2", number: 2 });
  });

  it("handles no-content responses and API error payloads", async () => {
    server.use(
      http.delete("/v1/documents/doc_1", () => new HttpResponse(null, { status: 204 })),
      http.get("/v1/documents/doc_404", () =>
        HttpResponse.json(
          { error: { code: "not_found", message: "missing document" } },
          { status: 404 },
        ),
      ),
    );

    await expect(api.deleteDocument("doc_1")).resolves.toBeUndefined();
    await expect(api.getDocument("doc_404")).rejects.toMatchObject({
      status: 404,
      code: "not_found",
      message: "missing document",
    });
  });

  it("covers URL helpers and fallback display helpers", () => {
    expect(api.loginURL("/app/d/doc_1?mode=edit")).toContain(
      "redirect=%2Fapp%2Fd%2Fdoc_1%3Fmode%3Dedit",
    );
    expect(docID({})).toBe("");
    expect(docTitle({})).toBe("Untitled");
    expect(currentVersionID({})).toBe("");
    expect(versionID({})).toBe("");
  });
});
