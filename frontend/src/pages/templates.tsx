import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BookCopy,
  Check,
  FileStack,
  LayoutTemplate,
  Search,
  Sparkles,
} from "lucide-react";
import { toast } from "sonner";
import { api, type AidocsTemplate } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input, Label } from "@/components/ui/input";
import { EmptyState, Skeleton } from "@/components/ui/misc";
import { errorMessage } from "@/lib/errors";
import { queryKeys } from "@/lib/queryKeys";
import { useTitle } from "@/lib/use-title";

export function TemplatesPage() {
  useTitle("Templates");
  const [search, setSearch] = useState("");
  const [category, setCategory] = useState("all");
  const [selected, setSelected] = useState<AidocsTemplate>();
  const templates = useQuery({
    queryKey: queryKeys.templates(),
    queryFn: api.listTemplates,
  });
  const items = useMemo(() => templates.data?.items || [], [templates.data?.items]);
  const categories = useMemo(
    () => [
      "all",
      ...new Set(
        items.map((item) => item.category).filter((item): item is string => !!item),
      ),
    ],
    [items],
  );
  const filtered = useMemo(() => {
    const term = search.trim().toLowerCase();
    return items.filter((item) => {
      if (category !== "all" && item.category !== category) return false;
      return (
        !term ||
        [item.name, item.description, item.category, ...(item.tags || [])]
          .filter(Boolean)
          .some((value) => String(value).toLowerCase().includes(term))
      );
    });
  }, [items, search, category]);
  return (
    <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6 sm:py-10">
      <div className="mb-7">
        <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-[var(--color-accent-muted)] text-[var(--color-accent)]">
          <LayoutTemplate className="h-5 w-5" />
        </div>
        <h1 className="mt-4 text-2xl font-semibold tracking-tight">
          Document templates
        </h1>
        <p className="mt-1 max-w-2xl text-sm text-[var(--color-fg-muted)]">
          Reuse proven structures for runbooks, product specs, policies, onboarding, and
          operational knowledge without overwriting published docs.
        </p>
      </div>
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="relative max-w-lg flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--color-fg-muted)]" />
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search templates…"
            className="pl-9"
          />
        </div>
        <div className="flex gap-1 overflow-x-auto rounded-lg bg-[var(--color-surface-muted)] p-1">
          {categories.map((item) => (
            <button
              key={item}
              type="button"
              onClick={() => setCategory(item)}
              className={`shrink-0 rounded-md px-3 py-1.5 text-xs font-medium capitalize ${category === item ? "bg-[var(--color-surface)] shadow-sm" : "text-[var(--color-fg-muted)]"}`}
            >
              {item}
            </button>
          ))}
        </div>
      </div>
      {templates.isLoading ? (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          <Skeleton className="h-64 w-full" />
          <Skeleton className="h-64 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      ) : templates.error ? (
        <EmptyState
          title="Could not load templates"
          description={errorMessage(templates.error)}
          action={
            <Button variant="outline" onClick={() => templates.refetch()}>
              Try again
            </Button>
          }
        />
      ) : !filtered.length ? (
        <EmptyState
          icon={<LayoutTemplate className="h-5 w-5" />}
          title={items.length ? "No matching templates" : "No templates available"}
          description={
            items.length
              ? "Try another search or category."
              : "Templates published for your workspace will appear here."
          }
        />
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {filtered.map((template) => (
            <Card key={template.id} className="group flex flex-col overflow-hidden">
              <div className="flex h-28 items-center justify-center border-b border-[var(--color-border)] bg-[linear-gradient(135deg,var(--color-accent-muted),var(--color-surface-muted))] text-[var(--color-accent)]">
                <FileStack className="h-9 w-9 transition-transform group-hover:-rotate-3 group-hover:scale-110" />
              </div>
              <CardHeader>
                <div className="mb-2 flex items-center justify-between gap-2">
                  {template.category ? (
                    <Badge variant="muted" className="capitalize">
                      {template.category}
                    </Badge>
                  ) : (
                    <span />
                  )}
                  {template.document_count ? (
                    <span className="text-xs text-[var(--color-fg-muted)]">
                      {template.document_count} docs
                    </span>
                  ) : null}
                </div>
                <CardTitle>{template.name}</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-1 flex-col">
                <p className="line-clamp-3 text-sm leading-6 text-[var(--color-fg-muted)]">
                  {template.description ||
                    "A reusable starting point for your documentation."}
                </p>
                {template.tags?.length ? (
                  <div className="mt-3 flex flex-wrap gap-1.5">
                    {template.tags.slice(0, 4).map((tag) => (
                      <span
                        key={tag}
                        className="rounded-full bg-[var(--color-surface-muted)] px-2 py-1 text-[10px] text-[var(--color-fg-muted)]"
                      >
                        {tag}
                      </span>
                    ))}
                  </div>
                ) : null}
                <Button className="mt-5 w-full" onClick={() => setSelected(template)}>
                  <Sparkles className="h-4 w-4" /> Use template
                </Button>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
      <ApplyTemplateDialog
        template={selected}
        onOpenChange={(open) => {
          if (!open) setSelected(undefined);
        }}
      />
    </div>
  );
}

function ApplyTemplateDialog({
  template,
  onOpenChange,
}: {
  template?: AidocsTemplate;
  onOpenChange: (open: boolean) => void;
}) {
  const nav = useNavigate();
  const qc = useQueryClient();
  const repositories = useQuery({
    queryKey: queryKeys.repositories(),
    queryFn: api.listRepositories,
    enabled: !!template,
  });
  const [repositoryId, setRepositoryId] = useState("");
  const [targetDocument, setTargetDocument] = useState("");
  const documents = useQuery({
    queryKey: queryKeys.repositoryDocuments(repositoryId),
    queryFn: () => api.listRepositoryDocuments(repositoryId),
    enabled: !!repositoryId,
  });
  const apply = useMutation({
    mutationFn: () =>
      api.applyTemplate(repositoryId, template!.id, targetDocument || undefined),
    onSuccess: (result) => {
      qc.invalidateQueries({ queryKey: queryKeys.repositoryDocuments(repositoryId) });
      qc.invalidateQueries({ queryKey: queryKeys.pullRequests(repositoryId) });
      toast.success(
        result.pull_request
          ? `Template proposed in pull request #${result.pull_request.number}.`
          : "Template applied.",
      );
      onOpenChange(false);
      if (result.pull_request)
        nav(`/app/repos/${repositoryId}/pulls/${result.pull_request.number}`);
      else nav(`/app/repos/${repositoryId}`);
    },
  });
  return (
    <Dialog open={!!template} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Use {template?.name}</DialogTitle>
          <DialogDescription>
            Choose an existing document. The template is proposed through a reviewable
            pull request.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(event) => {
            event.preventDefault();
            if (repositoryId && targetDocument) apply.mutate();
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="template-repository">Repository</Label>
            <select
              id="template-repository"
              value={repositoryId}
              onChange={(event) => {
                setRepositoryId(event.target.value);
                setTargetDocument("");
              }}
              disabled={repositories.isLoading}
              className="h-9 w-full rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 text-sm"
            >
              <option value="">
                {repositories.isLoading
                  ? "Loading repositories…"
                  : "Choose a repository"}
              </option>
              {(repositories.data?.items || []).map((repository) => (
                <option key={repository.id} value={repository.id}>
                  {repository.namespace_name || repository.owner?.name || "Personal"} /{" "}
                  {repository.name}
                </option>
              ))}
            </select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="template-target">Document to update</Label>
            <select
              id="template-target"
              value={targetDocument}
              onChange={(event) => setTargetDocument(event.target.value)}
              disabled={!repositoryId || documents.isLoading}
              className="h-9 w-full rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 text-sm disabled:opacity-50"
            >
              <option value="">
                {!repositoryId
                  ? "Choose a repository first"
                  : documents.isLoading
                    ? "Loading documents…"
                    : (documents.data?.items || []).length
                      ? "Choose a document"
                      : "This repository has no document to update"}
              </option>
              {(documents.data?.items || []).map((document) => (
                <option key={document.id} value={document.id}>
                  {document.title}
                </option>
              ))}
            </select>
          </div>
          <div className="rounded-xl bg-[var(--color-surface-muted)] p-3 text-sm text-[var(--color-fg-muted)]">
            <div className="flex items-center gap-2 font-medium text-[var(--color-fg)]">
              <Check className="h-4 w-4 text-[var(--color-success)]" /> Existing
              documents are never overwritten silently.
            </div>
            <p className="mt-1">
              Applying a template opens a pull request against the selected document, so
              its current content remains published until approval.
            </p>
          </div>
          {apply.error ? (
            <p role="alert" className="text-sm text-[var(--color-danger)]">
              {errorMessage(apply.error, "Could not apply template")}
            </p>
          ) : null}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={!repositoryId || !targetDocument || apply.isPending}
            >
              <BookCopy className="h-4 w-4" />{" "}
              {apply.isPending ? "Applying…" : "Apply template"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
