// Query client defaults. Bounded staleness; refetch on focus so an operator
// returning to the tab sees current data without any global polling loop.

import { QueryCache, QueryClient } from "@tanstack/react-query";

import { HttpError } from "../adapters/api/http-fleet-api";
import { markSessionExpired } from "../adapters/auth/session-expiry";

/** A stale/expired session: behind the oauth-proxy an XHR with an invalid/expired cookie
 *  comes back 401. Re-authenticating (a full-document reload) mints a fresh cookie, so this
 *  is the only status that should drive the "sign in again" takeover. */
function isSessionExpired(error: unknown): boolean {
  return error instanceof HttpError && error.status === 401;
}

/** An auth failure that a retry cannot fix: 401 (expired, needs re-login) or 403 (the
 *  authenticated user is forbidden). Neither recovers by hammering the BFF again. A 403 is
 *  deliberately NOT treated as session-expired: reloading re-auths the same identity and
 *  would 403 again, dead-ending a forbidden user in a reload loop -- it falls through to the
 *  normal error surface instead. */
function isUnrecoverableAuthError(error: unknown): boolean {
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
        if (isSessionExpired(error)) {
          markSessionExpired();
        }
      },
    }),
    defaultOptions: {
      queries: {
        staleTime: 10_000,
        gcTime: 300_000,
        // Never retry an auth failure (401 re-login or 403 forbidden): neither succeeds on
        // retry and would just hammer the BFF. Other errors keep the prior 2-retry behavior.
        retry: (failureCount, error) =>
          !isUnrecoverableAuthError(error) && failureCount < 2,
        refetchOnWindowFocus: true,
        // Refetch intervals are set per-hook; background tabs stay paused
        // because refetchIntervalInBackground defaults to false.
      },
    },
  });
}
