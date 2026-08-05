type DocumentError = { title: string; description: string };

export function documentErrorState({
  failed,
  status,
  hasCachedCopy,
}: {
  failed: boolean;
  status: number | undefined;
  hasCachedCopy: boolean;
}): DocumentError | null {
  if (!failed) return null;
  if (status === 403) {
    return {
      title: "You don't have access to this document",
      description: "Please ask the document owner to grant you access.",
    };
  }
  if (status === 404) {
    return {
      title: "Document not found",
      description: "This document may have been deleted, or the link may be incorrect.",
    };
  }
  // Non-definitive error (network/5xx) with a cached copy: keep the readable
  // doc and its draft rather than replacing it; the next refetch recovers.
  if (hasCachedCopy) return null;
  return {
    title: "Could not load document",
    description: "Something went wrong while loading this document. Please try again.",
  };
}
