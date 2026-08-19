import { expect, test, uniqueName } from "./fixtures";

// Avatar reviews against the built-in mock avatar service
// (AIDOCS_AVATAR_SERVICE_MOCK=true in the compose stack). The mock walks
// queued → running → terminal on real timers (~10s), so this spec exercises
// the genuine convening states, comment fabrication, and feedback loop.

function avatarDocumentHTML(title: string) {
  return `<!doctype html>
<html lang="en">
  <head><meta charset="utf-8"><title>${title}</title></head>
  <body>
    <main>
      <h1>${title}</h1>
      <p>Requests beyond the burst budget are shed with a retriable status code.</p>
      <p>The limiter state lives in a shared cache with a five second sliding window.</p>
      <p>Every rejected request emits a metric tagged with the calling service.</p>
    </main>
  </body>
</html>`;
}

test("requests an avatar review and renders the comments with feedback", async ({
  page,
  aidocs,
}) => {
  test.setTimeout(90_000);
  const title = uniqueName("Avatar review target");
  const doc = await aidocs.createDocument(title, avatarDocumentHTML(title));

  await page.goto(`/app/d/${doc.id}`);
  await expect(page.getByText(title)).toBeVisible();

  // The promoted Avatar button sits in the doc bar.
  const avatarButton = page.getByRole("button", { name: "Avatar review" });
  await expect(avatarButton).toBeVisible();
  await avatarButton.click();

  // The picker lists the mock roster with The Bench teaser last,
  // non-selectable and tagged Coming soon.
  const dialog = page.getByRole("dialog", { name: "Request an avatar review" });
  await expect(dialog.getByText("The Bench")).toBeVisible();
  await expect(dialog.getByText("Coming soon")).toBeVisible();

  await dialog.getByRole("button", { name: /Priya's avatar/ }).click();
  await dialog.getByRole("button", { name: "Request review" }).click();
  await expect(dialog).toBeHidden();

  // Convening: the button and the per-avatar progress line in the comments
  // panel both render the named state while the mock walks queued → running.
  await expect(avatarButton).toContainText(/Priya's avatar is reading…/, {
    timeout: 15_000,
  });
  await expect(
    page
      .getByRole("complementary")
      .getByText(/Priya's avatar is (getting ready|reading)…/),
  ).toBeVisible({ timeout: 15_000 });

  // Terminal: the mock fabricates 2-3 anchored comments.
  await expect(
    page.getByRole("complementary").getByText(/Priya's avatar found \d+ things?/),
  ).toBeVisible({ timeout: 30_000 });

  // The comments arrive through the normal pipeline with the avatar marker,
  // severity badge, and thumbs feedback.
  const avatarCard = page
    .locator("article")
    .filter({ hasText: "Priya's avatar" })
    .first();
  await expect(avatarCard).toBeVisible();
  await expect(avatarCard.getByText(/blocking|should-fix|consider/)).toBeVisible();

  const thumbsUp = avatarCard.getByRole("button", { name: "Helpful", exact: true });
  await thumbsUp.click();
  await expect(thumbsUp).toHaveAttribute("aria-pressed", "true");
  await expect(thumbsUp).toContainText("1");

  // Voting again toggles the vote off.
  await thumbsUp.click();
  await expect(thumbsUp).toHaveAttribute("aria-pressed", "false");

  // The doc bar button now reports the avatar comment count.
  await expect(page.getByRole("button", { name: "Avatar review" })).toContainText(
    /Avatar · \d+/,
  );
});
