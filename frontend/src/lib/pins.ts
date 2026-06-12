import { createPersistentStore } from "@/lib/persistent-store";

const store = createPersistentStore<string[]>(
  "aidocs:pinned_docs",
  [],
  (p) => {
    if (Array.isArray(p))
      return (p as unknown[]).filter((x): x is string => typeof x === "string");
    return [];
  },
);

export function usePins(): Set<string> {
  return new Set(store.useStore());
}

export function togglePin(id: string) {
  const current = store.read();
  if (current.includes(id)) {
    store.write(current.filter((x) => x !== id));
  } else {
    store.write([...current, id]);
  }
}
