// frontend/src/lib/drive.test.ts
import { describe, it, expect, vi } from "vitest";
import { fetchDriveFile, DriveAccessError } from "./drive";

describe("fetchDriveFile", () => {
  it("returns HTML on 200", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
      new Response("<h1>ok</h1>", { status: 200 }),
    ));
    expect(await fetchDriveFile("f1", "tok")).toBe("<h1>ok</h1>");
  });
  it("throws DriveAccessError with status on 403", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 403 })));
    await expect(fetchDriveFile("f1", "tok")).rejects.toMatchObject({ status: 403 });
  });
});
