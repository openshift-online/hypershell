/**
 * Who pays for what (cost dashboard, "Attribution to users").
 *
 * Every gateway is charged in full to one billing owner: the current
 * `gateway:owner` RoleBinding with the earliest created_at, ties broken by the
 * lowest RoleBinding ID. The gateway access API already returns grants sorted
 * that way (`sort=granted_at&order=asc`, then `rb.id ASC`), and `granted_at`
 * is the RoleBinding's created_at, which a role change keeps.
 *
 * All reads use the caller's own access token, so the API's authorization
 * decides which gateways and grants are visible.
 */

const pageSize = 100;
const accessConcurrency = 8;

export type WorkspaceMode = "managed" | "operator" | "shared";
export type GatewayAccessRole = "admin" | "owner" | "user";

export interface CostGateway {
  clusterId: string;
  id: string;
  name: string;
  namespace: string;
  workspaceMode: WorkspaceMode;
}

export interface BillingOwner {
  userId: string;
  username: string;
}

export interface CostGatewayAccess {
  billingOwner?: BillingOwner;
  /** The caller's own binding on the gateway; undefined when they hold none. */
  callerRole?: GatewayAccessRole;
}

interface GatewayListBody {
  items: {
    cluster_id?: string;
    id: string;
    name?: string;
    namespace: string;
    workspace_mode?: string;
  }[];
  page: number;
  total: number;
}

interface AccessListBody {
  capabilities?: { caller_role?: string };
  items: {
    granted_at: string;
    role: string;
    role_binding_id: string;
    user_id: string;
    username: string;
  }[];
  total: number;
}

const namespacePattern = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/u;

function authHeaders(accessToken: string | undefined): Record<string, string> {
  return accessToken ? { authorization: `Bearer ${accessToken}` } : {};
}

async function getJson<T>(
  url: URL,
  accessToken: string | undefined,
  timeoutMs: number,
): Promise<T> {
  const response = await fetch(url, {
    headers: authHeaders(accessToken),
    signal: AbortSignal.timeout(timeoutMs),
  });
  if (!response.ok) {
    throw new Error(
      `Cost attribution request failed: ${String(response.status)}`,
    );
  }
  return (await response.json()) as T;
}

function workspaceMode(value: string | undefined): WorkspaceMode {
  return value === "shared" || value === "operator" ? value : "managed";
}

/** List the gateways visible to the caller (every gateway for an admin). */
export async function listCostGateways(
  apiOrigin: string,
  accessToken: string | undefined,
  timeoutMs: number,
): Promise<CostGateway[]> {
  const gateways: CostGateway[] = [];
  let page = 1;
  let total = 0;
  do {
    const url = new URL("/api/hypershell/v1/gateways", apiOrigin);
    url.searchParams.set("page", String(page));
    url.searchParams.set("size", String(pageSize));
    const body = await getJson<GatewayListBody>(url, accessToken, timeoutMs);
    if (body.page !== page || body.total < 0 || body.items.length > pageSize) {
      throw new Error("Authorized gateway list response was inconsistent");
    }
    for (const item of body.items) {
      if (
        !namespacePattern.test(item.namespace) ||
        item.namespace.length > 63
      ) {
        throw new Error(
          "Authorized gateway list returned an invalid namespace",
        );
      }
      gateways.push({
        clusterId: item.cluster_id ?? "",
        id: item.id,
        name: item.name ?? item.id,
        namespace: item.namespace,
        workspaceMode: workspaceMode(item.workspace_mode),
      });
    }
    total = body.total;
    page += 1;
  } while ((page - 1) * pageSize < total);
  return gateways;
}

function asRole(value: string | undefined): GatewayAccessRole | undefined {
  return value === "owner" || value === "admin" || value === "user"
    ? value
    : undefined;
}

/** Pick the billing owner from owner grants: earliest granted_at, then lowest ID. */
export function selectBillingOwner(
  grants: readonly {
    granted_at: string;
    role: string;
    role_binding_id: string;
    user_id: string;
    username: string;
  }[],
): BillingOwner | undefined {
  let best: (typeof grants)[number] | undefined;
  let bestTime = Number.POSITIVE_INFINITY;
  for (const grant of grants) {
    if (grant.role !== "owner") continue;
    const time = Date.parse(grant.granted_at);
    if (!Number.isFinite(time)) continue;
    if (
      best === undefined ||
      time < bestTime ||
      (time === bestTime && grant.role_binding_id < best.role_binding_id)
    ) {
      best = grant;
      bestTime = time;
    }
  }
  return best === undefined
    ? undefined
    : { userId: best.user_id, username: best.username };
}

async function listAccess(
  apiOrigin: string,
  accessToken: string | undefined,
  timeoutMs: number,
  gatewayId: string,
  role?: GatewayAccessRole,
): Promise<AccessListBody> {
  const items: AccessListBody["items"] = [];
  let capabilities: AccessListBody["capabilities"];
  let page = 1;
  let total = 0;
  do {
    const url = new URL(
      `/api/hypershell/v1/gateways/${encodeURIComponent(gatewayId)}/access`,
      apiOrigin,
    );
    url.searchParams.set("page", String(page));
    url.searchParams.set("size", String(pageSize));
    url.searchParams.set("sort", "granted_at");
    url.searchParams.set("order", "asc");
    if (role) url.searchParams.set("role", role);
    const body = await getJson<AccessListBody>(url, accessToken, timeoutMs);
    capabilities ??= body.capabilities;
    items.push(...body.items);
    total = body.total;
    page += 1;
  } while ((page - 1) * pageSize < total && page <= 50);
  return { capabilities, items, total };
}

/**
 * Resolve the billing owner of each gateway and, when `caller` is given, the
 * caller's own role on it. Matching on the caller's username (not on the
 * API's effective tier) matters for platform admins: the API reports them as
 * owner-equivalent on every gateway, but My cost shows only their own
 * bindings (WEB-COST-09).
 */
export async function resolveGatewayAccess(
  apiOrigin: string,
  accessToken: string | undefined,
  timeoutMs: number,
  gateways: readonly CostGateway[],
  caller?: { username?: string },
): Promise<Map<string, CostGatewayAccess>> {
  const result = new Map<string, CostGatewayAccess>();
  const queue = [...gateways];
  const worker = async () => {
    for (let gateway = queue.shift(); gateway; gateway = queue.shift()) {
      const body = await listAccess(
        apiOrigin,
        accessToken,
        timeoutMs,
        gateway.id,
        caller ? undefined : "owner",
      );
      const access: CostGatewayAccess = {
        billingOwner: selectBillingOwner(body.items),
      };
      if (caller?.username === undefined && caller) {
        // No signed-in identity (local development without OIDC): use the
        // API's view of the caller.
        access.callerRole = asRole(body.capabilities?.caller_role);
      } else if (caller?.username !== undefined) {
        const username = caller.username.toLowerCase();
        const own = body.items.filter(
          (item) => item.username.toLowerCase() === username,
        );
        const rank = { owner: 3, admin: 2, user: 1 } as const;
        for (const item of own) {
          const role = asRole(item.role);
          if (
            role &&
            (access.callerRole === undefined ||
              rank[role] > rank[access.callerRole])
          ) {
            access.callerRole = role;
          }
        }
      }
      result.set(gateway.id, access);
    }
  };
  await Promise.all(
    Array.from(
      { length: Math.min(accessConcurrency, gateways.length) },
      worker,
    ),
  );
  return result;
}

interface MembershipListBody {
  items: { workspace_id: string }[];
  total: number;
}

interface AgentWorkspaceBody {
  gateway_id: string;
  name: string;
}

/**
 * Workspaces where the caller's OIDC subject is a member (AGT-WSP-02), as
 * `gateway id -> workspace names`. Best effort: a failure returns an empty map,
 * which can only hide workspace rows, never reveal extra ones.
 */
export async function listMemberWorkspaces(
  apiOrigin: string,
  accessToken: string | undefined,
  timeoutMs: number,
  subject: string | undefined,
): Promise<Map<string, Set<string>>> {
  const result = new Map<string, Set<string>>();
  if (!subject || !/^[A-Za-z0-9-]{1,128}$/u.test(subject)) return result;
  try {
    const url = new URL("/api/hypershell/ext/workspace_memberships", apiOrigin);
    url.searchParams.set("size", String(pageSize));
    url.searchParams.set("search", `subject = '${subject}'`);
    const memberships = await getJson<MembershipListBody>(
      url,
      accessToken,
      timeoutMs,
    );
    const ids = [
      ...new Set(memberships.items.map((item) => item.workspace_id)),
    ];
    await Promise.all(
      ids.slice(0, pageSize).map(async (id) => {
        const workspace = await getJson<AgentWorkspaceBody>(
          new URL(
            `/api/hypershell/ext/agent_workspaces/${encodeURIComponent(id)}`,
            apiOrigin,
          ),
          accessToken,
          timeoutMs,
        );
        const names = result.get(workspace.gateway_id) ?? new Set<string>();
        names.add(workspace.name);
        result.set(workspace.gateway_id, names);
      }),
    );
    return result;
  } catch {
    return new Map();
  }
}
