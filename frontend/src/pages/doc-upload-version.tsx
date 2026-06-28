import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import { GitBranch, Terminal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { DropdownItem } from "@/components/ui/dropdown";
import {
  Sheet,
  SheetBody,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { FileDropZone, useStagedFile } from "@/components/ui/upload";
import { api } from "@/api";
import { queryKeys } from "@/lib/queryKeys";

export function UploadVersionItem({
  docId,
  baseVersion,
}: {
  docId: string;
  baseVersion: string;
}) {
  const q = useQueryClient();
  const [summary, setSummary] = useState("");
  const { file, setFile, reset } = useStagedFile();
  const [open, setOpen] = useState(false);
  const m = useMutation({
    mutationFn: () => api.createVersion(docId, baseVersion, summary, file!),
    onSuccess: () => {
      toast.success("Version uploaded.");
      setOpen(false);
      setSummary("");
      reset();
      q.invalidateQueries({ queryKey: queryKeys.versions(docId) });
      q.invalidateQueries({ queryKey: queryKeys.document(docId) });
      q.invalidateQueries({ queryKey: queryKeys.comments(docId) });
    },
    onError: (e) => toast.error(errorMessage(e, "Upload failed")),
  });
  return (
    <Sheet open={open} onOpenChange={(v) => { setOpen(v); if (!v) { reset(); setSummary(""); } }}>
      <SheetTrigger asChild>
        <DropdownItem
          onSelect={(e) => {
            e.preventDefault();
            setOpen(true);
          }}
          className="text-[var(--color-fg)]"
        >
          <GitBranch className="h-3.5 w-3.5" /> Upload new version (backup)
        </DropdownItem>
      </SheetTrigger>
      <SheetContent>
        {/* gradient header matching the backup dialog style */}
        <SheetHeader className="border-b-0 bg-gradient-to-br from-[var(--color-accent-muted)] via-[var(--color-surface)] to-[var(--color-surface)] relative overflow-hidden">
          <div className="pointer-events-none absolute -right-4 -top-4 h-24 w-24 rounded-full bg-[var(--color-accent)]/8 blur-2xl" />
          <div className="relative flex items-center gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-[var(--color-accent)] shadow-lg shadow-[var(--color-accent)]/25">
              <GitBranch className="h-5 w-5 text-white" />
            </div>
            <div>
              <SheetTitle className="text-base font-bold">Upload new version</SheetTitle>
              <p className="mt-0.5 text-xs text-[var(--color-fg-muted)]">
                Manually publish a new HTML version
              </p>
            </div>
          </div>
        </SheetHeader>
        <SheetBody>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (file) m.mutate();
            }}
          >
            {/* drop zone — hero */}
            <FileDropZone file={file} onFile={setFile} />

            <div className="space-y-1.5">
              <label className="flex items-center gap-1.5 text-xs font-medium text-[var(--color-fg-muted)]">
                Change summary
                <span className="rounded bg-[var(--color-surface-muted)] px-1.5 py-0.5 text-[10px] text-[var(--color-fg-muted)]/60">optional</span>
              </label>
              <Input
                placeholder="What changed in this version?"
                value={summary}
                onChange={(e) => setSummary(e.target.value)}
                className="h-10"
              />
            </div>

            {/* CLI note — subtle */}
            <div className="flex items-center gap-2 rounded-lg bg-[var(--color-surface-muted)] px-3 py-2.5">
              <Terminal className="h-3 w-3 shrink-0 text-[var(--color-fg-muted)]/60" />
              <p className="text-[11px] leading-relaxed text-[var(--color-fg-muted)]/70">
                Agents publish via <code className="font-mono text-[10px]">aidocs docs upload</code> automatically. This is a manual fallback.
              </p>
            </div>

            <Button
              type="submit"
              className="w-full"
              disabled={!file || m.isPending}
            >
              {m.isPending ? (
                <span className="flex items-center gap-2">
                  <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-current border-t-transparent" />
                  Uploading…
                </span>
              ) : (
                <span className="flex items-center gap-2">
                  <GitBranch className="h-3.5 w-3.5" />
                  Upload version
                </span>
              )}
            </Button>
          </form>
        </SheetBody>
      </SheetContent>
    </Sheet>
  );
}
