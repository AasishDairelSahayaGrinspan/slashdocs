import { type FrameLocator, type Page } from "@playwright/test";
import { expect, test, uniqueName } from "./fixtures";

function renderedDocument(page: Page): FrameLocator {
  return page
    .frameLocator('iframe[title="Rendered document"]')
    .frameLocator("iframe#aidocs-doc");
}

function documentHTML(heading: string, body: string) {
  return `<!doctype html>
<html>
  <head><meta charset="utf-8"><title>${heading}</title></head>
  <body>
    <h1>${heading}</h1>
    ${body}
  </body>
</html>`;
}

type CommentRecord = {
  id: string;
  body: string;
  selected_text: string;
  anchor: {
    quote: string;
    prefix: string;
    suffix: string;
    dom_path: string;
    start_offset: number;
    end_offset: number;
  };
};

async function postComment(
  page: Page,
  documentID: string,
  body: string,
): Promise<CommentRecord> {
  const composer = page.getByPlaceholder("Add a comment…");
  await renderedDocument(page)
    .getByRole("button", { name: "Comment on selected text" })
    .click();
  await expect(composer).toBeVisible();
  await composer.fill(body);
  const responsePromise = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return (
      response.request().method() === "POST" &&
      url.pathname === `/v1/documents/${documentID}/comments`
    );
  });
  await page.getByRole("button", { name: "Post comment" }).click();
  const response = await responsePromise;
  expect(response.ok()).toBeTruthy();
  return (await response.json()) as CommentRecord;
}

test("highlights a selection spanning inline markup end to end", async ({
  page,
  aidocs,
}) => {
  const title = uniqueName("Cross-element anchor");
  const document = await aidocs.createDocument(
    title,
    documentHTML(
      title,
      '<p id="rich">The gateway returned a <b>stale connection</b> error and the <code>conn_pool</code> metric spiked.</p>',
    ),
  );

  await page.goto(`/app/d/${document.id}`);
  const doc = renderedDocument(page);
  const paragraph = doc.locator("#rich");
  await expect(paragraph).toBeVisible();
  await paragraph.selectText();

  const comment = await postComment(
    page,
    document.id,
    "This selection crosses inline elements.",
  );
  expect(comment.anchor.end_offset).toBeGreaterThan(0);
  expect(comment.anchor.end_offset).toBeGreaterThan(comment.anchor.start_offset);
  expect(comment.anchor.quote).toContain("stale connection");

  const marks = doc.locator("mark.aidocs-mark");
  await expect(marks.first()).toBeVisible();
  const paragraphText = (await paragraph.innerText()).replace(/\s+/g, " ").trim();
  await expect
    .poll(async () =>
      (await marks.allTextContents()).join("").replace(/\s+/g, " ").trim(),
    )
    .toBe(paragraphText);
});

test("re-anchors a comment on content rendered by script after load", async ({
  page,
  aidocs,
}) => {
  const title = uniqueName("Late content anchor");
  const document = await aidocs.createDocument(
    title,
    documentHTML(
      title,
      `<p>Static intro paragraph.</p>
    <script>setTimeout(() => { const p = document.createElement('p'); p.id = 'late'; p.textContent = 'Late content appears after load.'; document.body.appendChild(p); }, 800);</script>`,
    ),
  );

  await page.goto(`/app/d/${document.id}`);
  const doc = renderedDocument(page);
  const late = doc.locator("#late");
  // The document's own script inserts this paragraph ~800ms after load.
  await expect(late).toBeVisible({ timeout: 15_000 });
  await late.selectText();
  await postComment(page, document.id, "Anchor survives late rendering.");

  await page.reload();

  // After the reload the paragraph does not exist yet when comments paint;
  // the bridge must re-anchor on its own once the script inserts it.
  const mark = renderedDocument(page)
    .locator("mark.aidocs-mark")
    .filter({ hasText: "Late content appears after load." });
  await expect(mark.first()).toBeVisible({ timeout: 20_000 });

  const card = page
    .getByRole("button")
    .filter({ hasText: "Anchor survives late rendering." });
  await expect(card).toBeVisible();
  await expect(
    card.getByText("highlight not found", { exact: true }),
  ).not.toBeVisible();
});

test("flags a comment whose text vanished from the new version", async ({
  page,
  aidocs,
}) => {
  const title = uniqueName("Orphaned anchor");
  const document = await aidocs.createDocument(
    title,
    documentHTML(
      title,
      '<p id="target">The quarterly latency regression traces back to the cache eviction policy.</p>',
    ),
  );

  await page.goto(`/app/d/${document.id}`);
  const doc = renderedDocument(page);
  const target = doc.locator("#target");
  await expect(target).toBeVisible();
  await target.selectText();
  await postComment(page, document.id, "This anchor is about to be orphaned.");

  // Replace the document with entirely unrelated prose so not even fuzzy
  // matching can rescue the original quote.
  await aidocs.pushVersion(
    document.id,
    documentHTML(
      title,
      '<p id="replacement">Bees navigate home by polarized sky light.</p>',
    ),
  );
  await page.reload();

  await expect(renderedDocument(page).locator("#replacement")).toBeVisible();

  const card = page
    .getByRole("button")
    .filter({ hasText: "This anchor is about to be orphaned." });
  await expect(card).toBeVisible();
  await expect(card.getByText("highlight not found", { exact: true })).toBeVisible({
    timeout: 20_000,
  });
  await expect(renderedDocument(page).locator("mark.aidocs-mark")).toHaveCount(0);
});
