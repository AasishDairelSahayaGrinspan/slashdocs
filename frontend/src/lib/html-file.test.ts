import { describe, expect, it } from "vitest";
import { EDITED_HTML_FILENAME, htmlToFile } from "@/lib/html-file";

describe("htmlToFile", () => {
  it("names the file so the server sees an HTML upload", () => {
    const file = htmlToFile("<p>hi</p>");
    expect(file.name).toBe(EDITED_HTML_FILENAME);
    expect(file.type).toBe("text/html");
  });

  it("preserves the HTML byte-for-byte", async () => {
    const html =
      "<!doctype html><html><body>\n  <script>let a = 1 < 2;</script>\n</body></html>";
    const file = htmlToFile(html);
    await expect(file.text()).resolves.toBe(html);
  });

  it("preserves non-ascii content", async () => {
    const html = "<p>₹1,200 — naïve</p>";
    await expect(htmlToFile(html).text()).resolves.toBe(html);
  });

  it("accepts an empty string", async () => {
    await expect(htmlToFile("").text()).resolves.toBe("");
  });
});
