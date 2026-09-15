import { type FrameLocator, type Page } from "@playwright/test";
import { expect, test, uniqueName } from "./fixtures";

test.use({
  viewport: { width: 390, height: 844 },
  hasTouch: true,
  isMobile: true,
});

function renderedDocument(page: Page): FrameLocator {
  return page
    .frameLocator('iframe[title="Rendered document"]')
    .frameLocator("iframe#aidocs-doc");
}

function documentHTML(heading: string, includeTarget = true) {
  return `<!doctype html>
<html>
  <head><meta charset="utf-8"><title>${heading}</title></head>
  <body>
    <h1>${heading}</h1>
    <p>Introductory context at the top of the document.</p>
    <div style="height:1400px"></div>
    ${includeTarget ? '<p id="mobile-target">The mobile reviewer needs this exact passage to return into view.</p>' : '<p id="replacement">This version no longer contains the reviewed passage.</p>'}
    <div style="height:900px"></div>
  </body>
</html>`;
}

async function createAnchoredComment(page: Page, documentID: string) {
  const doc = renderedDocument(page);
  const target = doc.locator("#mobile-target");
  await expect(target).toBeVisible();
  await target.selectText();
  // The selection toolbar is document-positioned. At the mobile viewport its
  // button can sit outside the inner iframe's clipped viewport even though it
  // is visible in the document, so activate the same control through the DOM.
  await doc
    .getByRole("button", { name: "Comment on selected text" })
    .evaluate((button: HTMLButtonElement) => button.click());

  const composer = page.getByPlaceholder("Add a comment…");
  await expect(composer).toBeVisible();
  await composer.fill("Return me to the mobile document context.");
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
  return (await response.json()) as { id: string };
}

async function createQuotedComment(
  page: Page,
  documentID: string,
  versionID: string,
  quote: string,
  body: string,
) {
  const response = await page.request.post(`/v1/documents/${documentID}/comments`, {
    headers: { Origin: "http://localhost:8080" },
    data: {
      version_id: versionID,
      body,
      mentions: [],
      anchor: {
        quote,
        prefix: "",
        suffix: "",
        dom_path: "body",
        start_offset: 0,
        end_offset: quote.length,
      },
    },
  });
  expect(response.ok()).toBeTruthy();
  return (await response.json()) as { id: string };
}

test("returns from the mobile comment list to the highlighted document context", async ({
  page,
  aidocs,
}) => {
  const title = uniqueName("Mobile comment context");
  const document = await aidocs.createDocument(title, documentHTML(title));

  await page.goto(`/app/d/${document.id}`);
  const comment = await createAnchoredComment(page, document.id);
  const dialog = page.getByRole("dialog");
  const card = page.locator(`article[data-comment-id="${comment.id}"]`);
  await expect(card).toBeVisible();

  await dialog.getByRole("button", { name: "Close panel" }).click();
  await expect(dialog).toBeHidden();
  await renderedDocument(page)
    .locator("body")
    .evaluate((body) => {
      body.ownerDocument.defaultView?.scrollTo({ top: 0 });
    });

  await page.getByRole("button", { name: "Show comments" }).click();
  await expect(dialog).toBeVisible();
  const viewInDocument = card.getByRole("button", { name: "View in document" });
  await expect(viewInDocument).toBeEnabled();
  await viewInDocument.click();

  await expect(dialog).toBeHidden();
  const activeHighlight = renderedDocument(page).locator("mark.aidocs-mark-active");
  await expect(activeHighlight.first()).toBeInViewport();
  const tray = page.locator('[aria-label="Selected comment"]');
  await expect(tray).toContainText("Return me to the mobile document context.");

  await tray.getByRole("button", { name: "Open thread" }).click();
  await expect(dialog).toBeVisible();
  await expect(card).toBeInViewport();
  await expect(card).toBeFocused();

  await renderedDocument(page)
    .locator("body")
    .evaluate((body) => {
      body.ownerDocument.defaultView?.scrollTo({ top: 0 });
    });
  await card.getByRole("button", { name: "View in document" }).click();
  await expect(dialog).toBeHidden();
  await expect(activeHighlight.first()).toBeInViewport();

  await tray.getByRole("button", { name: "Open thread" }).click();
  await card.getByRole("button", { name: "Reply" }).click();
  await expect(dialog).toBeVisible();
  const beforeBack = page.url();
  await page.goBack();
  await expect(dialog).toBeHidden();
  expect(page.url()).toBe(beforeBack);
});

test("opens unavailable mobile comment links in the focused sheet", async ({
  page,
  aidocs,
}) => {
  const title = uniqueName("Mobile unavailable comment");
  const document = await aidocs.createDocument(title, documentHTML(title));

  await page.goto(`/app/d/${document.id}`);
  const comment = await createAnchoredComment(page, document.id);
  await aidocs.pushVersion(document.id, documentHTML(title, false));

  await page.goto(`/app/d/${document.id}?comment=${comment.id}`);
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible({ timeout: 20_000 });
  const card = page.locator(`article[data-comment-id="${comment.id}"]`);
  await expect(card).toContainText(
    "This highlighted passage is not visible in this version.",
  );
  await card.click();
  await expect(dialog).toBeVisible();
});

test("reveals anchored mobile comment links directly in the document", async ({
  page,
  aidocs,
}) => {
  const title = uniqueName("Mobile anchored comment link");
  const document = await aidocs.createDocument(title, documentHTML(title));

  await page.goto(`/app/d/${document.id}`);
  const comment = await createAnchoredComment(page, document.id);
  await page.goto(`/app/d/${document.id}?comment=${comment.id}`);

  await expect(page.getByRole("dialog")).toBeHidden();
  await expect(
    renderedDocument(page).locator("mark.aidocs-mark-active").first(),
  ).toBeInViewport({
    timeout: 20_000,
  });
  await expect(page.locator('[aria-label="Selected comment"]')).toContainText(
    "Return me to the mobile document context.",
  );
});

test("clears the mobile context when a comment is deleted during the return", async ({
  page,
  aidocs,
}) => {
  const title = uniqueName("Mobile deleted comment");
  const document = await aidocs.createDocument(title, documentHTML(title));

  await page.goto(`/app/d/${document.id}`);
  const comment = await createAnchoredComment(page, document.id);
  const card = page.locator(`article[data-comment-id="${comment.id}"]`);

  await card.getByRole("button", { name: "View in document" }).click();
  const response = await page.request.delete(
    `/v1/documents/${document.id}/comments/${comment.id}`,
    { headers: { Origin: "http://localhost:8080" } },
  );
  expect(response.ok()).toBeTruthy();

  await expect(page.getByRole("dialog")).toBeHidden();
  await expect(
    page.getByText("That comment was deleted before its passage could be shown."),
  ).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('[aria-label="Selected comment"]')).toHaveCount(0);
  await expect(renderedDocument(page).locator("mark.aidocs-mark-active")).toHaveCount(
    0,
  );

  await page.getByRole("button", { name: "Show comments" }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(card).toHaveCount(0);
});

test("reveals the mobile passage without animation when reduced motion is preferred", async ({
  page,
  aidocs,
}) => {
  const title = uniqueName("Mobile reduced motion");
  const document = await aidocs.createDocument(title, documentHTML(title));

  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(`/app/d/${document.id}`);
  const comment = await createAnchoredComment(page, document.id);
  const dialog = page.getByRole("dialog");
  const card = page.locator(`article[data-comment-id="${comment.id}"]`);
  await dialog.getByRole("button", { name: "Close panel" }).click();
  await renderedDocument(page)
    .locator("body")
    .evaluate((body) => {
      body.ownerDocument.defaultView?.scrollTo({ top: 0 });
    });

  await page.getByRole("button", { name: "Show comments" }).click();
  await card.getByRole("button", { name: "View in document" }).click();

  await expect(dialog).toBeHidden();
  await expect(
    renderedDocument(page).locator("mark.aidocs-mark-active").first(),
  ).toBeInViewport();
  await expect(page.locator('[aria-label="Selected comment"]')).toContainText(
    "Return me to the mobile document context.",
  );
});

test("changes context while scrolling but keeps same-paragraph highlights stable", async ({
  page,
  aidocs,
}) => {
  const title = uniqueName("Mobile grouped comment context");
  const html = `<!doctype html>
<html>
  <head><meta charset="utf-8"><title>${title}</title></head>
  <body>
    <h1>${title}</h1>
    <div style="height:700px"></div>
    <p id="shared">The reviewer validates the deployment plan, confirms the rollback owner, and records the final approval.</p>
    <div style="height:900px"></div>
    <p id="later">Browser Back must keep document scrolling responsive after the comments sheet closes.</p>
    <div style="height:900px"></div>
  </body>
</html>`;
  const document = await aidocs.createDocument(title, html);
  const first = await createQuotedComment(
    page,
    document.id,
    document.current_version_id,
    "validates the deployment plan",
    "Same paragraph first comment",
  );
  const second = await createQuotedComment(
    page,
    document.id,
    document.current_version_id,
    "confirms the rollback owner",
    "Same paragraph second comment",
  );
  await createQuotedComment(
    page,
    document.id,
    document.current_version_id,
    "records the final approval",
    "Same paragraph third comment",
  );
  const later = await createQuotedComment(
    page,
    document.id,
    document.current_version_id,
    "document scrolling responsive",
    "Later document comment",
  );

  await page.goto(`/app/d/${document.id}?comment=${first.id}`);
  const tray = page.locator('[aria-label="Selected comment"]');
  await expect(tray).toContainText("Same paragraph first comment", {
    timeout: 20_000,
  });
  await expect(tray).toContainText("1/3");

  const nextButton = tray.getByRole("button", {
    name: "Next comment in this paragraph",
  });
  expect(
    await nextButton.evaluate((button) => button.getBoundingClientRect().height),
  ).toBeGreaterThanOrEqual(44);
  await nextButton.click();
  await expect(tray).toContainText("Same paragraph second comment");
  await expect(tray).toContainText("2/3");
  await expect(
    renderedDocument(page).locator(`mark[data-cid="${second.id}"].aidocs-mark-active`),
  ).toHaveCount(1);

  await page.waitForTimeout(950);
  await renderedDocument(page)
    .locator("#shared")
    .evaluate((paragraph) => {
      paragraph.ownerDocument.defaultView?.scrollBy({ top: 40 });
    });
  await expect(tray).toContainText("Same paragraph second comment");

  await renderedDocument(page)
    .locator("#later")
    .evaluate((paragraph) => {
      paragraph.scrollIntoView({ block: "center" });
    });
  await expect(tray).toContainText("Later document comment", { timeout: 10_000 });
  await expect(
    renderedDocument(page).locator(`mark[data-cid="${later.id}"].aidocs-mark-active`),
  ).toHaveCount(1);
  await expect(renderedDocument(page).locator("#later")).toBeInViewport();

  await tray.getByRole("button", { name: "Dismiss selected comment" }).click();
  await renderedDocument(page)
    .locator("#shared")
    .evaluate((paragraph) => {
      paragraph.scrollIntoView({ block: "center" });
    });
  await expect(tray).toHaveCount(0);

  await page.getByRole("button", { name: "Show comments" }).click();
  const secondCard = page.locator(`article[data-comment-id="${second.id}"]`);
  await secondCard.getByRole("button", { name: "Reply" }).click();
  await expect(secondCard.getByPlaceholder("Reply…")).toBeFocused();
});
