// Query client defaults. Bounded staleness; refetch on focus so an operator
// returning to the tab sees current data without any global polling loop.

import { QueryCache, QueryClient } from "@tanstack/react-query";

import { HttpError } from "../adapters/api/http-fleet-api";
import { markSessionExpired } from "../adapters/auth/session-expiry";

/** A stale/expired or unauthorized session: the BFF (behind oauth-proxy) returns 401 for an
 *  invalid token and 403 when the user lacks access. Both mean "re-authenticate", not "retry". */
function isAuthError(error: unknown): boolean {
  return (
    error instanceof HttpError && (error.status === 401 || error.status === 403)
  );
}

export function createQueryClient(): QueryClient {
  return new QueryClient({
    // A global cache error handler is the single choke point every query failure flows
    // through, so a stale session is detected once here (not per-hook) and flips the
    // app into its "sign in again" takeover.
    queryCache: new QueryCache({
      onError: (error) => {
        if (isAuthError(error)) {
          markSessionExpired();
        }
      },
    }),
    defaultOptions: {
      queries: {
        staleTime: 10_000,
        gcTime: 300_000,
        // Never retry an auth failure: it won't succeed without re-login and would just
        // hammer the BFF with repeat 401s. Other errors keep the prior 2-retry behavior.
        retry: (failureCount, error) => !isAuthError(error) && failureCount < 2,
        refetchOnWindowFocus: true,
        // Refetch intervals are set per-hook; background tabs stay paused
        // because refetchIntervalInBackground defaults to false.
      },
    },
  });
}
