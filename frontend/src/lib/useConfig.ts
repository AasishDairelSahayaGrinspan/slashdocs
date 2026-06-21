import { useQuery } from "@tanstack/react-query";
import { api } from "@/api";

// Fetches and indefinitely caches the server config (key `["config"]`).
// `staleTime: Infinity` keeps it fresh for the session without refetching.
export function useConfig() {
  return useQuery({
    queryKey: ["config"],
    queryFn: api.getConfig,
    staleTime: Infinity,
  });
}
