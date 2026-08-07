export function splitChangeSummary(raw: string): string[] {
  const trimmed = raw.trim();
  if (!trimmed) return [];
  const parts = trimmed.includes("\n") ? trimmed.split(/\r?\n/) : trimmed.split("; ");
  return parts
    .map((part) =>
      part
        .trim()
        .replace(/^[-•*]\s+/, "")
        .trim(),
    )
    .filter(Boolean);
}
