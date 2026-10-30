// Framework-free route codec for the Glass dashboard (FLT-NAV-02, FLT-NAV-04).
//
// This module is the single source of truth for the typed route/query contract.
// It parses a location (path + search) into a normalized RouteState, serializes a
// RouteState back to a canonical location, and exposes pure navigation actions
// that transition one RouteState into another. It depends on nothing from React,
// `window`, or `history`; the driving adapter in src/adapters/routing wraps the
// browser History API around it.
//
// Design rules (FLT-NAV-02):
//  - Equivalent views produce equivalent URLs: defaults are omitted, enums reject
//    unknown values, numerics reject out-of-range values, and multi-value
//    parameters are ordered deterministically.
//  - Entities are addressed by stable identity (full bundle seed = digest/version,
//    collision-free run address tenant/namespace/runId, SDLC issue number, map
//    node id, gate id, step id), never by display text or row index.
//  - Unknown or malformed input degrades to a safe canonical view, never throws
//    (FLT-NAV-14).
//  - Every identifier is treated as opaque runtime data and round-trips
//    byte-for-byte through standard URL percent-encoding (FLT-NAV-03).

/** The three top-level tabs. The URL path carries this; all else is query state. */
export type TabKey = "map" | "sdlc" | "e2e";

const TAB_KEYS: readonly TabKey[] = ["map", "sdlc", "e2e"];

/** A table sort: a column index and a direction. */
export interface SortState {
  readonly col: number;
  readonly dir: "asc" | "desc";
}

/** Node-detail drawer sub-tabs on the Fleet Map. */
export type NodeTabKey = "details" | "bundle" | "agents" | "links";
const NODE_TAB_KEYS: readonly NodeTabKey[] = [
  "details",
  "bundle",
  "agents",
  "links",
];

/** Progress/Logs sub-tab of the shared execution detail. */
export type ExecTabKey = "progress" | "logs";
const EXEC_TAB_KEYS: readonly ExecTabKey[] = ["progress", "logs"];

/**
 * The shared execution-detail contract (FLT-NAV-11): used by both the Fleet Map
 * gate drawer and the E2E run detail. Only the sub-tab, the selected-step/whole-run
 * scope, the step-table sort, the expanded viewer, and the (coalesced) log search
 * are URL-represented here; follow/pause and the timestamp toggle are transient.
 */
export interface ExecState {
  readonly tab: ExecTabKey;
  /** Selected step id; null means whole-run log scope. */
  readonly step: string | null;
  /** Step-table sort; null means the default (execution order). */
  readonly stepSort: SortState | null;
  /** Expanded log viewer (the log modal) open. */
  readonly logExpanded: boolean;
  /** Log search text (coalesced via replaceState, FLT-NAV-06). */
  readonly logQuery: string;
}

/** Fleet Map tab state. */
export interface MapState {
  /** Selected node id (mutually exclusive with gate). */
  readonly node: string | null;
  /** Selected gate id (mutually exclusive with node). */
  readonly gate: string | null;
  /** Node drawer sub-tab (only meaningful with a node selected). */
  readonly nodeTab: NodeTabKey;
  /**
   * Gate pinned attempt as a full run address; null is the explicit live/latest
   * mode that follows the current attempt (FLT-NAV-10).
   */
  readonly attempt: string | null;
  /** Gate execution detail (FLT-NAV-11). */
  readonly exec: ExecState;
}

/** Agent SDLC tab state. */
export interface SdlcState {
  /** Scope multi-select, canonically ordered (FLT-NAV-13). */
  readonly scopes: readonly string[];
  /** Issue-number/title search (coalesced). */
  readonly query: string;
  /** Selected board card by issue number. */
  readonly card: number | null;
  /** Selected shipped issue by issue number. */
  readonly shipped: number | null;
  /** Bundle version context for the shipped selection. */
  readonly shippedVersion: string | null;
  /** Expanded shipped-bundle matrix row, addressed by its bundle seed, not index. */
  readonly shippedRow: string | null;
  /** Escalations notification drawer open. */
  readonly escalations: boolean;
}

/** E2E Runs tab state. */
export interface E2eState {
  /** Run search (coalesced). */
  readonly query: string;
  /** Runs-table sort; null means the default (Started, newest first). */
  readonly runSort: SortState | null;
  /** Selected run as a collision-free address tenant/namespace/runId. */
  readonly run: string | null;
  /** Detail drawer open; independent of the selection (FLT-NAV-12). */
  readonly drawer: boolean;
  /** Run execution detail (FLT-NAV-11). */
  readonly exec: ExecState;
}

/** The whole addressable navigation state of Glass. */
export interface RouteState {
  readonly tab: TabKey;
  /** Shared freight-bar bundle seed (digest, version fallback); drives both tabs. */
  readonly bundle: string | null;
  readonly map: MapState;
  readonly sdlc: SdlcState;
  readonly e2e: E2eState;
}

/** A framework-free location: the path and the raw query string (with or without `?`). */
export interface RouteLocation {
  readonly path: string;
  readonly search: string;
}

/**
 * Explicit live/latest gate mode for the `gattempt` parameter (FLT-NAV-10): the
 * view follows the current attempt instead of pinning one. Distinct from both an
 * absent attempt (a fresh open, which auto-pins the current attempt so it does not
 * drift per E2E-LOG-12) and a concrete pinned run address. Real run addresses always
 * contain slashes, so this short token never collides with one.
 */
export const GATE_LIVE_ATTEMPT = "live";

// --- Limits (FLT-NAV-02) -------------------------------------------------

/** Practical caps so a crafted or runaway URL cannot grow unbounded. */
export const ROUTE_LIMITS = {
  /** Maximum length of a single decoded identifier value. */
  value: 512,
  /** Maximum number of scope values retained. */
  scopes: 20,
  /** Maximum serialized URL length (path + search). */
  url: 2048,
} as const;

// --- Defaults --------------------------------------------------------------

const DEFAULT_EXEC: ExecState = {
  tab: "progress",
  step: null,
  stepSort: null,
  logExpanded: false,
  logQuery: "",
};

/** The canonical default view: the Fleet Map with nothing selected. */
export const DEFAULT_ROUTE: RouteState = {
  tab: "map",
  bundle: null,
  map: {
    node: null,
    gate: null,
    nodeTab: "details",
    attempt: null,
    exec: DEFAULT_EXEC,
  },
  sdlc: {
    scopes: [],
    query: "",
    card: null,
    shipped: null,
    shippedVersion: null,
    shippedRow: null,
    escalations: false,
  },
  e2e: {
    query: "",
    runSort: null,
    run: null,
    drawer: false,
    exec: DEFAULT_EXEC,
  },
};

// --- Primitive codecs ------------------------------------------------------

function clampValue(raw: string): string | null {
  // Reserved/Unicode characters round-trip through percent-encoding; we only
  // reject values beyond the practical length cap (FLT-NAV-02, FLT-NAV-03).
  if (raw.length === 0 || raw.length > ROUTE_LIMITS.value) {
    return null;
  }
  return raw;
}

function parseEnum<T extends string>(
  raw: string | null,
  allowed: readonly T[],
  fallback: T,
): T {
  return raw !== null && (allowed as readonly string[]).includes(raw)
    ? (raw as T)
    : fallback;
}

/** A positive integer identifier (issue number); rejects zero, negatives, junk. */
function parsePositiveInt(raw: string | null): number | null {
  if (raw === null || !/^[0-9]{1,15}$/.test(raw)) {
    return null;
  }
  const n = Number(raw);
  return Number.isSafeInteger(n) && n > 0 ? n : null;
}

/**
 * A sort as `col-dir`, e.g. `2-asc`. The `-` separator survives URL encoding
 * unescaped, keeping the URL readable. Rejects out-of-range columns and bad dirs.
 */
function parseSort(raw: string | null, columns: number): SortState | null {
  if (raw === null) {
    return null;
  }
  const m = /^([0-9]{1,3})-(asc|desc)$/.exec(raw);
  if (!m) {
    return null;
  }
  const col = Number(m[1]);
  if (!Number.isInteger(col) || col < 0 || col >= columns) {
    return null;
  }
  return { col, dir: m[2] as "asc" | "desc" };
}

function serializeSort(sort: SortState | null): string | null {
  return sort ? `${String(sort.col)}-${sort.dir}` : null;
}

function sameSort(a: SortState | null, b: SortState | null): boolean {
  return (
    a === b || (a !== null && b !== null && a.col === b.col && a.dir === b.dir)
  );
}

// Column counts, used to range-check sort indices on parse (FLT-NAV-02).
const RUN_SORT_COLUMNS = 7; // revision, attempt, started, duration, verdict, bundle, target
const STEP_SORT_COLUMNS = 3; // name, state, duration

/**
 * The runs table's default sort (Started column, newest first); an explicit sort
 * equal to it is the same view as the default, so it canonicalizes to null
 * (FLT-NAV-02, FLT-NAV-12). The steps table has no default sort (execution
 * order), so its default is already the null state.
 */
const RUN_DEFAULT_SORT: SortState = { col: 2, dir: "desc" };

// --- Exec codec (shared by map gate + e2e run, FLT-NAV-11) ---------------

interface ExecKeys {
  readonly tab: string;
  readonly step: string;
  readonly stepSort: string;
  readonly logExpanded: string;
  readonly logQuery: string;
}

function execKeys(prefix: string): ExecKeys {
  return {
    tab: `${prefix}tab`,
    step: `${prefix}step`,
    stepSort: `${prefix}ssort`,
    logExpanded: `${prefix}log`,
    logQuery: `${prefix}logq`,
  };
}

function parseExec(params: URLSearchParams, prefix: string): ExecState {
  const k = execKeys(prefix);
  const step = params.get(k.step);
  const logQuery = params.get(k.logQuery);
  return {
    tab: parseEnum(params.get(k.tab), EXEC_TAB_KEYS, "progress"),
    step: step !== null ? clampValue(step) : null,
    stepSort: parseSort(params.get(k.stepSort), STEP_SORT_COLUMNS),
    logExpanded: params.get(k.logExpanded) === "1",
    logQuery: logQuery !== null ? (clampValue(logQuery) ?? "") : "",
  };
}

function serializeExec(
  params: URLSearchParams,
  prefix: string,
  exec: ExecState,
): void {
  const k = execKeys(prefix);
  if (exec.tab !== "progress") {
    params.set(k.tab, exec.tab);
  }
  if (exec.step !== null) {
    params.set(k.step, exec.step);
  }
  const stepSort = serializeSort(exec.stepSort);
  if (stepSort !== null) {
    params.set(k.stepSort, stepSort);
  }
  if (exec.logExpanded) {
    params.set(k.logExpanded, "1");
  }
  if (exec.logQuery !== "") {
    params.set(k.logQuery, exec.logQuery);
  }
}

// --- Path codec ------------------------------------------------------------

function parseTab(path: string): TabKey {
  // Accept a base-path prefix: the last non-empty segment selects the tab, and the
  // empty root path is the default Fleet Map (FLT-NAV-19).
  const segments = path.split("/").filter((s) => s.length > 0);
  const last = segments[segments.length - 1];
  return parseEnum(last ?? null, TAB_KEYS, "map");
}

// --- Parse -----------------------------------------------------------------

/**
 * Parse a location into a normalized RouteState. Never throws: unknown or
 * malformed parameters degrade to safe defaults and are dropped on
 * re-serialization (FLT-NAV-14).
 */
export function parseRoute(location: RouteLocation): RouteState {
  const search = location.search.startsWith("?")
    ? location.search.slice(1)
    : location.search;
  const params = new URLSearchParams(search);

  const tab = parseTab(location.path);
  const bundleRaw = params.get("bundle");
  const bundle = bundleRaw !== null ? clampValue(bundleRaw) : null;

  // Map: node wins if both node and gate are present (FLT-NAV-14 canonicalizes
  // the redundant one away).
  const nodeRaw = params.get("node");
  const gateRaw = params.get("gate");
  const node = nodeRaw !== null ? clampValue(nodeRaw) : null;
  const gate = node === null && gateRaw !== null ? clampValue(gateRaw) : null;
  const attemptRaw = params.get("gattempt");

  const map: MapState = {
    node,
    gate,
    nodeTab: parseEnum(params.get("nodeTab"), NODE_TAB_KEYS, "details"),
    attempt: attemptRaw !== null ? clampValue(attemptRaw) : null,
    exec: parseExec(params, "g"),
  };

  // SDLC: scopes are repeated `scope=` params, deduplicated, canonically sorted,
  // and capped (FLT-NAV-02, FLT-NAV-13).
  const scopes = canonicalScopes(
    params
      .getAll("scope")
      .map(clampValue)
      .filter((v): v is string => v !== null),
  );
  const sdlcQuery = params.get("q");
  const shippedVerRaw = params.get("shippedVer");
  const shippedRowRaw = params.get("shiprow");
  const sdlc: SdlcState = {
    scopes,
    query: sdlcQuery !== null ? (clampValue(sdlcQuery) ?? "") : "",
    card: parsePositiveInt(params.get("card")),
    shipped: parsePositiveInt(params.get("shipped")),
    shippedVersion: shippedVerRaw !== null ? clampValue(shippedVerRaw) : null,
    shippedRow: shippedRowRaw !== null ? clampValue(shippedRowRaw) : null,
    escalations: params.get("escalations") === "1",
  };

  // E2E: the drawer defaults to open when a run is selected; `rund=0` is the
  // explicit selected-but-closed state (FLT-NAV-12).
  const runRaw = params.get("run");
  const run = runRaw !== null ? clampValue(runRaw) : null;
  const e2eQuery = params.get("runq");
  const e2e: E2eState = {
    query: e2eQuery !== null ? (clampValue(e2eQuery) ?? "") : "",
    runSort: parseSort(params.get("runsort"), RUN_SORT_COLUMNS),
    run,
    drawer: run !== null && params.get("rund") !== "0",
    exec: parseExec(params, "r"),
  };

  return normalizeRoute({ tab, bundle, map, sdlc, e2e });
}

/** Deduplicate and order scopes deterministically, applying the count cap. */
function canonicalScopes(values: readonly string[]): string[] {
  const unique = Array.from(new Set(values));
  unique.sort((a, b) => (a < b ? -1 : a > b ? 1 : 0));
  return unique.slice(0, ROUTE_LIMITS.scopes);
}

// --- Normalize -------------------------------------------------------------

/**
 * Normalize a RouteState so equivalent states are structurally identical: drop
 * sub-state that cannot apply (e.g. a shippedVersion with no shipped selection),
 * canonicalize scopes, and resolve the node/gate exclusion. Pure and idempotent.
 */
export function normalizeRoute(state: RouteState): RouteState {
  const node = state.map.node;
  const gate = node !== null ? null : state.map.gate;
  // The node sub-tab and gate execution detail only apply to their own selection.
  const map: MapState = {
    node,
    gate,
    nodeTab: node !== null ? state.map.nodeTab : "details",
    attempt: gate !== null ? state.map.attempt : null,
    exec: gate !== null ? state.map.exec : DEFAULT_EXEC,
  };

  // SDLC selections are mutually exclusive the way the reducers enforce them, so a
  // crafted or stale link that sets more than one must canonicalize to a single
  // winner rather than round-tripping an impossible combination (FLT-NAV-14). A
  // board card wins over a shipped-issue selection; a shipped-issue selection cannot
  // coexist with the escalations list.
  const card = state.sdlc.card;
  const shipped = card !== null ? null : state.sdlc.shipped;
  const escalations = shipped !== null ? false : state.sdlc.escalations;
  const sdlc: SdlcState = {
    ...state.sdlc,
    scopes: canonicalScopes(state.sdlc.scopes),
    card,
    shipped,
    // A version context only applies when a shipped issue is selected.
    shippedVersion: shipped !== null ? state.sdlc.shippedVersion : null,
    escalations,
  };

  // Execution detail only applies when a run is selected and the drawer is open.
  const runOpen = state.e2e.run !== null && state.e2e.drawer;
  const e2e: E2eState = {
    ...state.e2e,
    // An explicit runs sort equal to the table default collapses to null.
    runSort: sameSort(state.e2e.runSort, RUN_DEFAULT_SORT)
      ? null
      : state.e2e.runSort,
    drawer: state.e2e.run !== null && state.e2e.drawer,
    exec: runOpen ? state.e2e.exec : DEFAULT_EXEC,
  };

  return { tab: state.tab, bundle: state.bundle, map, sdlc, e2e };
}

// --- Serialize -------------------------------------------------------------

/**
 * Serialize a RouteState to a canonical location. Default-valued state is omitted,
 * query parameters are emitted in a fixed order, and scopes are sorted, so two
 * navigation sequences that reach the same view produce identical URLs
 * (FLT-NAV-02).
 */
export function serializeRoute(state: RouteState): RouteLocation {
  const norm = normalizeRoute(state);
  const params = new URLSearchParams();

  // Shared bundle first.
  if (norm.bundle !== null) {
    params.set("bundle", norm.bundle);
  }

  // Map.
  if (norm.map.node !== null) {
    params.set("node", norm.map.node);
    if (norm.map.nodeTab !== "details") {
      params.set("nodeTab", norm.map.nodeTab);
    }
  } else if (norm.map.gate !== null) {
    params.set("gate", norm.map.gate);
    if (norm.map.attempt !== null) {
      params.set("gattempt", norm.map.attempt);
    }
    serializeExec(params, "g", norm.map.exec);
  }

  // SDLC.
  for (const scope of norm.sdlc.scopes) {
    params.append("scope", scope);
  }
  if (norm.sdlc.query !== "") {
    params.set("q", norm.sdlc.query);
  }
  if (norm.sdlc.card !== null) {
    params.set("card", String(norm.sdlc.card));
  }
  if (norm.sdlc.shipped !== null) {
    params.set("shipped", String(norm.sdlc.shipped));
    if (norm.sdlc.shippedVersion !== null) {
      params.set("shippedVer", norm.sdlc.shippedVersion);
    }
  }
  if (norm.sdlc.shippedRow !== null) {
    params.set("shiprow", norm.sdlc.shippedRow);
  }
  if (norm.sdlc.escalations) {
    params.set("escalations", "1");
  }

  // E2E.
  if (norm.e2e.query !== "") {
    params.set("runq", norm.e2e.query);
  }
  const runSort = serializeSort(norm.e2e.runSort);
  if (runSort !== null) {
    params.set("runsort", runSort);
  }
  if (norm.e2e.run !== null) {
    params.set("run", norm.e2e.run);
    if (!norm.e2e.drawer) {
      params.set("rund", "0");
    } else {
      serializeExec(params, "r", norm.e2e.exec);
    }
  }

  const query = params.toString();
  const path = norm.tab === "map" ? "/" : `/${norm.tab}`;
  const search = query === "" ? "" : `?${query}`;
  const location = { path, search };

  // A pathological over-length URL degrades to the tab-only canonical view rather
  // than emitting something a browser or proxy may truncate (FLT-NAV-02).
  if (path.length + search.length > ROUTE_LIMITS.url) {
    return { path, search: "" };
  }
  return location;
}

/** Serialize to a single `path?search` string. */
export function routeToHref(state: RouteState): string {
  const loc = serializeRoute(state);
  return `${loc.path}${loc.search}`;
}

/** Parse a full `path?search` href into a RouteState (convenience for links). */
export function parseHref(href: string): RouteState {
  const qIndex = href.indexOf("?");
  const path = qIndex === -1 ? href : href.slice(0, qIndex);
  const search = qIndex === -1 ? "" : href.slice(qIndex);
  return parseRoute({ path, search });
}
