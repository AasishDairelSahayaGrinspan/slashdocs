import { renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  isFrontendSignedOut,
  setFrontendSignedOut,
  useFrontendSignedOut,
} from "./auth-state";

describe("auth-state", () => {
  it("stores sign-out state and updates hook consumers", async () => {
    const { result } = renderHook(() => useFrontendSignedOut());
    expect(result.current).toBe(false);
    expect(isFrontendSignedOut()).toBe(false);

    setFrontendSignedOut(true);
    await waitFor(() => expect(result.current).toBe(true));
    expect(isFrontendSignedOut()).toBe(true);

    setFrontendSignedOut(false);
    await waitFor(() => expect(result.current).toBe(false));
    expect(isFrontendSignedOut()).toBe(false);
  });
});
