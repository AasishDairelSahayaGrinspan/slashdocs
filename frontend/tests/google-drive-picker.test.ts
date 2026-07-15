import assert from "node:assert/strict";
import test from "node:test";

import { googleDriveDocumentLocation } from "../src/lib/google-drive-picker.ts";

test("Drive Picker keeps the canonical selected URL", () => {
  assert.equal(
    googleDriveDocumentLocation({
      id: "file-id",
      url: "https://docs.google.com/document/d/file-id/edit",
    }),
    "https://docs.google.com/document/d/file-id/edit",
  );
});

test("Drive Picker falls back to an encoded file ID", () => {
  assert.equal(
    googleDriveDocumentLocation({ id: "file id/with slash" }),
    "https://drive.google.com/open?id=file%20id%2Fwith%20slash",
  );
  assert.equal(googleDriveDocumentLocation({}), "");
});
