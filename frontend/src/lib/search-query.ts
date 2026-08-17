export type ActiveAuthorToken = {
  start: number;
  end: number;
  query: string;
};

export function hasDocumentSearchOperators(value: string): boolean {
  return /(?:^|\s)(?:author|labels|in):/i.test(value);
}

export function activeAuthorToken(
  value: string,
  cursor: number,
): ActiveAuthorToken | null {
  const position = Math.max(0, Math.min(cursor, value.length));
  let start = position;
  let end = position;
  while (start > 0 && !/\s/.test(value[start - 1])) start -= 1;
  while (end < value.length && !/\s/.test(value[end])) end += 1;
  const token = value.slice(start, end);
  const match = /^author:(.*)$/i.exec(token);
  if (!match) return null;
  const queryEnd = Math.max(start + "author:".length, position);
  const query = value.slice(start + "author:".length, queryEnd);
  if (query.startsWith("/")) return null;
  return { start, end, query };
}

export function replaceAuthorToken(
  value: string,
  token: ActiveAuthorToken,
  email: string,
): { value: string; cursor: number } {
  const replacement = `author:${email}`;
  const trailing = token.end === value.length ? " " : "";
  const next =
    value.slice(0, token.start) + replacement + trailing + value.slice(token.end);
  return { value: next, cursor: token.start + replacement.length + trailing.length };
}
