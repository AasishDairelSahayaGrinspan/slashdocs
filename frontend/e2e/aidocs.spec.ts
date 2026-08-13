import { type FrameLocator, type Locator, type Page } from "@playwright/test";
import { APP_ORIGIN, RENDER_ORIGIN, expect, test, uniqueName } from "./fixtures";

function documentHTML(heading: string, body = "Select this sentence for review.") {
  return `<!doctype html>
<html lang="en">
  <head><meta charset="utf-8"><title>${heading}</title></head>
  <body>
    <main>
      <h1 id="e2e-heading">${heading}</h1>
      <p id="comment-target">${body}</p>
    </main>
  </body>
</html>`;
}

function renderedDocument(page: Page): FrameLocator {
  return page
    .frameLocator('iframe[title="Rendered document"]')
    .frameLocator("iframe#aidocs-doc");
}

// The composer mirrors to storage in a passive effect after paint, so wait for
// the write to land rather than racing the next reload or unmount.
function storedDrafts(page: Page) {
  return expect.poll(() =>
    page.evaluate(() => window.localStorage.getItem("aidocs.comment-drafts")),
  );
}

function entryRow(page: Page, title: string): Locator {
  return page
    .locator("tbody tr")
    .filter({ has: page.getByRole("link", { name: title, exact: true }) });
}

async function dragToDndKit(page: Page, source: Locator, target: Locator) {
  await source.scrollIntoViewIfNeeded();
  await target.scrollIntoViewIfNeeded();
  const sourceBox = await source.boundingBox();
  const targetBox = await target.boundingBox();
  if (!sourceBox || !targetBox) throw new Error("drag source or target is not visible");

  const sourcePoint = {
    x: sourceBox.x + sourceBox.width / 2,
    y: sourceBox.y + sourceBox.height / 2,
  };
  const targetPoint = {
    x: targetBox.x + targetBox.width / 2,
    y: targetBox.y + targetBox.height / 2,
  };
  await page.mouse.move(sourcePoint.x, sourcePoint.y);
  await page.mouse.down();
  // dnd-kit's PointerSensor activates after eight pixels of movement.
  await page.mouse.move(sourcePoint.x + 12, sourcePoint.y, { steps: 3 });
  await page.mouse.move(targetPoint.x, targetPoint.y, { steps: 12 });
  await page.mouse.up();
}

test("the saved browser session came from a complete Dex login", async ({ page }) => {
  await page.goto("/app/documents");
  await expect(page).toHaveURL(`${APP_ORIGIN}/app/documents`);
  await expect(page.getByRole("heading", { name: "Documents" })).toBeVisible();

  const response = await page.request.get("/v1/me");
  expect(response.ok()).toBeTruthy();
  const me = (await response.json()) as {
    principal: { email?: string };
    user?: { email?: string };
  };
  expect(me.user?.email ?? me.principal.email).toBe("admin@example.com");
});

test("uploads HTML in the UI and renders it only on the render origin", async ({
  page,
  aidocs,
}) => {
  const title = uniqueName("Browser upload");
  await page.goto("/app/documents");
  await page.getByRole("button", { name: "More" }).click();
  await page.getByRole("menuitem", { name: "Upload HTML (backup)" }).click();

  const dialog = page.getByRole("dialog", { name: "Upload HTML (backup)" });
  await dialog.getByPlaceholder("Title").fill(title);
  await dialog.locator('input[type="file"]').setInputFiles({
    name: "browser-upload.html",
    mimeType: "text/html",
    buffer: Buffer.from(documentHTML(title)),
  });
  await dialog.getByRole("button", { name: "Upload", exact: true }).click();

  await expect(page).toHaveURL(/\/app\/d\/doc_[^/?]+$/);
  const documentID = new URL(page.url()).pathname.split("/").at(-1);
  expect(documentID).toBeTruthy();
  aidocs.trackDocument(documentID!);

  const outerFrame = page.locator('iframe[title="Rendered document"]');
  await expect(outerFrame).toBeVisible();
  await expect(outerFrame).toHaveAttribute(
    "src",
    new RegExp(`^${RENDER_ORIGIN.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}/v/`),
  );
  await expect(
    renderedDocument(page).getByRole("heading", { name: title }),
  ).toBeVisible();
});

test("creates a structured mention with a clean API body", async ({ page, aidocs }) => {
  const title = uniqueName("Structured mention");
  const document = await aidocs.createDocument(title, documentHTML(title));

  await page.goto(`/app/d/${document.id}`);
  const selectedText = renderedDocument(page).locator("#comment-target");
  await expect(selectedText).toBeVisible();
  await selectedText.selectText();

  const composer = page.getByPlaceholder("Add a comment…");
  await expect(composer).toBeVisible();
  await composer.fill("Please review @ad");
  await page.getByRole("button", { name: /admin@example\.com/ }).click();
  await expect(composer).toHaveValue("Please review @admin ");
  // Accepting a suggestion focuses the composer and places the caret after
  // the inserted mention in the same commit (layout effect), so once the
  // value assertion above passes it is safe to keep typing.
  await composer.pressSequentially("when you can");

  const commentResponsePromise = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return (
      response.request().method() === "POST" &&
      url.pathname === `/v1/documents/${document.id}/comments`
    );
  });
  await page.getByRole("button", { name: "Comment ⌘↵" }).click();
  const commentResponse = await commentResponsePromise;
  expect(commentResponse.ok()).toBeTruthy();
  const comment = (await commentResponse.json()) as {
    body: string;
    mentions: Array<{
      principal: { type: string; id: string };
      offset: number;
      length: number;
    }>;
  };
  expect(comment.body).toBe("Please review @admin when you can");
  expect(comment.body).not.toContain("\u2063");
  expect(comment.mentions).toHaveLength(1);
  expect(
    comment.body.slice(
      comment.mentions[0].offset,
      comment.mentions[0].offset + comment.mentions[0].length,
    ),
  ).toBe("@admin");

  const commentCard = page
    .getByRole("button")
    .filter({ hasText: "Please review" })
    .filter({ hasText: "@admin" });
  const renderedMention = commentCard.getByText("@admin", { exact: true });
  await expect(renderedMention).toBeVisible();
  await expect(renderedMention).toHaveClass(/bg-\[var\(--color-accent-muted\)\]/);
});

test("keeps an unsent comment draft across a reload", async ({ page, aidocs }) => {
  const title = uniqueName("Draft survives reload");
  const document = await aidocs.createDocument(title, documentHTML(title));

  await page.goto(`/app/d/${document.id}`);
  const selectedText = renderedDocument(page).locator("#comment-target");
  await expect(selectedText).toBeVisible();
  await selectedText.selectText();

  const composer = page.getByPlaceholder("Add a comment…");
  await expect(composer).toBeVisible();
  // A composer opened by a live selection takes focus, so typing lands in it.
  await expect(composer).toBeFocused();
  await composer.fill("half-written thought");
  await storedDrafts(page).toContain("half-written thought");

  await page.reload();
  const restored = page.getByPlaceholder("Add a comment…");
  await expect(restored).toBeVisible();
  await expect(restored).toHaveValue("half-written thought");
  // A restored draft must not, or scroll keys would go to the textarea instead
  // of the document the reader came back to read.
  await expect(restored).not.toBeFocused();
  await expect(page.getByText("Select this sentence for review.")).toBeVisible();

  // Cancelling has to drop the stored copy too, not just close the composer.
  await page.getByRole("button", { name: "Cancel" }).click();
  await expect(restored).toBeHidden();
  await storedDrafts(page).toBe("{}");
  await page.reload();
  await expect(page.getByPlaceholder("Add a comment…")).toBeHidden();
});

test("keeps an unsent comment draft while the panel is closed", async ({
  page,
  aidocs,
}) => {
  const title = uniqueName("Draft survives panel");
  const document = await aidocs.createDocument(title, documentHTML(title));

  await page.goto(`/app/d/${document.id}`);
  const selectedText = renderedDocument(page).locator("#comment-target");
  await expect(selectedText).toBeVisible();
  await selectedText.selectText();

  const composer = page.getByPlaceholder("Add a comment…");
  await expect(composer).toBeVisible();
  await composer.fill("thought parked mid-sentence");
  await storedDrafts(page).toContain("thought parked mid-sentence");

  // Closing the panel unmounts Comments entirely, which is a different loss
  // path from a reload.
  await page.getByRole("button", { name: "Hide comments" }).click();
  await expect(composer).toBeHidden();
  await page.getByRole("button", { name: "Show comments" }).click();

  const restored = page.getByPlaceholder("Add a comment…");
  await expect(restored).toBeVisible();
  await expect(restored).toHaveValue("thought parked mid-sentence");
});

test("clears the stored draft once the comment is posted", async ({ page, aidocs }) => {
  const title = uniqueName("Draft cleared on post");
  const document = await aidocs.createDocument(title, documentHTML(title));

  await page.goto(`/app/d/${document.id}`);
  const selectedText = renderedDocument(page).locator("#comment-target");
  await expect(selectedText).toBeVisible();
  await selectedText.selectText();

  const composer = page.getByPlaceholder("Add a comment…");
  await expect(composer).toBeVisible();
  await composer.fill("posted for real");
  await storedDrafts(page).toContain("posted for real");

  const commentResponsePromise = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return (
      response.request().method() === "POST" &&
      url.pathname === `/v1/documents/${document.id}/comments`
    );
  });
  await page.getByRole("button", { name: "Comment ⌘↵" }).click();
  expect((await commentResponsePromise).ok()).toBeTruthy();
  await storedDrafts(page).toBe("{}");

  await page.reload();
  await expect(page.getByPlaceholder("Add a comment…")).toBeHidden();
  await expect(
    page.getByRole("button").filter({ hasText: "posted for real" }),
  ).toBeVisible();
});

test("clears the stored draft on sign out", async ({ page, aidocs }) => {
  const title = uniqueName("Draft cleared on sign out");
  const document = await aidocs.createDocument(title, documentHTML(title));

  await page.goto(`/app/d/${document.id}`);
  const selectedText = renderedDocument(page).locator("#comment-target");
  await expect(selectedText).toBeVisible();
  await selectedText.selectText();

  const composer = page.getByPlaceholder("Add a comment…");
  await expect(composer).toBeVisible();
  await composer.fill("should not outlive this session");
  await storedDrafts(page).toContain("should not outlive this session");

  // localStorage is scoped to the origin, not the account, so signing out has
  // to erase the draft before the next person signs in on this browser.
  await page.getByRole("button", { name: "Account menu" }).click();
  await page.getByRole("menuitem", { name: "Sign out" }).click();
  await storedDrafts(page).toBe("{}");
});

test("restores a draft mention that still posts a valid offset", async ({
  page,
  aidocs,
}) => {
  const title = uniqueName("Draft mention survives reload");
  const document = await aidocs.createDocument(title, documentHTML(title));

  await page.goto(`/app/d/${document.id}`);
  const selectedText = renderedDocument(page).locator("#comment-target");
  await expect(selectedText).toBeVisible();
  await selectedText.selectText();

  const composer = page.getByPlaceholder("Add a comment…");
  await expect(composer).toBeVisible();
  await composer.fill("Please review @ad");
  await page.getByRole("button", { name: /admin@example\.com/ }).click();
  await expect(composer).toHaveValue("Please review @admin ");
  await storedDrafts(page).toContain("@admin");

  await page.reload();
  const restored = page.getByPlaceholder("Add a comment…");
  await expect(restored).toBeVisible();
  await expect(restored).toHaveValue("Please review @admin ");

  // The mention offsets are stored separately from the body, so the round trip
  // has to keep them pointing at the same span of text.
  const commentResponsePromise = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return (
      response.request().method() === "POST" &&
      url.pathname === `/v1/documents/${document.id}/comments`
    );
  });
  await page.getByRole("button", { name: "Comment ⌘↵" }).click();
  const commentResponse = await commentResponsePromise;
  expect(commentResponse.ok()).toBeTruthy();
  const comment = (await commentResponse.json()) as {
    body: string;
    mentions: Array<{ offset: number; length: number }>;
  };
  expect(comment.mentions).toHaveLength(1);
  expect(
    comment.body.slice(
      comment.mentions[0].offset,
      comment.mentions[0].offset + comment.mentions[0].length,
    ),
  ).toBe("@admin");
});

test("shares a document through its general access role", async ({ page, aidocs }) => {
  const title = uniqueName("General access");
  const document = await aidocs.createDocument(title, documentHTML(title));

  await page.goto(`/app/d/${document.id}`);
  await page.getByRole("button", { name: "Share", exact: true }).click();
  const shareSheet = page.getByRole("dialog", { name: "Share document" });
  const generalAccess = shareSheet.getByLabel("General access role");
  await generalAccess.selectOption("viewer");
  await expect(generalAccess).toHaveValue("viewer");
  const access = await aidocs.documentAccess(document.id);
  expect(
    access.some(
      (item) => item.principal.type === "anyone" && item.effective_role === "viewer",
    ),
  ).toBeTruthy();
});

test("moves a document into a folder and returns it to root when deleting the folder", async ({
  page,
  aidocs,
}) => {
  const documentTitle = uniqueName("Folder document");
  const folderTitle = uniqueName("Browser folder");
  const document = await aidocs.createDocument(
    documentTitle,
    documentHTML(documentTitle),
  );

  await page.goto("/app/documents");
  await page.getByRole("button", { name: "New folder" }).click();
  const newFolderDialog = page.getByRole("dialog", { name: "New folder" });
  await newFolderDialog.getByPlaceholder("Folder name").fill(folderTitle);
  const createFolderResponsePromise = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return response.request().method() === "POST" && url.pathname === "/v1/folders";
  });
  await newFolderDialog.getByRole("button", { name: "Create" }).click();
  const createFolderResponse = await createFolderResponsePromise;
  expect(createFolderResponse.ok()).toBeTruthy();
  const createdFolder = (await createFolderResponse.json()) as { id: string };
  expect(createdFolder.id).toMatch(/^fld_/);
  const folderID = createdFolder.id;
  aidocs.trackFolder(folderID);
  await expect(entryRow(page, folderTitle)).toBeVisible();

  const documentRow = entryRow(page, documentTitle);
  const folderRow = entryRow(page, folderTitle);
  await dragToDndKit(page, documentRow, folderRow);
  await expect(documentRow).toBeHidden();

  await folderRow.getByRole("link", { name: folderTitle, exact: true }).click();
  await expect(page.getByRole("heading", { name: folderTitle })).toBeVisible();
  await expect(entryRow(page, documentTitle)).toBeVisible();

  await page.goto("/app/documents");
  const rootFolderRow = entryRow(page, folderTitle);
  await rootFolderRow.hover();
  await rootFolderRow.getByRole("button", { name: "Delete folder" }).click();
  const deleteDialog = page.getByRole("alertdialog");
  await expect(deleteDialog).toContainText(folderTitle);
  const deleteResponsePromise = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return (
      response.request().method() === "DELETE" &&
      url.pathname === `/v1/folders/${folderID}`
    );
  });
  await deleteDialog.getByRole("button", { name: "Delete", exact: true }).click();
  const deleteResponse = await deleteResponsePromise;
  expect(deleteResponse.ok()).toBeTruthy();
  aidocs.untrackFolder(folderID);

  await expect(entryRow(page, folderTitle)).toBeHidden();
  await expect(entryRow(page, documentTitle)).toBeVisible();
  const movedDocument = await aidocs.getDocument(document.id);
  expect(movedDocument.parent_folder_id).toBeFalsy();
});

test("delivers a KV patch to a second open page through the browser SSE stream", async ({
  page,
  context,
  aidocs,
}) => {
  const title = uniqueName("Live KV");
  const key = `live_${crypto.randomUUID().replaceAll("-", "")}`;
  const value = uniqueName("received");
  const html = `<!doctype html>
<html lang="en">
  <head><meta charset="utf-8"><title>${title}</title></head>
  <body>
    <h1>${title}</h1>
    <output id="live-value">waiting</output>
    <script>
      const output = document.getElementById("live-value");
      async function renderValue() {
        const value = await window.aidocs.kv.get(${JSON.stringify(key)});
        output.textContent = value === undefined ? "ready" : String(value);
      }
      window.aidocs.on("kvchange", renderValue);
      renderValue();
    </script>
  </body>
</html>`;
  const document = await aidocs.createDocument(title, html);
  const secondPage = await context.newPage();
  const secondPageStream = secondPage.waitForResponse((response) => {
    const url = new URL(response.url());
    return (
      response.status() === 200 &&
      url.pathname === `/v1/documents/${document.id}/kv/events`
    );
  });

  await page.goto(`/app/d/${document.id}`);
  await secondPage.goto(`/app/d/${document.id}`);
  const firstOutput = renderedDocument(page).locator("#live-value");
  const secondOutput = renderedDocument(secondPage).locator("#live-value");
  await expect(firstOutput).toHaveText("ready");
  await expect(secondOutput).toHaveText("ready");
  await secondPageStream;

  await aidocs.patchKV(document.id, key, value);
  await expect(secondOutput).toHaveText(value);
});
