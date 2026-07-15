import { useMemo, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, ArrowRight, BookOpen, FileText, Search } from "lucide-react";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { EmptyState, Skeleton } from "@/components/ui/misc";
import { queryKeys } from "@/lib/queryKeys";
import { useTitle } from "@/lib/use-title";

export function PublicPortalPage() {
  const { slug = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const [search, setSearch] = useState("");
  const portal = useQuery({
    queryKey: queryKeys.publicPortal(slug),
    queryFn: () => api.getPublicPortal(slug),
    enabled: !!slug,
    meta: { suppressGlobalError: true },
  });
  const documents = useMemo(
    () => portal.data?.documents || [],
    [portal.data?.documents],
  );
  const selectedDocument = documents.find(
    (document) => document.id === params.get("document"),
  );
  useTitle(
    selectedDocument?.title ||
      portal.data?.title ||
      portal.data?.repository?.name ||
      "Knowledge portal",
  );
  const filtered = useMemo(() => {
    const term = search.trim().toLowerCase();
    return documents.filter(
      (document) =>
        !term ||
        [document.title, document.path, document.excerpt]
          .filter(Boolean)
          .some((value) => String(value).toLowerCase().includes(term)),
    );
  }, [documents, search]);
  if (portal.isLoading) {
    return (
      <div className="min-h-screen bg-[var(--color-bg)]">
        <div className="mx-auto max-w-5xl px-6 py-12">
          <Skeleton className="h-12 w-72" />
          <Skeleton className="mt-6 h-40 w-full" />
          <div className="mt-8 grid gap-4 md:grid-cols-2">
            <Skeleton className="h-44 w-full" />
            <Skeleton className="h-44 w-full" />
          </div>
        </div>
      </div>
    );
  }
  if (portal.error || !portal.data) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[var(--color-bg)] p-6">
        <EmptyState
          icon={<BookOpen className="h-5 w-5" />}
          title="Portal not found"
          description="This knowledge portal may be private, unpublished, or the link may be incorrect."
          action={
            <Button asChild variant="outline">
              <Link to="/">Go to aidocs</Link>
            </Button>
          }
        />
      </div>
    );
  }
  const title = portal.data.title || portal.data.repository?.name || "Knowledge portal";
  const accent = portal.data.accent_color || "var(--color-accent)";
  return (
    <div
      className="min-h-screen bg-[var(--color-bg)]"
      style={{ "--portal-accent": accent } as React.CSSProperties}
    >
      <header className="border-b border-[var(--color-border)] bg-[var(--color-surface)]/85 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-4 sm:px-6">
          <Link
            to={`/portal/${slug}`}
            className="flex items-center gap-2 font-semibold"
          >
            <span
              className="flex h-9 w-9 items-center justify-center rounded-lg text-white"
              style={{ background: "var(--portal-accent)" }}
            >
              <BookOpen className="h-4 w-4" />
            </span>
            <span>{title}</span>
          </Link>
          <a
            href="https://aidocs.dev"
            className="text-xs text-[var(--color-fg-muted)] hover:text-[var(--color-fg)]"
          >
            Published with aidocs
          </a>
        </div>
      </header>
      <main>
        {selectedDocument ? (
          <article className="mx-auto max-w-4xl px-4 py-10 sm:px-6 sm:py-14">
            <button
              type="button"
              onClick={() => setParams({})}
              className="mb-8 inline-flex items-center gap-2 text-sm text-[var(--color-fg-muted)] transition-colors hover:text-[var(--color-fg)]"
            >
              <ArrowLeft className="h-4 w-4" /> Back to all documents
            </button>
            <div className="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] px-6 py-8 shadow-sm sm:px-10 sm:py-12">
              <div className="mb-3 text-xs font-medium uppercase tracking-[0.16em] text-[var(--color-fg-muted)]">
                Approved document
              </div>
              <h1 className="text-balance text-3xl font-semibold tracking-tight sm:text-4xl">
                {selectedDocument.title}
              </h1>
              {selectedDocument.path ? (
                <p className="mt-2 text-sm text-[var(--color-fg-muted)]">
                  {selectedDocument.path}
                </p>
              ) : null}
              <div className="my-8 h-px bg-[var(--color-border)]" />
              <div className="whitespace-pre-wrap text-[15px] leading-7 text-[var(--color-fg)]">
                {selectedDocument.text ||
                  selectedDocument.excerpt ||
                  "This document does not have published text yet."}
              </div>
            </div>
          </article>
        ) : (
          <>
            <section className="border-b border-[var(--color-border)] bg-[linear-gradient(135deg,var(--color-surface),var(--color-surface-muted))]">
              <div className="mx-auto max-w-6xl px-4 py-14 sm:px-6 sm:py-20">
                <div className="max-w-3xl">
                  <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1 text-xs text-[var(--color-fg-muted)]">
                    <BookOpen className="h-3.5 w-3.5" /> Approved knowledge
                  </div>
                  <h1 className="text-balance text-4xl font-semibold tracking-tight sm:text-5xl">
                    {title}
                  </h1>
                  <p className="mt-4 max-w-2xl text-pretty text-base leading-7 text-[var(--color-fg-muted)] sm:text-lg">
                    {portal.data.description ||
                      portal.data.repository?.description ||
                      "Browse current, reviewed documentation from this repository."}
                  </p>
                </div>
                {documents.length ? (
                  <div className="relative mt-8 max-w-2xl">
                    <Search className="pointer-events-none absolute left-4 top-1/2 h-5 w-5 -translate-y-1/2 text-[var(--color-fg-muted)]" />
                    <Input
                      value={search}
                      onChange={(event) => setSearch(event.target.value)}
                      placeholder="Search this knowledge base…"
                      className="h-12 rounded-xl bg-[var(--color-surface)] pl-12 text-base shadow-sm"
                    />
                  </div>
                ) : null}
              </div>
            </section>
            <section className="mx-auto max-w-6xl px-4 py-10 sm:px-6">
              {!documents.length ? (
                <EmptyState
                  icon={<FileText className="h-5 w-5" />}
                  title="No published documents"
                  description="Approved documents will appear here when this portal is published."
                />
              ) : !filtered.length ? (
                <EmptyState
                  icon={<Search className="h-5 w-5" />}
                  title="No matching documents"
                  description={`Nothing matched “${search}”.`}
                />
              ) : (
                <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
                  {filtered.map((document) => {
                    const href =
                      document.public_url ||
                      `?document=${encodeURIComponent(document.id)}`;
                    const content = (
                      <>
                        <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-[var(--color-surface-muted)] text-[var(--color-fg-muted)]">
                          <FileText className="h-5 w-5" />
                        </div>
                        <h2 className="mt-4 text-lg font-semibold group-hover:underline">
                          {document.title}
                        </h2>
                        {document.path ? (
                          <p className="mt-1 truncate text-xs text-[var(--color-fg-muted)]">
                            {document.path}
                          </p>
                        ) : null}
                        <p className="mt-3 line-clamp-3 min-h-[4.5rem] text-sm leading-6 text-[var(--color-fg-muted)]">
                          {document.excerpt ||
                            "Open the latest approved version of this document."}
                        </p>
                        <span
                          className="mt-5 flex items-center gap-1 text-sm font-medium"
                          style={{ color: "var(--portal-accent)" }}
                        >
                          Read document <ArrowRight className="h-4 w-4" />
                        </span>
                      </>
                    );
                    return document.public_url ? (
                      <a
                        key={document.id}
                        href={href}
                        className="group rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 transition-[transform,box-shadow] hover:-translate-y-0.5 hover:shadow-md"
                      >
                        {content}
                      </a>
                    ) : (
                      <Link
                        key={document.id}
                        to={href}
                        className="group rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 transition-[transform,box-shadow] hover:-translate-y-0.5 hover:shadow-md"
                      >
                        {content}
                      </Link>
                    );
                  })}
                </div>
              )}
            </section>
          </>
        )}
      </main>
      <footer className="border-t border-[var(--color-border)] py-8">
        <div className="mx-auto flex max-w-6xl items-center justify-between px-4 text-xs text-[var(--color-fg-muted)] sm:px-6">
          <span>{title}</span>
          <span>
            {documents.length} published document{documents.length === 1 ? "" : "s"}
          </span>
        </div>
      </footer>
    </div>
  );
}
