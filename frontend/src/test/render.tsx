import type { ReactElement, ReactNode } from "react";
import { render } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// renderWithProviders mounts a component with a fresh QueryClient per test, so
// no cache or retry state leaks between cases. Retries are off so a rejected
// query surfaces its error immediately instead of after a backoff. The client
// is returned alongside the render result so a test can wait for a specific
// query to settle before asserting a negative -- otherwise the assertion
// passes on first paint, before any response has arrived.
export function renderWithProviders(ui: ReactElement) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  const Wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return { ...render(ui, { wrapper: Wrapper }), client };
}
