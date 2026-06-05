import * as React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, type RenderOptions } from "@testing-library/react";
import { MemoryRouter, type MemoryRouterProps } from "react-router-dom";
import { TooltipProvider } from "@radix-ui/react-tooltip";
import { ThemeProvider } from "@/lib/theme";

type Options = Omit<RenderOptions, "wrapper"> & {
  route?: string;
  routerProps?: Omit<MemoryRouterProps, "children">;
};

export function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, refetchOnWindowFocus: false },
      mutations: { retry: false },
    },
  });
}

export function renderWithProviders(ui: React.ReactElement, options: Options = {}) {
  const client = createTestQueryClient();
  const initialEntries = [options.route || "/"];

  function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <ThemeProvider>
        <QueryClientProvider client={client}>
          <TooltipProvider delayDuration={0}>
            <MemoryRouter initialEntries={initialEntries} {...options.routerProps}>
              {children}
            </MemoryRouter>
          </TooltipProvider>
        </QueryClientProvider>
      </ThemeProvider>
    );
  }

  return { client, ...render(ui, { ...options, wrapper: Wrapper }) };
}
