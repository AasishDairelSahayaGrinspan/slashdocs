import { renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { createPersistentStore } from "./persistent-store";

describe("createPersistentStore", () => {
  it("reads, writes, deserializes, and notifies subscribers", async () => {
    const store = createPersistentStore("sample", 1, (parsed) =>
      typeof parsed === "number" ? parsed : 1,
    );

    expect(store.read()).toBe(1);
    localStorage.setItem("sample", JSON.stringify(3));
    expect(store.read()).toBe(3);
    localStorage.setItem("sample", "bad json");
    expect(store.read()).toBe(1);

    const { result } = renderHook(() => store.useStore());
    const listener = vi.fn();
    const stop = window.addEventListener("aidocs:persist:sample", listener);
    expect(stop).toBeUndefined();
    store.write(5);

    await waitFor(() => expect(result.current).toBe(5));
    expect(listener).toHaveBeenCalledTimes(1);
  });
});
