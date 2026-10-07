// Tracks whether the user's session has gone stale, i.e. the BFF returned 401 for an
// API call. This is a tiny external store (not React state) because the signal originates
// in the TanStack QueryCache error handler, which lives OUTSIDE the React tree - see
// composition/query-client. The App subscribes via useSessionExpired and, when set, shows
// a "session expired / sign in again" takeover instead of a wall of generic error panels.
//
// Cookies/OIDC are owned by the OpenShift oauth-proxy sidecar that fronts the whole origin;
// the HTTP-only session cookie is not reachable from JS, so we cannot clear it ourselves.
// Recovery therefore has to go through the proxy's own login endpoint - see signInAgain.

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

// The ose-oauth-proxy sidecar runs under the "/oauth" prefix (its OpenShift default; the
// callback is "/oauth/callback"), NOT the upstream "/oauth2". "/oauth/sign_out" clears the
// session cookie server-side and redirects to "/", where the proxy serves its login page.
const SIGN_OUT_URL = "/oauth/sign_out";

/** Recover from an expired session by signing out of the oauth-proxy. A plain reload is NOT
 *  enough: the proxy's session cookie outlives the upstream access token (longer TTL, refresh
 *  disabled), so a reload finds the cookie still valid, skips re-auth, and keeps forwarding the
 *  dead token - the 401 just comes straight back. Navigating to the proxy's sign-out endpoint
 *  clears that cookie (it is HTTP-only, so JS cannot) and returns to the app, where the proxy,
 *  now seeing no session, serves its "Log in with OpenShift" page to start a fresh login. */
export function signInAgain(): void {
  window.location.assign(SIGN_OUT_URL);
}
