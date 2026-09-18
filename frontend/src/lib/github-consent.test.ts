import { describe, expect, it, vi } from "vitest";
import {
  GitHubConsentGate,
  normalizeRepoRef,
  parseGitHubRawRequest,
  repoKey,
  type GitHubRepoRef,
} from "./github-consent";

const acmeDocs: GitHubRepoRef = { owner: "acme", repo: "docs" };
const acmeSecrets: GitHubRepoRef = { owner: "acme", repo: "secrets" };

function newGate(maxQueuedRepos?: number) {
  const onPrompt = vi.fn<(repository: GitHubRepoRef) => void>();
  const gate = new GitHubConsentGate({ onPrompt, maxQueuedRepos });
  return { gate, onPrompt };
}

describe("normalizeRepoRef", () => {
  it("lowercases valid coordinates", () => {
    expect(normalizeRepoRef("ACME", "Docs.v2_final-1")).toEqual({
      owner: "acme",
      repo: "docs.v2_final-1",
    });
    expect(normalizeRepoRef("a-b-c", "d")).toEqual({ owner: "a-b-c", repo: "d" });
  });

  it.each([
    ["a slashed owner", "acme/evil", "docs"],
    ["consecutive hyphens", "ac--me", "docs"],
    ["a leading hyphen", "-acme", "docs"],
    ["a trailing hyphen", "acme-", "docs"],
    ["an over-long owner", "a".repeat(40), "docs"],
    ["an empty owner", "", "docs"],
    ["a slashed repo", "acme", "docs/evil"],
    ["a dot repo", "acme", "."],
    ["a dot-dot repo", "acme", ".."],
    ["a spaced repo", "acme", "do cs"],
    ["an over-long repo", "acme", "d".repeat(101)],
    ["an empty repo", "acme", ""],
  ])("rejects %s", (_name, owner, repo) => {
    expect(normalizeRepoRef(owner, repo)).toBeNull();
  });

  it("rejects values that are not strings", () => {
    expect(normalizeRepoRef(undefined, "docs")).toBeNull();
    expect(normalizeRepoRef({ toString: () => "acme" }, "docs")).toBeNull();
    expect(normalizeRepoRef("acme", 7)).toBeNull();
  });
});

describe("parseGitHubRawRequest", () => {
  it("accepts a well-formed request", () => {
    expect(
      parseGitHubRawRequest({
        operation: "raw",
        owner: "Acme",
        repo: "Docs",
        path: "docs/architecture.html",
        ref: "main",
      }),
    ).toEqual({ repository: acmeDocs, path: "docs/architecture.html", ref: "main" });
  });

  it.each([
    ["another operation", { operation: "write" }],
    ["a missing path", { operation: "raw", owner: "acme", repo: "docs", ref: "main" }],
    [
      "a traversing path",
      { operation: "raw", owner: "acme", repo: "docs", path: "../.env", ref: "main" },
    ],
    [
      "an absolute path",
      {
        operation: "raw",
        owner: "acme",
        repo: "docs",
        path: "/etc/passwd",
        ref: "main",
      },
    ],
    [
      "an over-long path",
      {
        operation: "raw",
        owner: "acme",
        repo: "docs",
        path: "a".repeat(1025),
        ref: "main",
      },
    ],
    [
      "an over-long ref",
      {
        operation: "raw",
        owner: "acme",
        repo: "docs",
        path: "README.md",
        ref: "m".repeat(256),
      },
    ],
    [
      "a non-string ref",
      { operation: "raw", owner: "acme", repo: "docs", path: "README.md", ref: 3 },
    ],
  ])("rejects %s", (_name, data) => {
    expect(parseGitHubRawRequest(data as Record<string, unknown>)).toBeNull();
  });
});

describe("GitHubConsentGate", () => {
  it("prompts once for an unknown repository and allows it after approval", async () => {
    const { gate, onPrompt } = newGate();
    const decision = gate.request(acmeDocs);

    expect(onPrompt).toHaveBeenCalledExactlyOnceWith(acmeDocs);
    expect(gate.prompt).toEqual(acmeDocs);
    gate.settle("allowed");

    await expect(decision).resolves.toBe("allowed");
    expect(gate.prompt).toBeNull();
    // An approved repository never prompts again.
    await expect(gate.request(acmeDocs)).resolves.toBe("allowed");
    expect(onPrompt).toHaveBeenCalledOnce();
  });

  it("does not prompt for repositories the server already approved", async () => {
    const { gate, onPrompt } = newGate();
    gate.setGranted([{ owner: "ACME", repo: "Docs" }]);

    expect(gate.isGranted(acmeDocs)).toBe(true);
    await expect(gate.request(acmeDocs)).resolves.toBe("allowed");
    expect(onPrompt).not.toHaveBeenCalled();
  });

  it("keeps a refused repository refused without prompting again", async () => {
    const { gate, onPrompt } = newGate();
    const first = gate.request(acmeSecrets);
    gate.settle("denied");

    await expect(first).resolves.toBe("denied");
    await expect(gate.request(acmeSecrets)).resolves.toBe("denied");
    await expect(gate.request(acmeSecrets)).resolves.toBe("denied");
    expect(onPrompt).toHaveBeenCalledOnce();
  });

  it("asks again after a grant that could not be saved", async () => {
    const { gate, onPrompt } = newGate();
    const first = gate.request(acmeDocs);
    gate.settle("failed");

    await expect(first).resolves.toBe("denied");
    void gate.request(acmeDocs);
    expect(onPrompt).toHaveBeenCalledTimes(2);
  });

  it("answers repeated requests for one repository with a single dialog", async () => {
    const { gate, onPrompt } = newGate();
    const decisions = Promise.all([
      gate.request(acmeDocs),
      gate.request(acmeDocs),
      gate.request({ owner: "acme", repo: "docs" }),
    ]);

    expect(onPrompt).toHaveBeenCalledOnce();
    gate.settle("allowed");
    await expect(decisions).resolves.toEqual(["allowed", "allowed", "allowed"]);
  });

  it("queues a second repository behind the open dialog", async () => {
    const { gate, onPrompt } = newGate();
    const docs = gate.request(acmeDocs);
    const secrets = gate.request(acmeSecrets);

    expect(onPrompt).toHaveBeenCalledExactlyOnceWith(acmeDocs);
    gate.settle("allowed");
    await expect(docs).resolves.toBe("allowed");

    expect(onPrompt).toHaveBeenLastCalledWith(acmeSecrets);
    expect(gate.prompt).toEqual(acmeSecrets);
    gate.settle("denied");
    await expect(secrets).resolves.toBe("denied");
    expect(gate.prompt).toBeNull();
  });

  it("refuses repositories beyond the queue limit instead of prompting", async () => {
    const { gate, onPrompt } = newGate(1);
    const open = gate.request(acmeDocs);
    const queued = gate.request(acmeSecrets);

    await expect(gate.request({ owner: "contoso", repo: "docs" })).resolves.toBe(
      "denied",
    );
    expect(onPrompt).toHaveBeenCalledOnce();

    gate.settle("allowed");
    await expect(open).resolves.toBe("allowed");
    expect(onPrompt).toHaveBeenLastCalledWith(acmeSecrets);
    gate.settle("allowed");
    await expect(queued).resolves.toBe("allowed");
    // The refusal above was not remembered, so the repository can be asked
    // for again once the queue drains.
    void gate.request({ owner: "contoso", repo: "docs" });
    expect(onPrompt).toHaveBeenLastCalledWith({ owner: "contoso", repo: "docs" });
  });

  it("ignores a settle with no open dialog", async () => {
    const { gate, onPrompt } = newGate();
    gate.settle("allowed");

    expect(onPrompt).not.toHaveBeenCalled();
    void gate.request(acmeDocs);
    expect(onPrompt).toHaveBeenCalledOnce();
  });

  it("refuses everything in flight on reset", async () => {
    const { gate, onPrompt } = newGate();
    const open = gate.request(acmeDocs);
    const queued = gate.request(acmeSecrets);
    gate.reset();

    await expect(open).resolves.toBe("denied");
    await expect(queued).resolves.toBe("denied");
    expect(gate.prompt).toBeNull();
    // Reset forgets decisions too: the next document starts from nothing.
    void gate.request(acmeDocs);
    expect(onPrompt).toHaveBeenCalledTimes(2);
  });
});

describe("repoKey", () => {
  it("joins owner and repository", () => {
    expect(repoKey(acmeDocs)).toBe("acme/docs");
  });
});
