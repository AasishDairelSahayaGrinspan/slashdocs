export type Principal = {
  type: "user" | "service_account" | "anyone";
  id: string;
  email?: string;
  name?: string;
  picture_url?: string;
};
export type CommentMentionPrincipal = Omit<Principal, "type"> & {
  type: "user" | "service_account";
};
export type CommentMentionPrincipalRef = Pick<CommentMentionPrincipal, "type" | "id">;
export type UserPrincipal = Omit<Principal, "type"> & { type: "user" };
export type CommentMention = {
  principal: CommentMentionPrincipal;
  offset: number;
  length: number;
};
export type Document = {
  id: string;
  title: string;
  owner?: Principal;
  current_version_id?: string;
  parent_folder_id?: string;
  parent_folder?: { id: string; title: string };
  repository_id?: string;
  updated_at?: string;
};
export type ServerConfig = {
  deployment: "public" | "org";
  org_name: string;
  everyone_label: string;
  provider_name: string;
  development?: boolean;
};
export type Version = {
  id: string;
  number: number;
  document_id?: string;
  created_by?: Principal;
  change_summary?: string;
  sha256?: string;
};
export type Anchor = {
  quote: string;
  prefix?: string;
  suffix?: string;
  dom_path?: string;
  start_offset?: number;
  end_offset?: number;
};
export type Comment = {
  id: string;
  body: string;
  selected_text?: string;
  status: string;
  author: Principal;
  created_on_version_id: string;
  parent_comment_id?: string;
  root_comment_id?: string;
  // Optional only for rolling-upgrade reads from an older server. New servers
  // always return an array, including when it is empty.
  mentions?: CommentMention[];
  anchor?: Anchor;
  replies?: Comment[];
  current_placement?: {
    status: string;
    confidence: number;
    matched_text: string;
  };
};
export type CommentPatch = {
  body?: string;
  status?: string;
  mentions?: CommentMention[];
};

const COMMENT_MENTIONS_MEDIA_TYPE = "application/vnd.aidocs.comment-mentions+json";
export type Grant = {
  id: string;
  principal?: Principal;
  role: string;
};
export type FolderBreadcrumb = {
  id: string;
  title: string;
};

export type Folder = {
  id: string;
  title: string;
  owner?: Principal;
  parent_folder_id?: string;
  ancestors?: FolderBreadcrumb[];
};
export type FolderEntry = {
  type: "folder" | "document";
  folder?: Folder;
  document?: Document;
  last_active?: string;
};
export type Pin = {
  resource_type: "document" | "folder";
  resource_id: string;
};
export type DocumentKVSnapshot = {
  revision: number;
  items: Record<string, unknown>;
};
export type DocumentKVChange = {
  key: string;
  value?: unknown;
  deleted?: boolean;
  revision: number;
  updated_at?: string;
  updated_by?: Principal;
  operation?: string;
};
export type DocumentKVPatch = {
  set?: Record<string, unknown>;
  delete?: string[];
};
export type DocumentKVPatchResult = {
  revision: number;
  changes: DocumentKVChange[];
  updated_by?: Principal;
};
export type DocumentKVEvent = {
  document_id: string;
  revision: number;
  changes: DocumentKVChange[];
  updated_at?: string;
  updated_by?: Principal;
};
export type AccessSource = {
  resource_type: string;
  resource_id: string;
  resource_title: string;
  role: string;
  grant_id?: string;
};
export type EffectiveAccessItem = {
  principal: Principal;
  effective_role: string;
  sources: AccessSource[];
};
export type ServiceAccount = {
  id: string;
  name: string;
  disabled?: boolean;
  owner?: Principal;
};
export type ServiceAccountKey = { id: string; name: string };

export type RepositoryScope = "personal" | "organization";
export type RepositoryNamespace = {
  id?: string;
  name?: string;
  slug?: string;
};
export type Repository = {
  id: string;
  name: string;
  slug: string;
  description?: string;
  scope: RepositoryScope;
  namespace_id?: string;
  namespace_name?: string;
  namespace?: RepositoryNamespace;
  owner?: Principal;
  root_folder_id?: string;
  created_by?: Principal;
  required_approvals?: number;
  protected?: boolean;
  created_at?: string;
  updated_at?: string;
};
export type PullRequestReviewState = "approved" | "changes_requested";
export type PullRequestReview = {
  id: string;
  state: PullRequestReviewState;
  body?: string;
  reviewer: Principal;
  head_version_id?: string;
  submitted_at?: string;
};
export type PullRequestStatus = "open" | "closed" | "merged";
export type PullRequestChangeOperation =
  | "update"
  | "create"
  | "rename"
  | "move"
  | "delete";
export type PullRequestChange = {
  id: string;
  pull_request_id?: string;
  patchset?: number;
  operation: PullRequestChangeOperation;
  document_id?: string;
  folder_id?: string;
  base_version_id?: string;
  head_version_id?: string;
  base_title?: string;
  title?: string;
  base_path?: string;
  path?: string;
};
export type PullRequestChangeInput = {
  operation: PullRequestChangeOperation;
  document_id?: string;
  folder_id?: string;
  base_version_id?: string;
  title?: string;
  path?: string;
  content_html?: string;
};
export type PullRequest = {
  id: string;
  number: number;
  repository_id: string;
  document_id: string;
  base_version_id: string;
  head_version_id: string;
  title: string;
  body?: string;
  change_summary?: string;
  status: PullRequestStatus;
  author?: Principal;
  reviews?: PullRequestReview[];
  required_approvals?: number;
  mergeable?: boolean;
  merge_blocked_reason?: string;
  created_at?: string;
  updated_at?: string;
  merged_at?: string;
  merged_by?: Principal;
  patchset?: number;
  changes?: PullRequestChange[];
};
export type RepositorySourceProvider =
  | "github"
  | "slack"
  | "devrev"
  | "google_drive"
  | "confluence"
  | "notion"
  | "manual";
export type ConnectedAccountProvider = Extract<
  RepositorySourceProvider,
  "github" | "slack" | "devrev" | "google_drive"
>;
export type ConnectedAccount = {
  id: string;
  provider: ConnectedAccountProvider;
  external_account_id?: string;
  display_name: string;
  email?: string;
  workspace_id?: string;
  workspace_name?: string;
  scopes?: string[];
  status: "active" | "demo" | "needs_reconnect" | string;
  expires_at?: string;
  created_at?: string;
  updated_at?: string;
};
export type RepositorySource = {
  id: string;
  repository_id: string;
  provider: RepositorySourceProvider;
  connection_id?: string;
  connection?: ConnectedAccount;
  external_repository?: string;
  branch?: string;
  include_paths?: string[];
  exclude_paths?: string[];
  target_folder_id?: string;
  target_document_id?: string;
  trigger_mode?:
    | "default_branch"
    | "merged_pull_requests"
    | "event"
    | "manual"
    | "scheduled"
    | string;
  direction?: "into_aidocs" | "export_only" | "bidirectional" | string;
  status?: string;
  freshness?: {
    state?: string;
    checked_at?: string;
    stale_after_hours?: number;
  };
  last_synced_at?: string;
  created_by?: Principal;
  created_at?: string;
  updated_at?: string;
};
export type RepositorySourceInput = {
  provider: RepositorySourceProvider;
  connection_id?: string;
  external_repository?: string;
  branch?: string;
  include_paths?: string[];
  exclude_paths?: string[];
  target_folder_id?: string;
  target_document_id?: string;
  trigger_mode?: string;
  direction?: string;
  status?: "active" | "paused";
};
export type RepositorySourceCheckInput = {
  before_sha?: string;
  after_sha?: string;
  delivery_id?: string;
  changed_files?: string[];
};
export type RepositorySyncRun = {
  id: string;
  repository_id?: string;
  source_id?: string;
  provider?: RepositorySourceProvider;
  direction?: string;
  trigger?: string;
  status: string;
  summary?: string;
  message?: string;
  outcome?: string;
  changed_files?: string[];
  pull_request_number?: number;
  started_at?: string;
  completed_at?: string;
  finished_at?: string;
  created_at?: string;
  added?: number;
  updated?: number;
  removed?: number;
  error?: string;
};
export type RepositoryActivity = {
  id: string;
  repository_id?: string;
  type?: string;
  action?: string;
  title?: string;
  description?: string;
  actor?: Principal;
  resource_type?: string;
  resource_id?: string;
  document_id?: string;
  pull_request_number?: number;
  occurred_at?: string;
  created_at?: string;
  metadata?: Record<string, unknown>;
  subject_type?: string;
  subject_id?: string;
  details?: Record<string, unknown>;
};
export type KnowledgeCitation = {
  document_id?: string;
  title?: string;
  path?: string;
  quote?: string;
  snippet?: string;
  excerpt?: string;
  url?: string;
  score?: number;
  version_id?: string;
};
export type KnowledgeSearchResult = KnowledgeCitation & {
  id?: string;
  document?: Document;
};
export type KnowledgeGraphNode = {
  id: string;
  type?: string;
  title?: string;
  label?: string;
  document_id?: string;
  path?: string;
};
export type KnowledgeGraphEdge = {
  id?: string;
  source: string;
  target: string;
  type?: string;
  label?: string;
};
export type RepositorySettings = {
  repository_id?: string;
  archived?: boolean;
  required_reviewers?: string[];
  required_teams?: string[];
  path_rules?: Array<{
    pattern: string;
    required_reviewers?: string[];
    required_teams?: string[];
  }>;
  portal?: {
    enabled?: boolean;
    slug?: string;
    visibility?: "public" | "organization" | "private" | string;
  };
  freshness_policy?: {
    stale_after_hours?: number;
    on_stale?: "notify" | "open_pull_request" | "none" | string;
  };
  transfer?: {
    owner_id?: string;
    owner_email?: string;
    status?: string;
    requested_by_id?: string;
    updated_at?: string;
  };
  updated_at?: string;
};
export type RepositoryTeam = {
  id: string;
  repository_id?: string;
  name: string;
  description?: string;
  role?: string;
  members?: Principal[];
  member_ids?: string[];
  created_at?: string;
};
export type AidocsNotification = {
  id: string;
  type?: string;
  title: string;
  body?: string;
  read?: boolean;
  repository_id?: string;
  repository_name?: string;
  document_id?: string;
  pull_request_number?: number;
  url?: string;
  created_at?: string;
};
export type ReviewInboxItem = {
  id?: string;
  repository?: Repository;
  repository_id?: string;
  pull_request?: PullRequest;
  reason?: string;
  requested_at?: string;
  updated_at?: string;
};
export type AidocsTemplate = {
  id: string;
  name: string;
  description?: string;
  category?: string;
  tags?: string[];
  document_count?: number;
  preview_url?: string;
  title?: string;
};
export type PublicPortal = {
  slug: string;
  title?: string;
  description?: string;
  accent_color?: string;
  repository?: Repository;
  documents?: Array<
    Document & {
      path?: string;
      excerpt?: string;
      text?: string;
      public_url?: string;
    }
  >;
};
export type UserProfileDocument = {
  document: Document;
  repository?: Repository;
};
export type UserProfilePullRequest = {
  pull_request: PullRequest;
  repository: Repository;
};
export type UserContribution = {
  id: string;
  type: string;
  title: string;
  document_id?: string;
  document_title?: string;
  repository_id?: string;
  repository_name?: string;
  pull_request_number?: number;
  state?: string;
  occurred_at?: string;
};
export type UserProfile = {
  user: Principal;
  stats: {
    documents_created: number;
    pull_requests_authored: number;
    contributions: number;
    repositories_contributed_to: number;
  };
  documents: UserProfileDocument[];
  pull_requests: UserProfilePullRequest[];
  contributions: UserContribution[];
};
export type CreateRepositoryInput = {
  name: string;
  slug: string;
  description?: string;
  scope: RepositoryScope;
  root_folder_id?: string;
  document_id?: string;
};
export type ImportRepositoryInput = {
  name: string;
  slug: string;
  description?: string;
  scope: RepositoryScope;
  import_kind: "folder" | "document";
  files: File[];
  paths?: string[];
};
export type ImportRepositoryResult = {
  repository: Repository;
  documents: Document[];
};
export type CreatePullRequestInput = {
  document_id: string;
  base_version_id: string;
  title: string;
  body?: string;
  change_summary?: string;
  file: File;
};

export class APIError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public details?: unknown,
  ) {
    super(message);
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(path, {
    credentials: "include",
    ...init,
    headers: {
      ...(init.body instanceof FormData ? {} : { "Content-Type": "application/json" }),
      ...(init.headers || {}),
    },
  });
  if (!res.ok) {
    let msg = res.statusText;
    let code = "http_error";
    let details: unknown;
    try {
      const payload = (await res.json()) as {
        error?: { message?: string; code?: string; details?: unknown };
      };
      msg = payload.error?.message || msg;
      code = payload.error?.code || code;
      details = payload.error?.details;
    } catch {}
    throw new APIError(res.status, code, msg, details);
  }
  if (res.status === 204) return undefined as T;
  const ct = res.headers.get("content-type") || "";
  if (ct.includes("application/json")) {
    return (await res.json()) as T;
  }
  return (await res.text()) as T;
}

function mentionPayload(mentions: CommentMention[]) {
  return mentions.map((mention) => ({
    principal: {
      type: mention.principal.type,
      id: mention.principal.id,
    } satisfies CommentMentionPrincipalRef,
    offset: mention.offset,
    length: mention.length,
  }));
}

export const api = {
  me: () =>
    request<{
      principal: Principal;
      user?: Principal;
      service_account?: unknown;
    }>("/v1/me"),
  loginURL: (redirect: string = location.pathname + location.search) =>
    `/v1/auth/google/start?mode=web&redirect=${encodeURIComponent(redirect)}`,
  listDocuments: (q?: string) =>
    request<{ items: Document[] }>(
      `/v1/documents${q ? `?q=${encodeURIComponent(q)}` : ""}`,
    ),
  getDocument: (id: string) => request<Document>(`/v1/documents/${id}`),
  getConfig: () => request<ServerConfig>("/v1/config"),
  searchUsers: (q: string) =>
    request<{ items: UserPrincipal[] }>(`/v1/users?q=${encodeURIComponent(q)}`),
  getUserProfile: (id: string) =>
    request<UserProfile>(`/v1/users/${encodeURIComponent(id)}/profile`),
  listConnections: () =>
    request<{ connections: ConnectedAccount[] }>("/v1/connections"),
  connectionStartURL: (
    provider: Exclude<ConnectedAccountProvider, "devrev">,
    redirect = "/app/settings/connections",
  ) => `/v1/connections/${provider}/start?redirect=${encodeURIComponent(redirect)}`,
  connectDevRev: (personalAccessToken: string) =>
    request<ConnectedAccount>("/v1/connections/devrev", {
      method: "POST",
      body: JSON.stringify({ personal_access_token: personalAccessToken }),
    }),
  getGooglePickerToken: (connectionId: string) =>
    request<{
      access_token: string;
      expires_at?: string | null;
      api_key: string;
      app_id: string;
    }>(`/v1/connections/${encodeURIComponent(connectionId)}/google-picker-token`, {
      method: "POST",
    }),
  deleteConnection: (id: string) =>
    request<void>(`/v1/connections/${encodeURIComponent(id)}`, { method: "DELETE" }),
  createDocument: (title: string, file: File) => {
    const fd = new FormData();
    fd.set("title", title);
    fd.set("file", file);
    return request<{ id: string; current_version_id: string }>("/v1/documents", {
      method: "POST",
      body: fd,
    });
  },
  listVersions: (doc: string) =>
    request<{ items: Version[] }>(`/v1/documents/${doc}/versions`),
  createVersion: (doc: string, base: string, summary: string, file: File) => {
    const fd = new FormData();
    fd.set("base_version_id", base);
    fd.set("change_summary", summary);
    fd.set("file", file);
    return request<Version>(`/v1/documents/${doc}/versions`, {
      method: "POST",
      body: fd,
    });
  },
  renderToken: (doc: string, version: string) =>
    request<{ token: string; url: string }>(
      `/v1/documents/${doc}/versions/${version}/render-token`,
      { method: "POST" },
    ),
  listDocumentKV: (doc: string) =>
    request<DocumentKVSnapshot>(`/v1/documents/${doc}/kv`),
  patchDocumentKV: (doc: string, patch: DocumentKVPatch) =>
    request<DocumentKVPatchResult>(`/v1/documents/${doc}/kv`, {
      method: "PATCH",
      body: JSON.stringify(patch),
    }),
  listComments: (doc: string, version?: string) =>
    request<{ items: Comment[] }>(
      `/v1/documents/${doc}/comments${version ? `?version_id=${encodeURIComponent(version)}` : ""}`,
    ),
  createComment: (
    doc: string,
    version: string,
    body: string,
    quote: string,
    anchor?: Partial<Anchor>,
    mentions: CommentMention[] = [],
  ) =>
    request<Comment>(`/v1/documents/${doc}/comments`, {
      method: "POST",
      ...(mentions.length > 0
        ? { headers: { "Content-Type": COMMENT_MENTIONS_MEDIA_TYPE } }
        : {}),
      body: JSON.stringify({
        version_id: version,
        body,
        mentions: mentionPayload(mentions),
        // Use the anchor captured from the render bridge, falling back to a
        // placeholder when the bridge supplied no selection offsets.
        anchor: {
          quote,
          prefix: anchor?.prefix ?? "",
          suffix: anchor?.suffix ?? "",
          dom_path: anchor?.dom_path ?? "body",
          start_offset: anchor?.start_offset ?? 0,
          end_offset: anchor?.end_offset ?? quote.length,
        },
      }),
    }),
  createCommentReply: (
    doc: string,
    id: string,
    body: string,
    mentions: CommentMention[] = [],
  ) =>
    request<Comment>(`/v1/documents/${doc}/comments/${id}/replies`, {
      method: "POST",
      ...(mentions.length > 0
        ? { headers: { "Content-Type": COMMENT_MENTIONS_MEDIA_TYPE } }
        : {}),
      body: JSON.stringify({ body, mentions: mentionPayload(mentions) }),
    }),
  patchComment: (doc: string, id: string, patch: CommentPatch) =>
    request<Comment>(`/v1/documents/${doc}/comments/${id}`, {
      method: "PATCH",
      ...(patch.mentions !== undefined
        ? { headers: { "Content-Type": COMMENT_MENTIONS_MEDIA_TYPE } }
        : {}),
      body: JSON.stringify({
        ...patch,
        ...(patch.mentions !== undefined
          ? { mentions: mentionPayload(patch.mentions) }
          : {}),
      }),
    }),
  deleteComment: (doc: string, id: string) =>
    request<void>(`/v1/documents/${doc}/comments/${id}`, { method: "DELETE" }),
  createGrant: (doc: string, address: string, role: string) =>
    request<Grant>(`/v1/documents/${doc}/grants`, {
      method: "POST",
      body: JSON.stringify({ address, role }),
    }),
  // setGeneralAccess upserts the "anyone" grant (everyone who can reach this
  // server). Pass role "" / null to remove it (no general access).
  setGeneralAccess: (doc: string, role: string) =>
    request<Grant>(`/v1/documents/${doc}/grants`, {
      method: "POST",
      body: JSON.stringify({ principal: { type: "anyone" }, role }),
    }),
  updateGrant: (doc: string, grantId: string, role: string) =>
    request<Grant>(`/v1/documents/${doc}/grants/${grantId}`, {
      method: "PATCH",
      body: JSON.stringify({ role }),
    }),
  deleteGrant: (doc: string, grantId: string) =>
    request<void>(`/v1/documents/${doc}/grants/${grantId}`, {
      method: "DELETE",
    }),
  listServiceAccounts: () =>
    request<{ items: ServiceAccount[] }>("/v1/service-accounts"),
  createServiceAccount: (label: string, domain?: string) =>
    request<{
      id: string;
      label: string;
      name: string;
      key: { id: string; token: string };
    }>("/v1/service-accounts", {
      method: "POST",
      body: JSON.stringify(domain ? { label, domain } : { label }),
    }),
  deleteDocument: (id: string) =>
    request<void>(`/v1/documents/${id}`, { method: "DELETE" }),
  updateServiceAccount: (id: string, name: string, disabled: boolean) =>
    request<ServiceAccount>(`/v1/service-accounts/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ name, disabled }),
    }),
  listServiceAccountKeys: (id: string) =>
    request<{ items: ServiceAccountKey[] }>(`/v1/service-accounts/${id}/keys`),
  createServiceAccountKey: (id: string, name: string) =>
    request<{ id: string; token: string }>(`/v1/service-accounts/${id}/keys`, {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  revokeServiceAccountKey: (id: string, key: string) =>
    request<void>(`/v1/service-accounts/${id}/keys/${key}`, {
      method: "DELETE",
    }),
  // Repositories and pull requests
  listRepositories: () => request<{ items: Repository[] }>("/v1/repositories"),
  createRepository: (input: CreateRepositoryInput) =>
    request<Repository>("/v1/repositories", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  importRepository: (input: ImportRepositoryInput) => {
    const fd = new FormData();
    fd.set("name", input.name);
    fd.set("slug", input.slug);
    fd.set("description", input.description || "");
    fd.set("scope", input.scope);
    fd.set("import_kind", input.import_kind);
    input.files.forEach((file, index) => {
      fd.append("files", file, file.name);
      const path = input.paths?.[index];
      if (path) fd.append("paths", path);
    });
    return request<ImportRepositoryResult>("/v1/repositories/import", {
      method: "POST",
      body: fd,
    });
  },
  getRepository: (id: string) => request<Repository>(`/v1/repositories/${id}`),
  deleteRepository: (id: string) =>
    request<void>(`/v1/repositories/${id}`, { method: "DELETE" }),
  getRepositorySettings: (id: string) =>
    request<RepositorySettings>(`/v1/repositories/${id}/settings`),
  updateRepositorySettings: (id: string, settings: Partial<RepositorySettings>) =>
    request<RepositorySettings>(`/v1/repositories/${id}/settings`, {
      method: "PATCH",
      body: JSON.stringify(settings),
    }),
  archiveRepository: (id: string) =>
    request<RepositorySettings>(`/v1/repositories/${id}/archive`, {
      method: "POST",
    }),
  restoreRepository: (id: string) =>
    request<RepositorySettings>(`/v1/repositories/${id}/restore`, {
      method: "POST",
    }),
  transferRepository: (id: string, owner: string) =>
    request<{ repository_id: string; target?: Principal; transfer?: unknown }>(
      `/v1/repositories/${id}/transfer`,
      {
        method: "POST",
        body: JSON.stringify(
          owner.includes("@") ? { new_owner_email: owner } : { new_owner_id: owner },
        ),
      },
    ),
  acceptRepositoryTransfer: (id: string) =>
    request<{ repository_id: string; transfer?: unknown }>(
      `/v1/repositories/${id}/transfer/accept`,
      { method: "POST" },
    ),
  cancelRepositoryTransfer: (id: string) =>
    request<{ repository_id: string; transfer?: unknown }>(
      `/v1/repositories/${id}/transfer/cancel`,
      { method: "POST" },
    ),
  listRepositoryDocuments: (id: string) =>
    request<{ items: Document[] }>(`/v1/repositories/${id}/documents`),
  createRepositoryDocument: (id: string, title: string, file: File) => {
    const fd = new FormData();
    fd.set("title", title);
    fd.set("file", file);
    return request<{ document: Document; version: Version }>(
      `/v1/repositories/${id}/documents`,
      { method: "POST", body: fd },
    );
  },
  listPullRequests: (repositoryId: string) =>
    request<{ items: PullRequest[] }>(`/v1/repositories/${repositoryId}/pull-requests`),
  createPullRequest: (repositoryId: string, input: CreatePullRequestInput) => {
    const fd = new FormData();
    fd.set("document_id", input.document_id);
    fd.set("base_version_id", input.base_version_id);
    fd.set("title", input.title);
    fd.set("body", input.body || "");
    fd.set("change_summary", input.change_summary || "");
    fd.set("file", input.file);
    return request<PullRequest>(`/v1/repositories/${repositoryId}/pull-requests`, {
      method: "POST",
      body: fd,
    });
  },
  createPullRequestChangeSet: (
    repositoryId: string,
    input: {
      title: string;
      body?: string;
      change_summary?: string;
      changes: PullRequestChangeInput[];
    },
  ) =>
    request<PullRequest>(`/v1/repositories/${repositoryId}/pull-requests/change-set`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
  getPullRequest: (repositoryId: string, number: number | string) =>
    request<PullRequest>(`/v1/repositories/${repositoryId}/pull-requests/${number}`),
  createPullRequestPatchset: (
    repositoryId: string,
    number: number | string,
    input: { change_summary?: string; changes: PullRequestChangeInput[] },
  ) =>
    request<PullRequest>(
      `/v1/repositories/${repositoryId}/pull-requests/${number}/patchsets`,
      { method: "POST", body: JSON.stringify(input) },
    ),
  rebasePullRequest: (repositoryId: string, number: number | string) =>
    request<PullRequest>(
      `/v1/repositories/${repositoryId}/pull-requests/${number}/rebase`,
      { method: "POST" },
    ),
  reviewPullRequest: (
    repositoryId: string,
    number: number | string,
    state: PullRequestReviewState,
    body: string,
  ) =>
    request<PullRequest>(
      `/v1/repositories/${repositoryId}/pull-requests/${number}/reviews`,
      { method: "POST", body: JSON.stringify({ state, body }) },
    ),
  mergePullRequest: (repositoryId: string, number: number | string) =>
    request<PullRequest>(
      `/v1/repositories/${repositoryId}/pull-requests/${number}/merge`,
      { method: "POST" },
    ),
  closePullRequest: (repositoryId: string, number: number | string) =>
    request<PullRequest>(`/v1/repositories/${repositoryId}/pull-requests/${number}`, {
      method: "PATCH",
      body: JSON.stringify({ status: "closed" }),
    }),
  listRepositorySources: (repositoryId: string) =>
    request<{ items: RepositorySource[] }>(`/v1/repositories/${repositoryId}/sources`),
  createRepositorySource: (repositoryId: string, input: RepositorySourceInput) =>
    request<RepositorySource>(`/v1/repositories/${repositoryId}/sources`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
  updateRepositorySource: (
    repositoryId: string,
    sourceId: string,
    input: Partial<RepositorySourceInput>,
  ) =>
    request<RepositorySource>(`/v1/repositories/${repositoryId}/sources/${sourceId}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    }),
  deleteRepositorySource: (repositoryId: string, sourceId: string) =>
    request<void>(`/v1/repositories/${repositoryId}/sources/${sourceId}`, {
      method: "DELETE",
    }),
  checkRepositorySource: (
    repositoryId: string,
    sourceId: string,
    input: RepositorySourceCheckInput = {},
  ) =>
    request<RepositorySyncRun>(
      `/v1/repositories/${repositoryId}/sources/${sourceId}/check`,
      { method: "POST", body: JSON.stringify(input) },
    ),
  simulateRepositorySource: (
    repositoryId: string,
    sourceId: string,
    input: RepositorySourceCheckInput = {},
  ) =>
    request<RepositorySyncRun>(
      `/v1/repositories/${repositoryId}/sources/${sourceId}/simulate`,
      { method: "POST", body: JSON.stringify(input) },
    ),
  exportRepositorySource: (repositoryId: string, sourceId: string) =>
    request<{ proposal: unknown }>(
      `/v1/repositories/${repositoryId}/sources/${sourceId}/export`,
      { method: "POST" },
    ),
  listRepositorySyncRuns: (repositoryId: string) =>
    request<{ items: RepositorySyncRun[] }>(
      `/v1/repositories/${repositoryId}/sync-runs`,
    ),
  listRepositoryActivity: (repositoryId: string) =>
    request<{ items: RepositoryActivity[] }>(
      `/v1/repositories/${repositoryId}/activity`,
    ),
  searchRepositoryKnowledge: (repositoryId: string, query: string) =>
    request<{ items: KnowledgeSearchResult[] }>(
      `/v1/repositories/${repositoryId}/search?q=${encodeURIComponent(query)}`,
    ),
  askRepositoryQuestion: (repositoryId: string, question: string) =>
    request<{ answer: string; citations: KnowledgeCitation[] }>(
      `/v1/repositories/${repositoryId}/questions`,
      { method: "POST", body: JSON.stringify({ question }) },
    ),
  listRepositoryBacklinks: (repositoryId: string, documentId: string) =>
    request<{
      items: Array<KnowledgeCitation & { from?: string; to?: string }>;
    }>(
      `/v1/repositories/${repositoryId}/backlinks?document_id=${encodeURIComponent(documentId)}`,
    ).then((result) => ({
      items: (result.items || []).map((item) => ({
        ...item,
        document_id: item.document_id || item.from,
        title: item.title || item.from,
      })),
    })),
  getRepositoryGraph: (repositoryId: string) =>
    request<{
      nodes: KnowledgeGraphNode[];
      edges: Array<KnowledgeGraphEdge & { from?: string; to?: string }>;
    }>(`/v1/repositories/${repositoryId}/graph`).then((result) => ({
      nodes: result.nodes || [],
      edges: (result.edges || []).map((edge) => ({
        ...edge,
        source: edge.source || edge.from || "",
        target: edge.target || edge.to || "",
      })),
    })),
  listRepositoryTeams: (repositoryId: string) =>
    request<{ items: RepositoryTeam[] }>(`/v1/repositories/${repositoryId}/teams`),
  createRepositoryTeam: (
    repositoryId: string,
    input: Pick<RepositoryTeam, "name" | "description" | "role">,
  ) =>
    request<RepositoryTeam>(`/v1/repositories/${repositoryId}/teams`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
  updateRepositoryTeam: (
    repositoryId: string,
    teamId: string,
    input: Partial<Pick<RepositoryTeam, "name" | "description" | "role">>,
  ) =>
    request<RepositoryTeam>(`/v1/repositories/${repositoryId}/teams/${teamId}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    }),
  deleteRepositoryTeam: (repositoryId: string, teamId: string) =>
    request<void>(`/v1/repositories/${repositoryId}/teams/${teamId}`, {
      method: "DELETE",
    }),
  addRepositoryTeamMember: (repositoryId: string, teamId: string, userId: string) =>
    request<RepositoryTeam>(
      `/v1/repositories/${repositoryId}/teams/${teamId}/members/${encodeURIComponent(userId)}`,
      { method: "PUT" },
    ),
  removeRepositoryTeamMember: (repositoryId: string, teamId: string, userId: string) =>
    request<RepositoryTeam>(
      `/v1/repositories/${repositoryId}/teams/${teamId}/members/${encodeURIComponent(userId)}`,
      { method: "DELETE" },
    ),
  // Folders
  listFolders: (folderId?: string, path?: string, q?: string) => {
    const params = new URLSearchParams();
    if (folderId) params.set("folder_id", folderId);
    if (path) params.set("path", path);
    if (q) params.set("q", q);
    const qs = params.toString();
    return request<{ folder?: Folder; items: FolderEntry[]; has_more: boolean }>(
      `/v1/folders${qs ? `?${qs}` : ""}`,
    );
  },
  getFolder: (id: string) => request<Folder>(`/v1/folders/${id}`),
  createFolder: (title: string, parentFolderId?: string) =>
    request<Folder>("/v1/folders", {
      method: "POST",
      body: JSON.stringify({ title, parent_folder_id: parentFolderId || undefined }),
    }),
  updateFolder: (id: string, title?: string, parentFolderId?: string | null) =>
    request<Folder & { permission_warning?: { kind: string; message: string } }>(
      `/v1/folders/${id}`,
      {
        method: "PATCH",
        body: JSON.stringify({
          ...(title ? { title } : {}),
          ...(parentFolderId !== undefined ? { parent_folder_id: parentFolderId } : {}),
        }),
      },
    ),
  deleteFolder: (id: string) =>
    request<void>(`/v1/folders/${id}`, { method: "DELETE" }),
  createFolderGrant: (id: string, address: string, role: string) =>
    request<Grant>(`/v1/folders/${id}/grants`, {
      method: "POST",
      body: JSON.stringify({ address, role }),
    }),
  setFolderGeneralAccess: (id: string, role: string) =>
    request<Grant>(`/v1/folders/${id}/grants`, {
      method: "POST",
      body: JSON.stringify({ principal: { type: "anyone" }, role }),
    }),
  updateFolderGrant: (folderId: string, grantId: string, role: string) =>
    request<Grant>(`/v1/folders/${folderId}/grants/${grantId}`, {
      method: "PATCH",
      body: JSON.stringify({ role }),
    }),
  deleteFolderGrant: (folderId: string, grantId: string) =>
    request<void>(`/v1/folders/${folderId}/grants/${grantId}`, {
      method: "DELETE",
    }),
  listFolderDocuments: (id: string) =>
    request<{ items: Document[] }>(`/v1/folders/${id}/documents`),
  moveDocumentToFolder: (folderId: string, documentId: string) =>
    request<{
      document_id: string;
      moved: boolean;
      permission_warning?: { message: string };
    }>(`/v1/folders/${folderId}/documents`, {
      method: "POST",
      body: JSON.stringify({ document_id: documentId }),
    }),
  moveDocumentToRoot: (folderId: string, documentId: string) =>
    request<void>(`/v1/folders/${folderId}/documents/${documentId}`, {
      method: "DELETE",
    }),
  folderAccess: (id: string) =>
    request<{ items: EffectiveAccessItem[] }>(`/v1/folders/${id}/access`),
  documentAccess: (id: string) =>
    request<{ items: EffectiveAccessItem[] }>(`/v1/documents/${id}/access`),
  listNotifications: () =>
    request<{ items: AidocsNotification[] }>("/v1/notifications"),
  markNotificationRead: (id: string, read = true) =>
    request<AidocsNotification>(`/v1/notifications/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ read }),
    }),
  listReviewInbox: () => request<{ items: ReviewInboxItem[] }>("/v1/review-inbox"),
  listTemplates: () => request<{ items: AidocsTemplate[] }>("/v1/templates"),
  applyTemplate: (
    repositoryId: string,
    templateId: string,
    targetDocumentId?: string,
    title?: string,
  ) =>
    request<{
      status: "preview" | "pull_request_opened";
      template?: AidocsTemplate;
      message?: string;
      pull_request?: PullRequest;
    }>(`/v1/repositories/${repositoryId}/templates/${templateId}/apply`, {
      method: "POST",
      body: JSON.stringify({
        target_document_id: targetDocumentId || undefined,
        title: title || undefined,
      }),
    }),
  getPublicPortal: (slug: string) =>
    request<{
      repository?: Repository;
      portal?: { enabled?: boolean; slug?: string; visibility?: string };
      documents?: Array<
        Document & {
          version_id?: string;
          text?: string;
          excerpt?: string;
          path?: string;
          public_url?: string;
        }
      >;
    }>(`/v1/public/portals/${encodeURIComponent(slug)}`).then(
      (result): PublicPortal => ({
        slug: result.portal?.slug || slug,
        title: result.repository?.name,
        description: result.repository?.description,
        repository: result.repository,
        documents: (result.documents || []).map((document) => ({
          ...document,
          excerpt: document.excerpt || document.text,
        })),
      }),
    ),
  // Pins
  listPins: () => request<{ items: Pin[] }>("/v1/pins"),
  putPin: (resourceType: string, resourceId: string) =>
    request<void>(`/v1/pins/${resourceType}/${resourceId}`, { method: "PUT" }),
  deletePin: (resourceType: string, resourceId: string) =>
    request<void>(`/v1/pins/${resourceType}/${resourceId}`, {
      method: "DELETE",
    }),
};

export const docID = (d: Partial<Document>) => d.id || "";
export const docTitle = (d: Partial<Document>) => d.title || "Untitled";
export const currentVersionID = (d: Partial<Document>) => d.current_version_id || "";
export const versionID = (v: Partial<Version>) => v.id || "";

export function repositoryNamespaceName(repository: Partial<Repository>) {
  return (
    repository.namespace_name ||
    repository.namespace?.name ||
    repository.namespace?.slug ||
    repository.owner?.name ||
    repository.owner?.email ||
    repository.owner?.id ||
    (repository.scope === "organization" ? "Organization" : "Personal")
  );
}
