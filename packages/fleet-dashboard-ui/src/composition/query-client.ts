// Query client defaults. Bounded staleness; refetch on focus so an operator
// returning to the tab sees current data without any global polling loop.

import { QueryClient } from "@tanstack/react-query";

export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 10_000,
        gcTime: 300_000,
        retry: 2,
        refetchOnWindowFocus: true,
        // Refetch intervals are set per-hook; background tabs stay paused
        // because refetchIntervalInBackground defaults to false.
      },
    },
  });
}
