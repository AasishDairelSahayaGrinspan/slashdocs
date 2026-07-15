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
import { api, APIError } from "@/api";
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
  const protectedPullRequest = useMutation({
    mutationFn: (repositoryId: string) =>
      api.createPullRequest(repositoryId, {
        document_id: docId,
        base_version_id: baseVersion,
        title: summary.trim() || "Update document",
        change_summary: summary.trim(),
        file: file!,
      }),
    onSuccess: (pullRequest) => {
      setOpen(false);
      setSummary("");
      reset();
      toast.success(`Pull request #${pullRequest.number} opened.`);
      window.location.assign(
        `/app/repos/${pullRequest.repository_id}/pulls/${pullRequest.number}`,
      );
    },
    onError: (error) => toast.error(errorMessage(error, "Could not open pull request")),
  });
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
    onError: (e) => {
      if (e instanceof APIError && e.code === "pull_request_required") {
        const details =
          e.details && typeof e.details === "object"
            ? (e.details as Record<string, unknown>)
            : undefined;
        const repositoryId =
          typeof details?.repository_id === "string" ? details.repository_id : "";
        toast.error("This document is protected", {
          description:
            "Open a pull request from its repository to propose this change.",
          action: {
            label: repositoryId ? "Create pull request" : "Repositories",
            onClick: () =>
              repositoryId
                ? protectedPullRequest.mutate(repositoryId)
                : window.location.assign("/app/repositories"),
          },
        });
        return;
      }
      toast.error(errorMessage(e, "Upload failed"));
    },
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
          <Upload className="h-3.5 w-3.5" /> Upload replacement file (advanced)
        </DropdownItem>
      </SheetTrigger>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>Upload replacement file</SheetTitle>
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
            <Button type="submit" disabled={!file || m.isPending}>
              {m.isPending ? "Uploading…" : "Upload replacement"}
            </Button>
          </form>
        </SheetBody>
      </SheetContent>
    </Sheet>
  );
}
