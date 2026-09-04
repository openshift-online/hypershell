/**
 * Host-owned adapter for the BFF browser session resource (`/auth/session`).
 *
 * This exposes only display identity and re-authentication state; it never
 * carries access, refresh, or ID tokens, which remain server-side per
 * WEB-AUTH-01/03. In no-auth mode the endpoint is absent, so a non-OK response
 * is treated as an unauthenticated session rather than an error.
 */
export interface BrowserSessionUser {
  email?: string;
  name?: string;
  preferredUsername?: string;
  sub?: string;
}

export interface BrowserSession {
  authenticated: boolean;
  /** True when the BFF exposes `/auth/session` (OIDC mode). */
  authEnabled: boolean;
  expiresAt?: number;
  roles: string[];
  user?: BrowserSessionUser;
}

export interface SessionGateway {
  getSession(signal?: AbortSignal): Promise<BrowserSession>;
}

const noAuthSession: BrowserSession = {
  authenticated: false,
  authEnabled: false,
  roles: [],
};

const unauthenticatedSession: BrowserSession = {
  authenticated: false,
  authEnabled: true,
  roles: [],
};

function optionalString(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function toBrowserSession(body: unknown): BrowserSession {
  if (
    typeof body !== "object" ||
    body === null ||
    (body as { authenticated?: unknown }).authenticated !== true
  ) {
    return unauthenticatedSession;
  }
  const record = body as {
    expires_at?: unknown;
    roles?: unknown;
    user?: unknown;
  };
  const rawUser =
    typeof record.user === "object" && record.user !== null
      ? (record.user as Record<string, unknown>)
      : {};
  const user: BrowserSessionUser = {};
  const email = optionalString(rawUser.email);
  const name = optionalString(rawUser.name);
  const preferredUsername = optionalString(rawUser.preferred_username);
  const sub = optionalString(rawUser.sub);
  if (email !== undefined) {
    user.email = email;
  }
  if (name !== undefined) {
    user.name = name;
  }
  if (preferredUsername !== undefined) {
    user.preferredUsername = preferredUsername;
  }
  if (sub !== undefined) {
    user.sub = sub;
  }

  return {
    authenticated: true,
    authEnabled: true,
    ...(typeof record.expires_at === "number"
      ? { expiresAt: record.expires_at }
      : {}),
    roles: Array.isArray(record.roles)
      ? record.roles.filter((role): role is string => typeof role === "string")
      : [],
    user,
  };
}

export function createSessionAdapter(
  fetchImplementation: typeof globalThis.fetch = globalThis.fetch,
): SessionGateway {
  return {
    async getSession(signal) {
      let response: Response;
      try {
        response = await fetchImplementation("/auth/session", {
          credentials: "same-origin",
          headers: { accept: "application/json" },
          ...(signal ? { signal } : {}),
        });
      } catch {
        // Network failure or absent endpoint (no-auth mode): treat as no session.
        return noAuthSession;
      }
      if (!response.ok) {
        return response.status === 404 ? noAuthSession : unauthenticatedSession;
      }
      try {
        return toBrowserSession(await response.json());
      } catch {
        return unauthenticatedSession;
      }
    },
  };
}
