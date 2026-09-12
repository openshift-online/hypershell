const githubRequestTimeoutMs = 8_000;
const githubOrgPageLimit = 5;

export interface GithubOrgGateInput {
  accessToken: string;
  allowlistRaw: string | undefined;
  fetchImpl?: typeof fetch;
  githubApiOrigin: string;
  oidcIssuer: string;
  onLookupError?: (error: unknown) => void;
  orgGate: string;
  username: string | undefined;
}

/** Splits a comma-separated GitHub username allowlist into lowercase names. */
export function parseUsernameAllowlist(raw: string | undefined): string[] {
  if (raw === undefined) {
    return [];
  }
  return raw
    .split(",")
    .map((entry) => entry.trim().toLowerCase())
    .filter((entry) => entry.length > 0);
}

/**
 * Reports whether a GitHub identity may use a pull-request environment.
 *
 * Allowlist entries are additive: a listed username is admitted even with no
 * org membership. Everyone else must belong to `orgGate`.
 */
export function githubIdentityAllowed(input: {
  allowlist: readonly string[];
  orgGate: string;
  orgLogins: readonly string[];
  username: string | undefined;
}): boolean {
  const username = input.username?.trim().toLowerCase();
  if (username !== undefined && username.length > 0) {
    if (input.allowlist.includes(username)) {
      return true;
    }
  }
  const orgGate = input.orgGate.trim().toLowerCase();
  if (orgGate.length === 0) {
    return false;
  }
  return input.orgLogins.some(
    (login) => login.trim().toLowerCase() === orgGate,
  );
}

/**
 * Evaluates the GitHub org gate for an OIDC callback.
 *
 * Allowlisted usernames skip the GitHub API. Public org membership is checked
 * next without a GitHub token, so members who keep their membership public
 * still get in when the OAuth App is not approved by the org. Private
 * membership uses the Keycloak-stored GitHub token against
 * `/user/memberships/orgs/{org}` (and `/user/orgs` as a fallback). Any
 * lookup failure is a denial.
 */
export async function evaluateGithubOrgGate(
  input: GithubOrgGateInput,
): Promise<boolean> {
  const allowlist = parseUsernameAllowlist(input.allowlistRaw);
  if (
    githubIdentityAllowed({
      allowlist,
      orgGate: input.orgGate,
      orgLogins: [],
      username: input.username,
    })
  ) {
    return true;
  }

  const fetchImpl = input.fetchImpl ?? globalThis.fetch;
  const origin = input.githubApiOrigin.replace(/\/+$/u, "");
  try {
    if (
      await isPublicOrgMember({
        fetchImpl,
        githubApiOrigin: origin,
        orgGate: input.orgGate,
        username: input.username,
      })
    ) {
      return true;
    }
    const githubToken = await fetchBrokerGithubToken({
      accessToken: input.accessToken,
      fetchImpl,
      oidcIssuer: input.oidcIssuer,
    });
    if (
      await isActiveOrgMember({
        fetchImpl,
        githubApiOrigin: origin,
        githubToken,
        orgGate: input.orgGate,
      })
    ) {
      return true;
    }
    const orgLogins = await fetchGithubOrgLogins({
      fetchImpl,
      githubApiOrigin: origin,
      githubToken,
    });
    return githubIdentityAllowed({
      allowlist,
      orgGate: input.orgGate,
      orgLogins,
      username: input.username,
    });
  } catch (error) {
    input.onLookupError?.(error);
    return false;
  }
}

function githubApiHeaders(token?: string): Record<string, string> {
  const headers: Record<string, string> = {
    Accept: "application/vnd.github+json",
    "User-Agent": "hypershell-web-console",
    "X-GitHub-Api-Version": "2022-11-28",
  };
  if (token !== undefined) {
    headers.Authorization = `Bearer ${token}`;
  }
  return headers;
}

/**
 * Public membership does not require an org-approved OAuth App. GitHub
 * returns 204 for public members and 404 for everyone else (including
 * private members).
 */
async function isPublicOrgMember(input: {
  fetchImpl: typeof fetch;
  githubApiOrigin: string;
  orgGate: string;
  username: string | undefined;
}): Promise<boolean> {
  const username = input.username?.trim();
  const orgGate = input.orgGate.trim();
  if (username === undefined || username.length === 0 || orgGate.length === 0) {
    return false;
  }
  const org = encodeURIComponent(orgGate);
  const login = encodeURIComponent(username);
  const response = await input.fetchImpl(
    `${input.githubApiOrigin}/orgs/${org}/public_members/${login}`,
    {
      headers: githubApiHeaders(),
      signal: AbortSignal.timeout(githubRequestTimeoutMs),
    },
  );
  return response.status === 204;
}

/**
 * Private (and public) membership for the authenticated user. A 403 usually
 * means the OAuth App is blocked by the org's third-party access policy, in
 * which case `/user/orgs` also omits the org.
 */
async function isActiveOrgMember(input: {
  fetchImpl: typeof fetch;
  githubApiOrigin: string;
  githubToken: string;
  orgGate: string;
}): Promise<boolean> {
  const orgGate = input.orgGate.trim();
  if (orgGate.length === 0) {
    return false;
  }
  const response = await input.fetchImpl(
    `${input.githubApiOrigin}/user/memberships/orgs/${encodeURIComponent(orgGate)}`,
    {
      headers: githubApiHeaders(input.githubToken),
      signal: AbortSignal.timeout(githubRequestTimeoutMs),
    },
  );
  if (response.status === 404) {
    return false;
  }
  if (response.status === 403) {
    throw new Error(
      `GitHub org membership failed with HTTP 403 (OAuth App may not be approved by ${orgGate})`,
    );
  }
  if (!response.ok) {
    throw new Error(
      `GitHub org membership failed with HTTP ${String(response.status)}`,
    );
  }
  const body: unknown = await response.json();
  return (
    typeof body === "object" &&
    body !== null &&
    "state" in body &&
    body.state === "active"
  );
}

async function fetchGithubOrgLogins(input: {
  fetchImpl: typeof fetch;
  githubApiOrigin: string;
  githubToken: string;
}): Promise<string[]> {
  const logins: string[] = [];
  // Membership visibility depends on the GitHub IdP requesting `read:org`
  // (deploy/base/keycloak GitHub identity provider defaultScope). Orgs that
  // restrict third-party OAuth Apps omit themselves from this list.
  const orgList = `${input.githubApiOrigin}/user/orgs?per_page=100`;
  let nextUrl: string | undefined = orgList;

  for (
    let page = 0;
    page < githubOrgPageLimit && nextUrl !== undefined;
    page++
  ) {
    const response = await input.fetchImpl(nextUrl, {
      headers: githubApiHeaders(input.githubToken),
      signal: AbortSignal.timeout(githubRequestTimeoutMs),
    });
    if (!response.ok) {
      throw new Error(
        `GitHub org listing failed with HTTP ${String(response.status)}`,
      );
    }
    const body: unknown = await response.json();
    if (!Array.isArray(body)) {
      throw new Error("GitHub org listing returned a non-array body");
    }
    for (const entry of body) {
      const login = githubOrgLogin(entry);
      if (login !== undefined) {
        logins.push(login);
      }
    }
    nextUrl = nextLinkFrom(response.headers.get("link"));
  }

  return logins;
}

async function fetchBrokerGithubToken(input: {
  accessToken: string;
  fetchImpl: typeof fetch;
  oidcIssuer: string;
}): Promise<string> {
  // Requires the GitHub IdP to set storeToken and addReadTokenRoleOnCreate
  // so this user's access token can read the stored GitHub token. Without
  // the broker read-token role Keycloak returns 403 and the org gate denies.
  const issuer = input.oidcIssuer.replace(/\/+$/u, "");
  const response = await input.fetchImpl(`${issuer}/broker/github/token`, {
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${input.accessToken}`,
    },
    signal: AbortSignal.timeout(githubRequestTimeoutMs),
  });
  if (!response.ok) {
    throw new Error(
      `Keycloak GitHub broker token failed with HTTP ${String(response.status)}`,
    );
  }
  const contentType = response.headers.get("content-type") ?? "";
  const body = await response.text();
  const token = readBrokerAccessToken(body, contentType);
  if (token === undefined || token.length === 0) {
    throw new Error(
      "Keycloak GitHub broker token response had no access_token",
    );
  }
  return token;
}

function readBrokerAccessToken(
  body: string,
  contentType: string,
): string | undefined {
  const trimmed = body.trim();
  if (trimmed.length === 0) {
    return undefined;
  }
  // Keycloak 26 retrieveToken sets Content-Type application/json for whatever
  // GitHub stored, including the default form-encoded access_token=... body.
  if (contentType.includes("json") || trimmed.startsWith("{")) {
    const fromJson = jsonAccessToken(trimmed);
    if (fromJson !== undefined) {
      return fromJson;
    }
  }
  if (
    contentType.includes("application/x-www-form-urlencoded") ||
    trimmed.includes("access_token=")
  ) {
    return new URLSearchParams(trimmed).get("access_token") ?? undefined;
  }
  return undefined;
}

function jsonAccessToken(body: string): string | undefined {
  try {
    const parsed: unknown = JSON.parse(body);
    if (
      typeof parsed === "object" &&
      parsed !== null &&
      "access_token" in parsed &&
      typeof parsed.access_token === "string"
    ) {
      return parsed.access_token;
    }
  } catch {
    return undefined;
  }
  return undefined;
}

function githubOrgLogin(entry: unknown): string | undefined {
  if (typeof entry !== "object" || entry === null || !("login" in entry)) {
    return undefined;
  }
  return typeof entry.login === "string" ? entry.login : undefined;
}

function nextLinkFrom(linkHeader: string | null): string | undefined {
  if (linkHeader === null || linkHeader.length === 0) {
    return undefined;
  }
  for (const part of linkHeader.split(",")) {
    const match = /<([^>]+)>\s*;\s*rel="next"/u.exec(part);
    const url = match?.[1];
    if (url !== undefined) {
      return url;
    }
  }
  return undefined;
}
