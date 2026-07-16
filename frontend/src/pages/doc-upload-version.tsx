import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import { Upload } from "lucide-react";
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
import { HtmlFileInput, useStagedFile } from "@/components/ui/upload";
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
  const [resetApprovals, setResetApprovals] = useState(false);
  const { file, setFile, reset } = useStagedFile();
  const [open, setOpen] = useState(false);
  const m = useMutation({
    mutationFn: () =>
      api.createVersion(docId, baseVersion, summary, file!, resetApprovals),
    onSuccess: () => {
      toast.success("Version uploaded.");
      setOpen(false);
      setSummary("");
      setResetApprovals(false);
      reset();
      q.invalidateQueries({ queryKey: queryKeys.versions(docId) });
      q.invalidateQueries({ queryKey: queryKeys.document(docId) });
      q.invalidateQueries({ queryKey: queryKeys.comments(docId) });
      q.invalidateQueries({ queryKey: queryKeys.approvalsAll(docId) });
    },
    onError: (e) => toast.error(errorMessage(e, "Upload failed")),
  });
  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <DropdownItem
          onSelect={(e) => {
            e.preventDefault();
            setOpen(true);
          }}
          className="text-[var(--color-fg-muted)]"
        >
          <Upload className="h-3.5 w-3.5" /> Upload new version (backup)
        </DropdownItem>
      </SheetTrigger>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>Upload new version</SheetTitle>
        </SheetHeader>
        <SheetBody>
          <form
            className="space-y-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (file) m.mutate();
            }}
          >
            <Input
              placeholder="Change summary"
              value={summary}
              onChange={(e) => setSummary(e.target.value)}
            />
            <HtmlFileInput onFile={setFile} />
            <label className="flex items-center gap-2 text-xs text-[var(--color-fg-muted)]">
              <input
                type="checkbox"
                checked={resetApprovals}
                onChange={(e) => setResetApprovals(e.target.checked)}
              />
              Require everyone to re-approve (don&apos;t carry approvals forward)
            </label>
            <Button type="submit" disabled={!file || m.isPending}>
              {m.isPending ? "Uploading…" : "Upload version"}
            </Button>
          </form>
        </SheetBody>
      </SheetContent>
    </Sheet>
  );
}
