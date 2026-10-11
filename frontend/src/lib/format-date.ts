export function formatTimestamp(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  });
}

// formatRelativeTime renders a short freshness label ("just now", "5m ago",
// "3h ago", "2d ago") for a version's created time so readers can judge at a
// glance whether a document is fresh or stale. Anything older than a week
// falls back to the medium date, since "12d ago" stops carrying signal.
export function formatRelativeTime(iso?: string, now: Date = new Date()): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const ms = now.getTime() - d.getTime();
  if (ms < 0) return formatTimestamp(iso);
  const minutes = Math.floor(ms / 60_000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days <= 7) return `${days}d ago`;
  return d.toLocaleDateString(undefined, { dateStyle: "medium" });
}
