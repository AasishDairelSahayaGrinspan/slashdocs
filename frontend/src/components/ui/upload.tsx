import * as React from "react";
import { useRef, useState } from "react";
import { CheckCircle2, FileCode2, UploadCloud, X } from "lucide-react";
import { cn } from "@/lib/cn";

export function useStagedFile() {
  const [file, setFile] = useState<File | null>(null);
  return { file, setFile, reset: () => setFile(null) };
}

export function HtmlFileInput({
  onFile,
  className,
  ...rest
}: Omit<React.InputHTMLAttributes<HTMLInputElement>, "type" | "onChange"> & {
  onFile: (f: File | null) => void;
}) {
  return (
    <input
      type="file"
      accept=".html,text/html"
      onChange={(e) => onFile(e.target.files?.[0] || null)}
      className={className ?? "text-sm"}
      {...rest}
    />
  );
}

export function FileDropZone({
  file,
  onFile,
  className,
}: {
  file: File | null;
  onFile: (f: File | null) => void;
  className?: string;
}) {
  const [dragging, setDragging] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
    const dropped = e.dataTransfer.files[0];
    if (dropped && (dropped.name.toLowerCase().endsWith(".html") || dropped.type === "text/html")) {
      onFile(dropped);
    }
  };

  /* ── FILE SELECTED STATE ── */
  if (file) {
    const kb = (file.size / 1024).toFixed(0);
    return (
      <div className={cn("file-ready rounded-2xl overflow-hidden", className)}>
        {/* green success strip */}
        <div className="flex items-center gap-3 bg-[var(--color-success)]/8 border border-[var(--color-success)]/25 rounded-2xl px-4 py-4">
          <div className="relative flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-[var(--color-success)]/12">
            <FileCode2 className="h-5 w-5 text-[var(--color-success)]" />
            <span className="absolute -right-1 -top-1 flex h-4 w-4 items-center justify-center rounded-full bg-[var(--color-success)]">
              <CheckCircle2 className="h-3 w-3 text-white" strokeWidth={2.5} />
            </span>
          </div>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-semibold text-[var(--color-fg)]">{file.name}</p>
            <p className="mt-0.5 text-[11px] text-[var(--color-fg-muted)]">
              {kb} KB · HTML document · ready to upload
            </p>
          </div>
          <button
            type="button"
            onClick={() => onFile(null)}
            aria-label="Remove file"
            className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-[var(--color-fg-muted)] transition-colors hover:bg-[var(--color-surface-muted)] hover:text-[var(--color-fg)]"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
    );
  }

  /* ── EMPTY / DRAG-OVER STATE ── */
  return (
    <div
      role="button"
      tabIndex={0}
      aria-label="Upload HTML file — click or drop here"
      onClick={() => inputRef.current?.click()}
      onKeyDown={(e) => e.key === "Enter" && inputRef.current?.click()}
      onDragOver={(e) => { e.preventDefault(); setDragging(true); }}
      onDragEnter={(e) => { e.preventDefault(); setDragging(true); }}
      onDragLeave={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node)) setDragging(false);
      }}
      onDrop={handleDrop}
      className={cn(
        "drop-zone group relative cursor-pointer select-none rounded-2xl outline-none transition-all duration-200 focus-visible:outline-2 focus-visible:outline-[var(--color-accent)]",
        dragging && "dz-active",
        className,
      )}
    >
      {/* marching-ants corner pseudo-elements anchors */}
      <span className="dz-left rounded-l-2xl" />
      <span className="dz-right rounded-r-2xl" />

      {/* inner content */}
      <div
        className={cn(
          "flex flex-col items-center justify-center gap-4 rounded-2xl px-8 py-12 text-center transition-colors duration-200",
          dragging
            ? "bg-[var(--color-accent-muted)]"
            : "bg-[var(--color-surface-muted)] group-hover:bg-[var(--color-surface-muted)]",
        )}
      >
        {/* icon */}
        <div
          className={cn(
            "relative flex h-16 w-16 items-center justify-center rounded-2xl shadow-sm transition-all duration-200",
            dragging
              ? "bg-[var(--color-accent)] shadow-[0_0_0_8px_var(--color-accent-muted)]"
              : "bg-[var(--color-surface)] border border-[var(--color-border)] group-hover:border-[var(--color-border-strong)] group-hover:shadow-md",
          )}
        >
          <UploadCloud
            className={cn(
              "h-7 w-7 transition-all duration-200",
              dragging
                ? "text-white translate-y-[-2px]"
                : "text-[var(--color-fg-muted)] group-hover:text-[var(--color-accent)] group-hover:translate-y-[-2px]",
            )}
          />
        </div>

        {/* text */}
        <div className="space-y-1.5">
          <p className={cn(
            "text-base font-semibold transition-colors",
            dragging ? "text-[var(--color-accent)]" : "text-[var(--color-fg)]",
          )}>
            {dragging ? "Release to upload" : "Drop your HTML file here"}
          </p>
          <p className="text-sm text-[var(--color-fg-muted)]">
            or{" "}
            <span className={cn(
              "font-medium transition-colors",
              dragging ? "text-[var(--color-accent)]" : "text-[var(--color-accent)] underline underline-offset-2",
            )}>
              click to browse
            </span>
            {" "}— .html files only
          </p>
        </div>
      </div>

      <input
        ref={inputRef}
        type="file"
        accept=".html,text/html"
        onChange={(e) => onFile(e.target.files?.[0] || null)}
        className="hidden"
        tabIndex={-1}
      />
    </div>
  );
}
