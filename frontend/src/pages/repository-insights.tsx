import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  AlertTriangle,
  ArrowRight,
  FileText,
  Link2,
  Loader2,
  Network,
  Quote,
  Search,
  Send,
  Sparkles,
} from "lucide-react";
import { api, type KnowledgeCitation, type KnowledgeGraphNode } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input, Textarea } from "@/components/ui/input";
import { EmptyState, Skeleton } from "@/components/ui/misc";
import { errorMessage } from "@/lib/errors";
import { queryKeys } from "@/lib/queryKeys";

type InsightsView = "ask" | "graph";

export function RepositoryInsights({ repositoryId }: { repositoryId: string }) {
  const [view, setView] = useState<InsightsView>("ask");
  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 className="text-lg font-semibold">Knowledge insights</h2>
          <p className="mt-1 max-w-2xl text-sm text-[var(--color-fg-muted)]">
            Search across this repository, ask grounded questions, and explore how
            documents cite and depend on one another.
          </p>
        </div>
        <div className="flex rounded-lg bg-[var(--color-surface-muted)] p-1">
          <ViewButton active={view === "ask"} onClick={() => setView("ask")}>
            <Sparkles className="h-4 w-4" /> Search & ask
          </ViewButton>
          <ViewButton active={view === "graph"} onClick={() => setView("graph")}>
            <Network className="h-4 w-4" /> Graph & backlinks
          </ViewButton>
        </div>
      </div>
      {view === "ask" ? (
        <SearchAndAsk repositoryId={repositoryId} />
      ) : (
        <KnowledgeGraph repositoryId={repositoryId} />
      )}
    </div>
  );
}

function SearchAndAsk({ repositoryId }: { repositoryId: string }) {
  const [search, setSearch] = useState("");
  const [submittedSearch, setSubmittedSearch] = useState("");
  const [question, setQuestion] = useState("");
  const results = useQuery({
    queryKey: queryKeys.repositorySearch(repositoryId, submittedSearch),
    queryFn: () => api.searchRepositoryKnowledge(repositoryId, submittedSearch),
    enabled: !!submittedSearch,
  });
  const ask = useMutation({
    mutationFn: (value: string) => api.askRepositoryQuestion(repositoryId, value),
  });
  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(360px,0.8fr)]">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Search className="h-4 w-4" /> Search this repository
          </CardTitle>
        </CardHeader>
        <CardContent>
          <form
            className="flex gap-2"
            onSubmit={(event) => {
              event.preventDefault();
              setSubmittedSearch(search.trim());
            }}
          >
            <Input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search policies, concepts, owners…"
              aria-label="Search repository knowledge"
            />
            <Button type="submit" disabled={!search.trim()}>
              Search
            </Button>
          </form>
          <div className="mt-5">
            {!submittedSearch ? (
              <EmptyState
                className="py-12"
                icon={<Search className="h-5 w-5" />}
                title="Find an answer in the docs"
                description="Results include the document, matching context, path, and relevance."
              />
            ) : results.isLoading ? (
              <div className="space-y-3">
                <Skeleton className="h-28 w-full" />
                <Skeleton className="h-28 w-full" />
              </div>
            ) : results.error ? (
              <EmptyState
                icon={<AlertTriangle className="h-5 w-5" />}
                title="Search failed"
                description={errorMessage(results.error)}
                action={
                  <Button variant="outline" onClick={() => results.refetch()}>
                    Try again
                  </Button>
                }
              />
            ) : (results.data?.items || []).length === 0 ? (
              <EmptyState
                title="No matching knowledge"
                description={`Nothing matched “${submittedSearch}”. Try a broader phrase.`}
              />
            ) : (
              <ol className="space-y-3">
                {(results.data?.items || []).map((result, index) => {
                  const document = result.document;
                  const documentId = result.document_id || document?.id;
                  return (
                    <li
                      key={result.id || `${documentId}-${index}`}
                      className="rounded-xl border border-[var(--color-border)] p-4"
                    >
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <div className="flex items-center gap-2">
                            <FileText className="h-4 w-4 shrink-0 text-[var(--color-accent)]" />
                            <span className="truncate font-semibold">
                              {result.title || document?.title || "Untitled"}
                            </span>
                          </div>
                          {result.path ? (
                            <div className="mt-1 truncate text-xs text-[var(--color-fg-muted)]">
                              {result.path}
                            </div>
                          ) : null}
                        </div>
                        {typeof result.score === "number" ? (
                          <Badge variant="muted">
                            {Math.round(result.score * (result.score <= 1 ? 100 : 1))}%
                          </Badge>
                        ) : null}
                      </div>
                      {result.excerpt || result.snippet || result.quote ? (
                        <p className="mt-3 line-clamp-3 text-sm leading-6 text-[var(--color-fg-muted)]">
                          {result.excerpt || result.snippet || result.quote}
                        </p>
                      ) : null}
                      {documentId ? (
                        <Button
                          asChild
                          variant="link"
                          className="mt-2 h-auto min-h-0 p-0"
                        >
                          <Link
                            to={`/app/d/${documentId}?repo=${encodeURIComponent(repositoryId)}`}
                          >
                            Open document <ArrowRight className="h-3.5 w-3.5" />
                          </Link>
                        </Button>
                      ) : null}
                    </li>
                  );
                })}
              </ol>
            )}
          </div>
        </CardContent>
      </Card>

      <Card className="self-start">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Sparkles className="h-4 w-4 text-[var(--color-accent)]" /> Ask the docs
          </CardTitle>
        </CardHeader>
        <CardContent>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (question.trim()) ask.mutate(question.trim());
            }}
          >
            <Textarea
              value={question}
              onChange={(event) => setQuestion(event.target.value)}
              placeholder="What is the approval process for production access?"
              rows={4}
            />
            <Button
              className="mt-3 w-full"
              type="submit"
              disabled={!question.trim() || ask.isPending}
            >
              {ask.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Send className="h-4 w-4" />
              )}
              {ask.isPending ? "Reading the docs…" : "Ask with citations"}
            </Button>
          </form>
          {ask.error ? (
            <p className="mt-4 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">
              {errorMessage(ask.error, "Could not answer this question")}
            </p>
          ) : null}
          {ask.data ? (
            <div className="mt-5 border-t border-[var(--color-border)] pt-5">
              <p className="whitespace-pre-wrap text-sm leading-7">{ask.data.answer}</p>
              <CitationList
                citations={ask.data.citations || []}
                repositoryId={repositoryId}
              />
            </div>
          ) : (
            <div className="mt-5 rounded-xl bg-[var(--color-surface-muted)]/60 p-4 text-xs leading-5 text-[var(--color-fg-muted)]">
              Answers are grounded only in documents you can access. Every answer links
              back to its supporting passages.
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function KnowledgeGraph({ repositoryId }: { repositoryId: string }) {
  const [selectedId, setSelectedId] = useState("");
  const graph = useQuery({
    queryKey: queryKeys.repositoryGraph(repositoryId),
    queryFn: () => api.getRepositoryGraph(repositoryId),
  });
  const nodes = graph.data?.nodes || [];
  const selected = nodes.find((node) => node.id === selectedId) || nodes[0];
  const selectedDocumentId =
    selected?.document_id || (selected?.type === "document" ? selected.id : "");
  const backlinks = useQuery({
    queryKey: queryKeys.repositoryBacklinks(repositoryId, selectedDocumentId || ""),
    queryFn: () => api.listRepositoryBacklinks(repositoryId, selectedDocumentId),
    enabled: !!selectedDocumentId,
  });
  const backlinkCitations = (backlinks.data?.items || []).map((citation) => {
    const source = nodes.find((node) => node.id === citation.document_id);
    return {
      ...citation,
      title: source ? nodeLabel(source) : citation.title,
    };
  });

  if (graph.isLoading) {
    return <Skeleton className="h-[36rem] w-full" />;
  }
  if (graph.error) {
    return (
      <EmptyState
        icon={<AlertTriangle className="h-5 w-5" />}
        title="Could not build the knowledge graph"
        description={errorMessage(graph.error)}
        action={
          <Button variant="outline" onClick={() => graph.refetch()}>
            Try again
          </Button>
        }
      />
    );
  }
  if (nodes.length === 0) {
    return (
      <EmptyState
        icon={<Network className="h-5 w-5" />}
        title="No document links yet"
        description="Links, citations, and shared concepts will form a graph as this repository grows."
      />
    );
  }
  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_340px]">
      <Card className="overflow-hidden">
        <CardHeader className="border-b border-[var(--color-border)]">
          <div className="flex items-center justify-between gap-3">
            <CardTitle>Document graph</CardTitle>
            <Badge variant="muted">
              {nodes.length} nodes · {(graph.data?.edges || []).length} links
            </Badge>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <GraphCanvas
            nodes={nodes}
            edges={graph.data?.edges || []}
            selectedId={selected?.id}
            onSelect={setSelectedId}
          />
        </CardContent>
      </Card>
      <Card className="self-start">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Link2 className="h-4 w-4" /> Backlinks
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="mb-4 rounded-lg bg-[var(--color-surface-muted)] p-3">
            <div className="font-medium">{nodeLabel(selected)}</div>
            {selected?.path ? (
              <div className="mt-1 truncate text-xs text-[var(--color-fg-muted)]">
                {selected.path}
              </div>
            ) : null}
          </div>
          {!selectedDocumentId ? (
            <p className="text-sm text-[var(--color-fg-muted)]">
              Select a document node to inspect its backlinks.
            </p>
          ) : backlinks.isLoading ? (
            <Skeleton className="h-32 w-full" />
          ) : backlinks.error ? (
            <p className="text-sm text-[var(--color-danger)]">
              Could not load backlinks.
            </p>
          ) : backlinkCitations.length === 0 ? (
            <p className="text-sm text-[var(--color-fg-muted)]">
              No other documents link here yet.
            </p>
          ) : (
            <CitationList
              citations={backlinkCitations}
              repositoryId={repositoryId}
              compact
            />
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function GraphCanvas({
  nodes,
  edges,
  selectedId,
  onSelect,
}: {
  nodes: KnowledgeGraphNode[];
  edges: Array<{ source: string; target: string; type?: string }>;
  selectedId?: string;
  onSelect: (id: string) => void;
}) {
  const visible = nodes.slice(0, 28);
  const positions = useMemo(() => {
    const map = new Map<string, { x: number; y: number }>();
    visible.forEach((node, index) => {
      const ring = index < 1 ? 0 : index < 9 ? 1 : 2;
      const ringItems =
        ring === 0
          ? 1
          : ring === 1
            ? Math.min(8, visible.length - 1)
            : Math.max(1, visible.length - 9);
      const ringIndex = ring === 0 ? 0 : ring === 1 ? index - 1 : index - 9;
      const angle = (Math.PI * 2 * ringIndex) / ringItems - Math.PI / 2;
      const radius = ring === 0 ? 0 : ring === 1 ? 135 : 225;
      map.set(node.id, {
        x: 450 + Math.cos(angle) * radius,
        y: 250 + Math.sin(angle) * radius,
      });
    });
    return map;
  }, [visible]);
  return (
    <div className="h-[34rem] overflow-hidden bg-[radial-gradient(circle_at_center,var(--color-surface-muted),transparent_68%)]">
      <svg
        viewBox="0 0 900 500"
        className="h-full w-full"
        role="img"
        aria-label="Knowledge graph"
      >
        <g
          stroke="currentColor"
          className="text-[var(--color-border-strong)]"
          strokeWidth="1.2"
        >
          {edges.map((edge, index) => {
            const source = positions.get(edge.source);
            const target = positions.get(edge.target);
            if (!source || !target) return null;
            return (
              <line
                key={`${edge.source}-${edge.target}-${index}`}
                x1={source.x}
                y1={source.y}
                x2={target.x}
                y2={target.y}
                opacity={
                  selectedId && edge.source !== selectedId && edge.target !== selectedId
                    ? 0.25
                    : 0.8
                }
              />
            );
          })}
        </g>
        {visible.map((node) => {
          const position = positions.get(node.id)!;
          const selected = node.id === selectedId;
          return (
            <g
              key={node.id}
              transform={`translate(${position.x} ${position.y})`}
              onClick={() => onSelect(node.id)}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") onSelect(node.id);
              }}
              tabIndex={0}
              role="button"
              className="cursor-pointer outline-none"
            >
              <circle
                r={selected ? 29 : 24}
                fill={selected ? "var(--color-accent)" : "var(--color-surface)"}
                stroke={selected ? "var(--color-accent)" : "var(--color-border-strong)"}
                strokeWidth={selected ? 4 : 2}
              />
              <text
                textAnchor="middle"
                y="4"
                fill={selected ? "var(--color-accent-fg)" : "var(--color-fg)"}
                fontSize="10"
                fontWeight="600"
              >
                {(node.type || "doc").slice(0, 4).toUpperCase()}
              </text>
              <text
                textAnchor="middle"
                y={selected ? 48 : 43}
                fill="var(--color-fg)"
                fontSize="11"
                fontWeight="500"
              >
                {truncate(nodeLabel(node), 22)}
              </text>
            </g>
          );
        })}
      </svg>
    </div>
  );
}

function CitationList({
  citations,
  repositoryId,
  compact = false,
}: {
  citations: KnowledgeCitation[];
  repositoryId: string;
  compact?: boolean;
}) {
  if (!citations.length) return null;
  return (
    <div className={compact ? "space-y-2" : "mt-5 space-y-2"}>
      {!compact ? (
        <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-[var(--color-fg-muted)]">
          <Quote className="h-3.5 w-3.5" /> Sources
        </div>
      ) : null}
      {citations.map((citation, index) => {
        const content = (
          <div className="rounded-lg border border-[var(--color-border)] p-3 transition-colors hover:bg-[var(--color-surface-muted)]/55">
            <div className="text-sm font-medium">
              {citation.title || `Citation ${index + 1}`}
            </div>
            {citation.excerpt || citation.quote || citation.snippet ? (
              <p className="mt-1 line-clamp-2 text-xs leading-5 text-[var(--color-fg-muted)]">
                {citation.excerpt || citation.quote || citation.snippet}
              </p>
            ) : null}
          </div>
        );
        return citation.document_id ? (
          <Link
            key={`${citation.document_id}-${index}`}
            to={`/app/d/${citation.document_id}?repo=${encodeURIComponent(repositoryId)}`}
            className="block"
          >
            {content}
          </Link>
        ) : (
          <div key={index}>{content}</div>
        );
      })}
    </div>
  );
}

function ViewButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex items-center gap-2 rounded-md px-3 py-2 text-xs font-medium transition-colors ${
        active
          ? "bg-[var(--color-surface)] text-[var(--color-fg)] shadow-sm"
          : "text-[var(--color-fg-muted)] hover:text-[var(--color-fg)]"
      }`}
    >
      {children}
    </button>
  );
}

function nodeLabel(node?: KnowledgeGraphNode) {
  return node?.title || node?.label || node?.id || "Select a node";
}

function truncate(value: string, length: number) {
  return value.length > length ? `${value.slice(0, length - 1)}…` : value;
}
