import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Search } from "lucide-react";
import { api } from "@/api";
import { Input } from "@/components/ui/input";
import { queryKeys } from "@/lib/queryKeys";
import { activeAuthorToken, replaceAuthorToken } from "@/lib/search-query";

export function DocumentSearchInput({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [cursor, setCursor] = useState(value.length);
  const [focused, setFocused] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const token = useMemo(() => activeAuthorToken(value, cursor), [value, cursor]);
  const [authorQuery, setAuthorQuery] = useState("");

  useEffect(() => {
    const timeout = window.setTimeout(
      () => setAuthorQuery(token?.query.trim() || ""),
      150,
    );
    return () => window.clearTimeout(timeout);
  }, [token?.query]);

  const authors = useQuery({
    queryKey: queryKeys.userSearch(authorQuery),
    queryFn: () => api.searchUsers(authorQuery),
    enabled: authorQuery.length >= 2,
    staleTime: 30_000,
  });
  const suggestions = authors.data?.items || [];
  const open = focused && !!token && authorQuery.length >= 2;

  useEffect(() => setActiveIndex(0), [authorQuery]);

  function updateCursor() {
    setCursor(inputRef.current?.selectionStart ?? value.length);
  }

  function chooseAuthor(index: number) {
    const author = suggestions[index];
    if (!author?.email || !token) return;
    const next = replaceAuthorToken(value, token, author.email);
    onChange(next.value);
    setCursor(next.cursor);
    window.requestAnimationFrame(() => {
      inputRef.current?.focus();
      inputRef.current?.setSelectionRange(next.cursor, next.cursor);
    });
  }

  return (
    <div className="relative min-w-0 flex-1 sm:flex-initial">
      <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--color-fg-muted)]" />
      <Input
        ref={inputRef}
        value={value}
        onChange={(event) => {
          setFocused(true);
          onChange(event.target.value);
          setCursor(event.target.selectionStart ?? event.target.value.length);
        }}
        onClick={updateCursor}
        onKeyUp={updateCursor}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        onKeyDown={(event) => {
          if (!open || suggestions.length === 0) return;
          if (event.key === "ArrowDown") {
            event.preventDefault();
            setActiveIndex((index) => (index + 1) % suggestions.length);
          } else if (event.key === "ArrowUp") {
            event.preventDefault();
            setActiveIndex(
              (index) => (index - 1 + suggestions.length) % suggestions.length,
            );
          } else if (event.key === "Enter") {
            event.preventDefault();
            chooseAuthor(activeIndex);
          } else if (event.key === "Escape") {
            event.preventDefault();
            setFocused(false);
          }
        }}
        placeholder="Search or use author:, labels:, in:"
        className="w-full pl-9 sm:w-[360px]"
        role="combobox"
        aria-label="Search documents"
        aria-autocomplete="list"
        aria-expanded={open}
        aria-controls={open ? "document-author-suggestions" : undefined}
        aria-activedescendant={
          open && suggestions.length > 0
            ? `document-author-suggestion-${activeIndex}`
            : undefined
        }
      />
      {open ? (
        <div
          id="document-author-suggestions"
          role="listbox"
          aria-label="Author suggestions"
          className="absolute right-0 top-[calc(100%+6px)] z-50 w-full min-w-[320px] rounded-[12px] bg-[var(--color-surface)] p-1 shadow-lg"
        >
          {authors.isFetching ? (
            <p className="px-3 py-2 text-sm text-[var(--color-fg-muted)]">
              Searching authors…
            </p>
          ) : authors.error ? (
            <p className="px-3 py-2 text-sm text-[var(--color-danger)]">
              Could not load authors.
            </p>
          ) : suggestions.length === 0 ? (
            <p className="px-3 py-2 text-sm text-[var(--color-fg-muted)]">
              No matching authors.
            </p>
          ) : (
            suggestions.map((author, index) => (
              <button
                id={`document-author-suggestion-${index}`}
                key={author.id}
                type="button"
                role="option"
                aria-selected={index === activeIndex}
                className="flex min-h-10 w-full items-center rounded-lg px-3 py-2 text-left transition-[background-color,color] duration-150 ease-out hover:bg-[var(--color-surface-muted)] aria-selected:bg-[var(--color-surface-muted)]"
                onMouseDown={(event) => event.preventDefault()}
                onMouseEnter={() => setActiveIndex(index)}
                onClick={() => chooseAuthor(index)}
              >
                <span className="min-w-0">
                  <span className="block truncate text-sm font-medium">
                    {author.email}
                  </span>
                  {author.name ? (
                    <span className="block truncate text-xs text-[var(--color-fg-muted)]">
                      {author.name}
                    </span>
                  ) : null}
                </span>
              </button>
            ))
          )}
        </div>
      ) : null}
    </div>
  );
}
