// React Query key factory. Keys share prefixes so related queries invalidate
// together.
export const queryKeys = {
  me: () => ["me"] as const,
  documents: () => ["documents"] as const,
  document: (id: string) => ["document", id] as const,
  versions: (doc: string) => ["versions", doc] as const,
  versionHTML: (doc: string, version: string) =>
    ["version-html", doc, version] as const,
  render: (version: string) => ["render", version] as const,
  comments: (doc: string) => ["comments", doc] as const,
  serviceAccounts: () => ["service-accounts"] as const,
  serviceAccountKeys: (id: string) => ["service-account-keys", id] as const,
  folders: (folderId?: string) => ["folders", folderId || "root"] as const,
  folder: (id: string) => ["folder", id] as const,
  folderAccess: (id: string) => ["folder-access", id] as const,
  documentAccess: (id: string) => ["document-access", id] as const,
  pins: () => ["pins"] as const,
};
