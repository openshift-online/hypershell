// Tracks whether the user's session has gone stale, i.e. the BFF returned 401 for an
// API call. This is a tiny external store (not React state) because the signal originates
// in the TanStack QueryCache error handler, which lives OUTSIDE the React tree - see
// composition/query-client. The App subscribes via useSessionExpired and, when set, shows
// a "session expired / sign in again" takeover instead of a wall of generic error panels.
//
// Cookies/OIDC are owned by the OpenShift oauth-proxy sidecar that fronts the whole origin;
// the HTTP-only session cookie is not reachable from JS, so we cannot clear it ourselves.
// Recovery therefore has to go through the proxy's own sign-out endpoint - see signInAgain.

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

// Endpoint path is the oauth-proxy default (no --proxy-prefix is set on the sidecar).
const SIGN_OUT_URL = "/oauth2/sign_out?rd=/";

/** Recover from an expired session by signing out of the oauth-proxy, then returning to the
 *  app. A plain reload is NOT enough: the proxy's session cookie outlives the upstream access
 *  token (it has a longer TTL and refresh is disabled), so a reload finds the cookie still
 *  valid, skips re-auth, and keeps forwarding the dead token - the 401 just comes straight
 *  back. Hitting sign_out clears that cookie server-side (the cookie is HTTP-only, so JS
 *  cannot), and rd=/ sends the now-cookieless browser back to the app, where the proxy
 *  redirects to the identity provider and mints a fresh session. */
export function signInAgain(): void {
  window.location.assign(SIGN_OUT_URL);
}
