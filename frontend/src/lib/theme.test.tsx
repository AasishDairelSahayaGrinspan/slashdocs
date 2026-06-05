import { renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ThemeProvider, useResolvedTheme, useTheme } from "./theme";

function wrapper({ children }: { children: React.ReactNode }) {
  return <ThemeProvider>{children}</ThemeProvider>;
}

describe("theme", () => {
  it("persists theme selection and applies the dark class", async () => {
    const { result } = renderHook(() => useTheme(), { wrapper });
    expect(result.current.theme).toBe("system");

    result.current.setTheme("dark");
    await waitFor(() => expect(result.current.theme).toBe("dark"));
    await waitFor(() =>
      expect(document.documentElement.classList.contains("dark")).toBe(true),
    );

    result.current.setTheme("light");
    await waitFor(() => expect(result.current.theme).toBe("light"));
    await waitFor(() =>
      expect(document.documentElement.classList.contains("dark")).toBe(false),
    );
  });

  it("resolves the current concrete theme", () => {
    const { result } = renderHook(() => useResolvedTheme(), { wrapper });
    expect(result.current).toBe("light");
  });
});
