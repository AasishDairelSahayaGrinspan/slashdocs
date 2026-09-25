import { Link } from "react-router-dom";
import { FileText, Folder as FolderIcon } from "lucide-react";
import { docID, docTitle, type FolderEntry } from "@/api";
import { formatTimestamp } from "@/lib/format-date";
import { principalLabel } from "@/lib/principals";

// Search results in the server's order, most relevant first. Like Slack, each
// document shows where it is, who owns it, and the text that matched.
export function SearchResults({ entries }: { entries: FolderEntry[] }) {
  return (
    <ul className="stagger-grid divide-y divide-[var(--color-border)] overflow-hidden rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)]">
      {entries.map(({ folder, document, last_active }) => {
        const id = folder?.id || docID(document!);
        const details = [
          principalLabel(folder?.owner || document?.owner),
          document?.parent_folder && `in ${document.parent_folder.title}`,
          formatTimestamp(last_active),
        ].filter(Boolean);
        return (
          <li key={id}>
            <Link
              to={folder ? `/app/folders/${id}` : `/app/d/${id}`}
              className="flex gap-3 px-4 py-3 transition-[background-color] duration-150 ease-out hover:bg-[var(--color-surface-muted)]"
            >
              {folder ? (
                <FolderIcon className="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-accent)]/70" />
              ) : (
                <FileText className="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-fg-muted)]" />
              )}
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-medium">
                  {folder?.title || docTitle(document!)}
                </div>
                <div className="truncate text-xs text-[var(--color-fg-muted)]">
                  {details.join(" · ")}
                </div>
                {document?.snippet ? <Snippet text={document.snippet} /> : null}
              </div>
            </Link>
          </li>
        );
      })}
    </ul>
  );
}

// The server puts U+0002 before and U+0003 after each match. Split at both,
// and the parts alternate between plain text and matches.
function Snippet({ text }: { text: string }) {
  const parts = text.split("\u0002").flatMap((part) => part.split("\u0003"));
  return (
    <p className="mt-1 line-clamp-2 text-sm text-[var(--color-fg-muted)]">
      {parts.map((part, index) =>
        index % 2 ? (
          <mark
            key={index}
            className="rounded-[3px] bg-[var(--color-accent)]/15 font-medium text-[var(--color-fg)]"
          >
            {part}
          </mark>
        ) : (
          part
        ),
      )}
    </p>
  );
}
