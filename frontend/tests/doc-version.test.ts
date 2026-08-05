import assert from "node:assert/strict";
import test from "node:test";

import { displayedVersion, versionAnnouncement } from "../src/lib/doc-version.ts";

test("follows latest when the reader has chosen nothing", () => {
  const result = displayedVersion({
    selected: undefined,
    queryVersion: undefined,
    latest: "ver_5",
  });

  assert.deepEqual(result, { version: "ver_5", pinned: false });
});

test("stays on an older version the reader deliberately picked", () => {
  const result = displayedVersion({
    selected: "ver_2",
    queryVersion: undefined,
    latest: "ver_5",
  });

  assert.deepEqual(result, { version: "ver_2", pinned: true });
});

test("treats picking the newest version as following latest, not freezing on it", () => {
  const result = displayedVersion({
    selected: "ver_5",
    queryVersion: undefined,
    latest: "ver_5",
  });

  assert.deepEqual(result, { version: "ver_5", pinned: false });
});

test("an explicit version in the URL pins, even when it is the newest", () => {
  const result = displayedVersion({
    selected: undefined,
    queryVersion: "ver_5",
    latest: "ver_5",
  });

  assert.deepEqual(result, { version: "ver_5", pinned: true });
});

test("an in-app selection overrides a version in the URL", () => {
  const result = displayedVersion({
    selected: "ver_2",
    queryVersion: "ver_4",
    latest: "ver_5",
  });

  assert.deepEqual(result, { version: "ver_2", pinned: true });
});

test("returning to latest in-app falls back to the URL pin", () => {
  const result = displayedVersion({
    selected: "ver_5",
    queryVersion: "ver_4",
    latest: "ver_5",
  });

  assert.deepEqual(result, { version: "ver_4", pinned: true });
});

test("reports no version before the document has loaded", () => {
  const result = displayedVersion({
    selected: undefined,
    queryVersion: undefined,
    latest: "",
  });

  assert.deepEqual(result, { version: "", pinned: false });
});

const versions = [
  { id: "ver_3", number: 3 },
  { id: "ver_5", number: 5 },
];

test("says nothing when there is no version yet", () => {
  const result = versionAnnouncement({
    previousLatest: undefined,
    latest: "",
    userPinned: false,
    versions,
    versionsUnavailable: false,
  });

  assert.deepEqual(result, { advance: false });
});

test("first observation establishes the baseline without announcing", () => {
  const result = versionAnnouncement({
    previousLatest: undefined,
    latest: "ver_3",
    userPinned: false,
    versions,
    versionsUnavailable: false,
  });

  assert.deepEqual(result, { announce: null, advance: true });
});

test("says nothing when the latest version has not changed", () => {
  const result = versionAnnouncement({
    previousLatest: "ver_3",
    latest: "ver_3",
    userPinned: false,
    versions,
    versionsUnavailable: false,
  });

  assert.deepEqual(result, { advance: false });
});

test("announces the new number when the server moves latest forward", () => {
  const result = versionAnnouncement({
    previousLatest: "ver_3",
    latest: "ver_5",
    userPinned: false,
    versions,
    versionsUnavailable: false,
  });

  assert.deepEqual(result, { announce: "Updated to version 5", advance: true });
});

test("tracks the change silently while the reader has pinned a version", () => {
  const result = versionAnnouncement({
    previousLatest: "ver_3",
    latest: "ver_5",
    userPinned: true,
    versions,
    versionsUnavailable: false,
  });

  assert.deepEqual(result, { announce: null, advance: true });
});

test("waits for the version list before naming a number", () => {
  const result = versionAnnouncement({
    previousLatest: "ver_3",
    latest: "ver_9",
    userPinned: false,
    versions,
    versionsUnavailable: false,
  });

  assert.deepEqual(result, { advance: false });
});

test("announces once the version list catches up", () => {
  const result = versionAnnouncement({
    previousLatest: "ver_3",
    latest: "ver_9",
    userPinned: false,
    versions: [...versions, { id: "ver_9", number: 9 }],
    versionsUnavailable: false,
  });

  assert.deepEqual(result, { announce: "Updated to version 9", advance: true });
});

test("announces without a number when the version list is unavailable", () => {
  const result = versionAnnouncement({
    previousLatest: "ver_3",
    latest: "ver_9",
    userPinned: false,
    versions,
    versionsUnavailable: true,
  });

  assert.deepEqual(result, { announce: "Updated to a newer version", advance: true });
});

test("prefers the number even when the version list later failed", () => {
  const result = versionAnnouncement({
    previousLatest: "ver_3",
    latest: "ver_5",
    userPinned: false,
    versions,
    versionsUnavailable: true,
  });

  assert.deepEqual(result, { announce: "Updated to version 5", advance: true });
});
