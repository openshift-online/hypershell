// Fleet users plane: per-user operational usage derived from the api-server's
// per-user gauges. The username list arrives from the server (aggregated across
// instances); no user identity is baked in here. Totals and ordering are pure
// derivations so the table and summary cards share one source of truth.

/** One registered user's fleet usage, as the BFF reports it. */
export interface UserUsage {
  readonly username: string;
  /** Gateways the user owns (gateway:owner bindings), summed across instances. */
  readonly gateways: number;
  /** Active sandboxes across the user's owned gateways. */
  readonly sandboxes: number;
  /** Distinct spoke clusters the user's owned gateways run on (sorted). */
  readonly clusters: readonly string[];
}

export interface UsersData {
  readonly users: readonly UserUsage[];
}

/** Fleet-wide roll-up for the summary cards. */
export interface UserUsageTotals {
  /** Distinct registered users, including those owning nothing. */
  readonly users: number;
  /** Users owning at least one gateway. */
  readonly owners: number;
  readonly gateways: number;
  readonly sandboxes: number;
}

export function userUsageTotals(users: readonly UserUsage[]): UserUsageTotals {
  let owners = 0;
  let gateways = 0;
  let sandboxes = 0;
  for (const user of users) {
    if (user.gateways > 0) {
      owners++;
    }
    gateways += user.gateways;
    sandboxes += user.sandboxes;
  }
  return { users: users.length, owners, gateways, sandboxes };
}

/** Sort keys the usage table exposes; each maps to a UserUsage field. */
export type UserSortKey = "username" | "gateways" | "sandboxes";

export type SortDirection = "asc" | "desc";

/**
 * Sort a copy of the usage list. The default view is gateways descending so
 * the heaviest users read first; ties and the username key sort
 * lexicographically (using numeric comparison for counts).
 */
export function sortUserUsage(
  users: readonly UserUsage[],
  key: UserSortKey,
  direction: SortDirection,
): readonly UserUsage[] {
  const factor = direction === "asc" ? 1 : -1;
  return [...users].sort((a, b) => {
    let result: number;
    if (key === "username") {
      result = a.username.localeCompare(b.username);
    } else {
      result = (a[key] - b[key]) * factor;
      if (result === 0) {
        // Stable secondary order so equal counts do not reshuffle per render.
        result = a.username.localeCompare(b.username);
      }
    }
    return result;
  });
}
