// Consent rules for `window.aidocs.github.getRawContent`.
//
// The rendered document is untrusted and shares an origin with the bridge, so
// every field of a github-request message is unknown data, and a repository is
// only read after the reader approved it in the app shell. The server enforces
// the same rule; this module decides when the shell asks.

export type GitHubRepoRef = { owner: string; repo: string };

/** What the shell should do with one raw-content request. */
export type ConsentDecision = "allowed" | "denied";

/** How an open prompt ended. "failed" does not remember the repository, so a
 *  grant that could not be saved can be asked again. */
export type ConsentOutcome = ConsentDecision | "failed";

export const GITHUB_OWNER_MAX_LENGTH = 39;
export const GITHUB_REPO_MAX_LENGTH = 100;
export const GITHUB_PATH_MAX_LENGTH = 1024;
export const GITHUB_REF_MAX_LENGTH = 255;

// GitHub logins are alphanumeric with single, non-leading and non-trailing
// hyphens; repository names also allow dots and underscores. Both mirror the
// server, so the shell prompts for exactly what the server would accept.
const OWNER_PATTERN = /^[A-Za-z0-9](?:-?[A-Za-z0-9])*$/;
const REPO_PATTERN = /^[A-Za-z0-9._-]+$/;

/** normalizeRepoRef validates repository coordinates and lowercases them.
 *  GitHub resolves owner and repository case-insensitively, so consent is
 *  recorded and matched in one spelling. Returns null when unusable. */
export function normalizeRepoRef(owner: unknown, repo: unknown): GitHubRepoRef | null {
  if (typeof owner !== "string" || typeof repo !== "string") return null;
  if (owner.length > GITHUB_OWNER_MAX_LENGTH || !OWNER_PATTERN.test(owner)) return null;
  if (repo.length > GITHUB_REPO_MAX_LENGTH || !REPO_PATTERN.test(repo)) return null;
  if (repo === "." || repo === "..") return null;
  return { owner: owner.toLowerCase(), repo: repo.toLowerCase() };
}

export function repoKey(ref: GitHubRepoRef): string {
  return `${ref.owner}/${ref.repo}`;
}

export type GitHubRawRequestFields = {
  repository: GitHubRepoRef;
  path: string;
  ref: string;
};

/** parseGitHubRawRequest reads a github-request message, rejecting anything
 *  that is not a bounded string in the shape the API accepts. */
export function parseGitHubRawRequest(
  data: Record<string, unknown>,
): GitHubRawRequestFields | null {
  if (data.operation !== "raw") return null;
  const repository = normalizeRepoRef(data.owner, data.repo);
  if (!repository) return null;
  const { path, ref } = data;
  if (typeof path !== "string" || !path || path.length > GITHUB_PATH_MAX_LENGTH)
    return null;
  if (path.startsWith("/") || path.includes("..")) return null;
  if (typeof ref !== "string" || !ref || ref.length > GITHUB_REF_MAX_LENGTH)
    return null;
  return { repository, path, ref };
}

export type GitHubConsentGateOptions = {
  /** Opens the shell's approval dialog for one repository. */
  onPrompt: (repository: GitHubRepoRef) => void;
  /** Repositories that may wait behind the open dialog. Further repositories
   *  are refused without a prompt so a hostile document cannot queue an
   *  endless run of dialogs. */
  maxQueuedRepos?: number;
};

type Waiter = (decision: ConsentDecision) => void;

/**
 * Serializes approval for the repositories a document asks for: one dialog at
 * a time, one dialog per repository, and a repository the reader refused stays
 * refused until the page is loaded again.
 */
export class GitHubConsentGate {
  private readonly onPrompt: (repository: GitHubRepoRef) => void;
  private readonly maxQueuedRepos: number;
  private readonly granted = new Set<string>();
  private readonly denied = new Set<string>();
  private readonly waiters = new Map<string, Waiter[]>();
  private queue: GitHubRepoRef[] = [];
  private open: GitHubRepoRef | null = null;

  constructor({ onPrompt, maxQueuedRepos = 3 }: GitHubConsentGateOptions) {
    this.onPrompt = onPrompt;
    this.maxQueuedRepos = maxQueuedRepos;
  }

  /** The repository whose dialog is open, or null. */
  get prompt(): GitHubRepoRef | null {
    return this.open;
  }

  /** Seeds the repositories the server already holds consent for. */
  setGranted(repositories: Iterable<GitHubRepoRef>): void {
    for (const repository of repositories) {
      const normalized = normalizeRepoRef(repository.owner, repository.repo);
      if (normalized) this.granted.add(repoKey(normalized));
    }
  }

  isGranted(repository: GitHubRepoRef): boolean {
    return this.granted.has(repoKey(repository));
  }

  request(repository: GitHubRepoRef): Promise<ConsentDecision> {
    const key = repoKey(repository);
    if (this.granted.has(key)) return Promise.resolve("allowed");
    if (this.denied.has(key)) return Promise.resolve("denied");
    const waiting = this.waiters.get(key);
    if (waiting) {
      // Same repository, same dialog: a document that asks a hundred times
      // still gets one prompt.
      return new Promise((resolve) => waiting.push(resolve));
    }
    if (this.open && this.queue.length >= this.maxQueuedRepos) {
      return Promise.resolve("denied");
    }
    return new Promise((resolve) => {
      this.waiters.set(key, [resolve]);
      this.queue.push(repository);
      if (!this.open) this.openNext();
    });
  }

  /** Settles the open dialog and opens the next repository, if any. */
  settle(outcome: ConsentOutcome): void {
    const repository = this.open;
    if (!repository) return;
    const key = repoKey(repository);
    this.open = null;
    if (outcome === "allowed") this.granted.add(key);
    if (outcome === "denied") this.denied.add(key);
    const waiting = this.waiters.get(key) || [];
    this.waiters.delete(key);
    for (const resolve of waiting)
      resolve(outcome === "allowed" ? "allowed" : "denied");
    this.openNext();
  }

  /** Drops every decision and pending request, for a new page or document. */
  reset(): void {
    const waiting = [...this.waiters.values()].flat();
    this.waiters.clear();
    this.queue = [];
    this.open = null;
    this.granted.clear();
    this.denied.clear();
    for (const resolve of waiting) resolve("denied");
  }

  private openNext(): void {
    const next = this.queue.shift();
    if (!next) return;
    this.open = next;
    this.onPrompt(next);
  }
}
