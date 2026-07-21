import { APIError } from "@/api";

// errorMessage extracts a human-readable message from an unknown thrown value,
// falling back to the supplied default when none is available.
export function errorMessage(e: unknown, fallback = "Something went wrong."): string {
  if (e instanceof Error && e.message) return e.message;
  return fallback;
}

// isVersionConflict reports whether a failed version create was rejected
// because base_version_id was stale — i.e. somebody else saved first. The
// server returns 409 with code "version_conflict"
// (api/internal/server/handlers_versions.go).
export function isVersionConflict(e: unknown): boolean {
  return e instanceof APIError && e.status === 409 && e.code === "version_conflict";
}
