// Tracks whether the user's session has gone stale, i.e. the BFF returned 401 for an
// API call. This is a tiny external store (not React state) because the signal originates
// in the TanStack QueryCache error handler, which lives OUTSIDE the React tree - see
// composition/query-client. The App subscribes via useSessionExpired and, when set, shows
// a "session expired / sign in again" takeover instead of a wall of generic error panels.
//
// Cookies/OIDC are owned by the OpenShift oauth-proxy sidecar that fronts the whole origin;
// the HTTP-only session cookie is not reachable from JS, so we cannot clear it ourselves.
// The correct recovery is a full-document navigation (signInAgain), which the proxy answers
// with its identity-provider redirect, minting a fresh session cookie.

import { useSyncExternalStore } from "react";

let expired = false;
const listeners = new Set<() => void>();

/** Mark the session stale and notify subscribers. Idempotent: once expired, stays expired
 *  until the page navigates (a successful re-auth reloads the document). */
export function markSessionExpired(): void {
  if (expired) {
    return;
  }
  expired = true;
  for (const listener of listeners) {
    listener();
  }
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function getSnapshot(): boolean {
  return expired;
}

/** React binding for the session-expiry flag. */
export function useSessionExpired(): boolean {
  return useSyncExternalStore(subscribe, getSnapshot);
}

/** Re-run the oauth-proxy sign-in by reloading the top-level document. Unlike an XHR, a
 *  document navigation triggers the proxy's redirect to the identity provider, so the user
 *  lands back on the dashboard with a fresh session. */
export function signInAgain(): void {
  window.location.reload();
}
