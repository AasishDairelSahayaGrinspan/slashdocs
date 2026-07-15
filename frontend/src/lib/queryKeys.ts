// React Query key factory. Keys share prefixes so related queries invalidate
// together.
export const queryKeys = {
  me: () => ["me"] as const,
  userProfile: (id: string) => ["user-profile", id] as const,
  connectedAccounts: () => ["connected-accounts"] as const,
  config: () => ["config"] as const,
  documents: () => ["documents"] as const,
  document: (id: string) => ["document", id] as const,
  versions: (doc: string) => ["versions", doc] as const,
  render: (version: string) => ["render", version] as const,
  comments: (doc: string) => ["comments", doc] as const,
  serviceAccounts: () => ["service-accounts"] as const,
  serviceAccountKeys: (id: string) => ["service-account-keys", id] as const,
  // Prefix key matching every folder listing (root and nested alike); use it
  // to invalidate after moves and deletes that can touch any folder.
  foldersAll: () => ["folders"] as const,
  folders: (folderId?: string) => ["folders", folderId || "root"] as const,
  folder: (id: string) => ["folder", id] as const,
  // Prefix key matching folder access lists for every folder.
  folderAccessAll: () => ["folder-access"] as const,
  folderAccess: (id: string) => ["folder-access", id] as const,
  documentAccess: (id: string) => ["document-access", id] as const,
  pins: () => ["pins"] as const,
  userSearch: (query: string) => ["users", "search", query] as const,
  repositories: () => ["repositories"] as const,
  repository: (id: string) => ["repository", id] as const,
  repositoryDocuments: (id: string) => ["repository-documents", id] as const,
  repositorySettings: (id: string) => ["repository-settings", id] as const,
  repositorySourcesAll: () => ["repository-sources"] as const,
  repositorySources: (id: string) => ["repository-sources", id] as const,
  repositorySyncRuns: (id: string) => ["repository-sync-runs", id] as const,
  repositoryActivity: (id: string) => ["repository-activity", id] as const,
  repositorySearch: (id: string, query: string) =>
    ["repository-search", id, query] as const,
  repositoryGraph: (id: string) => ["repository-graph", id] as const,
  repositoryBacklinks: (id: string, documentId: string) =>
    ["repository-backlinks", id, documentId] as const,
  repositoryTeams: (id: string) => ["repository-teams", id] as const,
  pullRequests: (repositoryId: string) => ["pull-requests", repositoryId] as const,
  pullRequest: (repositoryId: string, number: number | string) =>
    ["pull-request", repositoryId, String(number)] as const,
  notifications: () => ["notifications"] as const,
  reviewInbox: () => ["review-inbox"] as const,
  templates: () => ["templates"] as const,
  publicPortal: (slug: string) => ["public-portal", slug] as const,
};
