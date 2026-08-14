import * as oidc from "openid-client";

/**
 * A normalized OAuth token set held server-side by the BFF.
 *
 * The access token and its expiry live in the hot-path identity cookie; the
 * refresh and ID tokens live in the separate `session_tok` cookie so that no
 * single cookie approaches the browser per-cookie size limit.
 */
export interface TokenSet {
  accessToken: string;
  expiresAt?: number;
  idToken?: string;
  refreshToken?: string;
}

type GrantResult = Awaited<ReturnType<typeof oidc.refreshTokenGrant>>;

/** Refreshes the access token by exchanging a refresh token at the IdP. */
export type Refresher = (refreshToken: string) => Promise<TokenSet>;

// Access tokens are treated as expired this many seconds before their nominal
// expiry so a request never forwards a token that dies mid-flight upstream.
const skewSeconds = 30;

// Path characters at or below this code point (control chars and space) or the
// DEL character are rejected in a return-to path.
const maxRejectedControlCode = 0x20;
const deleteControlCode = 0x7f;

/**
 * Reports whether an access token should be refreshed before use. An unknown
 * expiry is treated as usable; the reactive upstream-401 path is the safety net
 * for that case, avoiding a needless re-authentication of a still-valid token.
 */
export function tokenExpired(
  expiresAt: number | undefined,
  nowMs: number = Date.now(),
): boolean {
  if (expiresAt === undefined) {
    return false;
  }
  return expiresAt - skewSeconds <= Math.floor(nowMs / 1000);
}

/** Converts an IdP token endpoint response into a normalized token set. */
export function toTokenSet(
  tokens: GrantResult,
  nowMs: number = Date.now(),
): TokenSet {
  const expiresIn = tokens.expiresIn();
  return {
    accessToken: tokens.access_token,
    ...(expiresIn === undefined
      ? {}
      : { expiresAt: Math.floor(nowMs / 1000) + expiresIn }),
    ...(tokens.id_token === undefined ? {} : { idToken: tokens.id_token }),
    ...(tokens.refresh_token === undefined
      ? {}
      : { refreshToken: tokens.refresh_token }),
  };
}

/**
 * Builds a single-flight refresher. Concurrent requests carrying the same
 * refresh token share one in-flight exchange, so a burst of parallel `/api/*`
 * calls performs a single token refresh and avoids refresh-token reuse
 * detection at the identity provider.
 */
export function createRefresher(
  oidcConfig: oidc.Configuration,
  nowMs: () => number = Date.now,
): Refresher {
  const inFlight = new Map<string, Promise<TokenSet>>();
  return (refreshToken) => {
    const existing = inFlight.get(refreshToken);
    if (existing) {
      return existing;
    }
    const pending = oidc
      .refreshTokenGrant(oidcConfig, refreshToken)
      .then((tokens) => toTokenSet(tokens, nowMs()));
    inFlight.set(refreshToken, pending);
    pending
      .catch(() => undefined)
      .finally(() => {
        inFlight.delete(refreshToken);
      });
    return pending;
  };
}

/**
 * Validates a caller-supplied post-login return path. Only same-origin absolute
 * paths are accepted; absolute URLs, scheme-relative (`//host`) values, and
 * anything without a leading single slash are rejected to prevent open
 * redirects. Returns the sanitized path or undefined.
 */
export function sanitizeReturnTo(value: unknown): string | undefined {
  if (typeof value !== "string" || value.length === 0) {
    return undefined;
  }
  // Must be a rooted path, but not protocol-relative ("//host") or a
  // backslash-obfuscated variant that some browsers treat as an authority.
  if (
    !value.startsWith("/") ||
    value.startsWith("//") ||
    value.startsWith("/\\")
  ) {
    return undefined;
  }
  // Reject raw control characters, space, and DEL that could smuggle a target.
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (code <= maxRejectedControlCode || code === deleteControlCode) {
      return undefined;
    }
  }
  return value;
}
