// Anti-corruption layer: the Go BFF serialises each plane in the shape defined by
// data-architecture.spec §5 ("/api/promotion ... preserves the promotion.json
// shape"). That wire contract is authoritative; the UI's domain models are a
// deliberately smaller, firewall-clean projection of it (see domain/promotion.ts).
// These mappers are the ONE place the wire shape is known - everything downstream
// consumes the domain types. Without this mapping the adapter was handing raw wire
// objects to components that expect the domain shape (e.g. `env.gates`, which the
// wire spells `activeGates`), throwing at render time.

import type { InstancesData } from "../../application/ports";
import type {
  FleetData,
  GatewayClusterBreakdown,
  InstanceFleet,
  RateStats,
} from "../../domain/fleet";
import type {
  PromotionData,
  PromotionEnvironment,
  PromotionGate,
  PullRequest,
  ReleaseBundle,
} from "../../domain/promotion";
import type {
  InstanceTopology,
  TopologyData,
  TopologyHub,
  TopologySpoke,
} from "../../domain/topology";

/** A pull request within a release bundle, as the delivery GitHub App reports it. */
interface WirePR {
  readonly number?: number;
  readonly title?: string;
  readonly url?: string;
  readonly author?: string;
  readonly mergedAt?: string;
}

/** A release as the BFF reports it (opaque to the UI beyond its version string). */
interface WireRelease {
  readonly version?: string;
  readonly tag?: string;
  readonly date?: string;
  readonly sha?: string;
  /**
   * Bundle identity: the versions-lock digest shared by every env rendering the
   * same release. Absent in short-SHA fallback mode, where `sha` is the only
   * identity. The server also keys the `releases` map by this value.
   */
  readonly digest?: string;
  readonly prs?: readonly WirePR[] | null;
}

/** A promotion gate on the wire: `{key, phase}` (the domain renames key -> name). */
interface WireGate {
  readonly key?: string;
  readonly phase?: string;
}

interface WirePromotionEnvironment {
  readonly active?: WireRelease | null;
  readonly proposed?: WireRelease | null;
  readonly activeGates?: readonly WireGate[] | null;
  readonly proposedGates?: readonly WireGate[] | null;
  // Delivery labels + Argo status the BFF attaches per instance (all optional;
  // every one is runtime data, never inferred from the key).
  readonly role?: string;
  readonly provider?: string;
  readonly env?: string;
  readonly cluster?: string;
  readonly argoHealth?: string;
  readonly argoSync?: string;
  readonly consoleUrl?: string;
  readonly grafanaUrl?: string;
  readonly argoUrl?: string;
  readonly prState?: string;
  readonly prUrl?: string;
  readonly analysisUrl?: string;
  readonly upToDate?: boolean;
}

interface WirePromotion {
  readonly order?: readonly string[] | null;
  readonly environments?: Readonly<
    Record<string, WirePromotionEnvironment>
  > | null;
  readonly releases?: Readonly<Record<string, WireRelease>> | null;
  readonly frontier?: WireRelease | null;
}

/** `/api/instances` returns a bare list of instance names. */
interface WireInstances {
  readonly instances?: readonly string[] | null;
}

function mapGate(gate: WireGate): PromotionGate {
  return {
    name: gate.key ?? "",
    phase: gate.phase ?? null,
    // The wire does not attribute a gate to a governing instance; the domain
    // keeps the field so the server can supply it later without a shape change.
    governingInstance: null,
  };
}

function nullableString(v: string | undefined): string | null {
  return v !== undefined && v !== "" ? v : null;
}

function mapEnvironment(
  name: string,
  env: WirePromotionEnvironment,
): PromotionEnvironment {
  return {
    name,
    activeRelease: env.active?.version ?? null,
    proposedRelease: env.proposed?.version ?? null,
    // The "Gates" column reflects the gates governing what is currently active.
    gates: (env.activeGates ?? []).map(mapGate),
    activeDigest:
      nullableString(env.active?.digest) ?? nullableString(env.active?.sha),
    proposedDigest:
      nullableString(env.proposed?.digest) ?? nullableString(env.proposed?.sha),
    role: nullableString(env.role),
    provider: nullableString(env.provider),
    envLabel: nullableString(env.env),
    cluster: nullableString(env.cluster),
    argoHealth: nullableString(env.argoHealth),
    argoSync: nullableString(env.argoSync),
    consoleUrl: nullableString(env.consoleUrl),
    grafanaUrl: nullableString(env.grafanaUrl),
    argoUrl: nullableString(env.argoUrl),
    prState: nullableString(env.prState),
    prUrl: nullableString(env.prUrl),
    analysisUrl: nullableString(env.analysisUrl),
    upToDate: env.upToDate ?? false,
  };
}

function mapPR(raw: WirePR): PullRequest {
  return {
    number: typeof raw.number === "number" ? raw.number : 0,
    title: raw.title ?? "",
    url: raw.url ?? "",
    author: raw.author ?? "",
    mergedAt: nullableString(raw.mergedAt),
  };
}

function mapRelease(raw: WireRelease): ReleaseBundle {
  return {
    version: raw.version ?? "",
    // Prefer the bundle digest (shared identity); fall back to the gitops SHA in
    // short-SHA mode so a bundle is still addressable.
    digest: nullableString(raw.digest) ?? nullableString(raw.sha),
    tag: nullableString(raw.tag),
    date: nullableString(raw.date),
    // PRs "since the previous build"; absent/null on older servers -> empty.
    prs: (raw.prs ?? []).map(mapPR),
  };
}

export function mapPromotion(raw: unknown): PromotionData {
  const wire = (raw ?? {}) as WirePromotion;
  const environments: Record<string, PromotionEnvironment> = {};
  for (const [key, env] of Object.entries(wire.environments ?? {})) {
    environments[key] = mapEnvironment(key, env);
  }
  // The wire keys `releases` by digest; keep both the version list (existing
  // callers) and the full bundle map (the freight-bar cards) keyed by digest.
  const releaseByDigest: Record<string, ReleaseBundle> = {};
  const versions: string[] = [];
  for (const [digest, rel] of Object.entries(wire.releases ?? {})) {
    const bundle = mapRelease(rel);
    releaseByDigest[digest] = bundle;
    if (bundle.version) {
      versions.push(bundle.version);
    }
  }
  return {
    order: wire.order ?? [],
    environments,
    releases: versions,
    releaseByDigest,
    frontier: wire.frontier ? mapRelease(wire.frontier) : null,
  };
}

/** A rate/error/p95 triple on the wire (all optional; default to zero). */
interface WireRateStats {
  readonly rate?: number;
  readonly errorPct?: number;
  readonly p95Ms?: number;
}

/** One instance's fleet record on the wire (data-architecture.spec §5, /api/fleet). */
interface WireInstanceFleet {
  readonly instance?: string;
  readonly gateways?: Readonly<Record<string, number>> | null;
  readonly gatewaysTotal?: number;
  readonly gatewaysByCluster?: readonly WireGatewayClusterBreakdown[] | null;
  readonly managedClusters?: number;
  readonly users?: number;
  readonly rpc?: WireRateStats | null;
  readonly reconcile?: WireRateStats | null;
  readonly bff?: WireRateStats | null;
  readonly provisionP95Ms?: number;
  readonly gatewayHistory?: readonly WireGatewayHistorySample[] | null;
  readonly sandboxes?: number;
  readonly sandboxesByCluster?: readonly WireSandboxClusterCount[] | null;
  readonly sandboxHistory?: readonly number[] | null;
  readonly logins?: number;
  readonly userHistory?: readonly number[] | null;
  readonly loginsHistory?: readonly number[] | null;
  readonly historyTimes?: readonly number[] | null;
}

interface WireGatewayHistorySample {
  readonly running?: number;
  readonly provisioning?: number;
  readonly failed?: number;
}

interface WireSandboxClusterCount {
  // `managedCluster` is the spoke name (the correct key); `cluster` is the legacy
  // scrape-injected HUB label, ignored for attribution (see mapInstanceFleet).
  readonly managedCluster?: string;
  readonly cluster?: string;
  readonly count?: number;
}

/** One managed cluster's gateway phase breakdown on the wire (/api/fleet). */
interface WireGatewayClusterBreakdown {
  readonly managedCluster?: string;
  readonly gateways?: Readonly<Record<string, number>> | null;
  readonly total?: number;
}

/** `/api/fleet` is a map keyed by instance name; the domain uses a flat list. */
type WireFleet = Readonly<Record<string, WireInstanceFleet>>;

function num(v: number | undefined): number {
  return typeof v === "number" && Number.isFinite(v) ? v : 0;
}

/** Project a wire phase-count record into a numified phase map (absent -> {}). */
function mapPhaseCounts(
  raw: Readonly<Record<string, number>> | null | undefined,
): Record<string, number> {
  const out: Record<string, number> = {};
  for (const [phase, count] of Object.entries(raw ?? {})) {
    out[phase] = num(count);
  }
  return out;
}

function mapRate(raw: WireRateStats | null | undefined): RateStats {
  return {
    rate: num(raw?.rate),
    errorPct: num(raw?.errorPct),
    p95Ms: num(raw?.p95Ms),
  };
}

function mapInstanceFleet(key: string, raw: WireInstanceFleet): InstanceFleet {
  const gateways = raw.gateways ?? {};
  return {
    // The wire keys the record by instance; trust the key over an absent field.
    instance: raw.instance && raw.instance !== "" ? raw.instance : key,
    // Role/provider are promotion-plane labels; the fleet metrics source omits
    // them. The map view merges them in from /api/promotion.
    role: null,
    provider: null,
    gateways,
    gatewaysTotal: num(raw.gatewaysTotal),
    // Per-spoke gateway breakdown: keep rows with a managed-cluster (spoke) name;
    // numify every phase count so each row is fully shaped.
    gatewaysByCluster: (raw.gatewaysByCluster ?? [])
      .map((g): GatewayClusterBreakdown => ({
        managedCluster: g.managedCluster ?? "",
        gateways: mapPhaseCounts(g.gateways),
        total: num(g.total),
      }))
      .filter((g) => g.managedCluster !== ""),
    managedClusters:
      typeof raw.managedClusters === "number" ? raw.managedClusters : null,
    users: typeof raw.users === "number" ? raw.users : null,
    rpc: mapRate(raw.rpc),
    reconcile: mapRate(raw.reconcile),
    bff: mapRate(raw.bff),
    provisionP95Ms:
      typeof raw.provisionP95Ms === "number" ? raw.provisionP95Ms : null,
    gatewayHistory: (raw.gatewayHistory ?? []).map((s) => ({
      running: num(s.running),
      provisioning: num(s.provisioning),
      failed: num(s.failed),
    })),
    sandboxes: num(raw.sandboxes),
    // Attribute per spoke by the application-emitted `managedCluster` name, NOT the
    // legacy scrape-injected `cluster` (hub) label; drop rows with no spoke name.
    sandboxesByCluster: (raw.sandboxesByCluster ?? [])
      .map((s) => ({
        managedCluster: s.managedCluster ?? "",
        count: num(s.count),
      }))
      .filter((s) => s.managedCluster !== ""),
    sandboxHistory: (raw.sandboxHistory ?? []).map((v) => num(v)),
    logins: typeof raw.logins === "number" ? raw.logins : null,
    userHistory: (raw.userHistory ?? []).map((v) => num(v)),
    loginsHistory: (raw.loginsHistory ?? []).map((v) => num(v)),
    historyTimes: (raw.historyTimes ?? []).map((v) => num(v)),
  };
}

export function mapFleet(raw: unknown): FleetData {
  const wire = (raw ?? {}) as WireFleet;
  const instances = Object.entries(wire)
    .map(([key, rec]) => mapInstanceFleet(key, rec))
    .sort((a, b) => a.instance.localeCompare(b.instance));
  return { instances };
}

/** The hub block inside a topology.json document (snake_case, as authored). */
interface WireTopologyHub {
  readonly instance?: string;
  readonly dns_label?: string;
  readonly remote_spokes?: readonly string[] | null;
}

/** A co-located spoke inside a topology.json document. */
interface WireTopologySpoke {
  readonly name?: string;
}

/** A decoded topology.json document (the `topology` field of an /api/topology entry). */
interface WireTopologyDoc {
  readonly hub?: WireTopologyHub | null;
  readonly spokes?: readonly WireTopologySpoke[] | null;
}

/** One /api/topology entry: provenance plus the raw topology/clone documents. */
interface WireTopologyEntry {
  readonly instance?: string;
  // The BFF serves topology.json verbatim as embedded JSON; tolerate a string too
  // (a server that passed the raw text through) by parsing it defensively.
  readonly topology?: WireTopologyDoc | string | null;
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null;
}

/** Parse a topology document that may arrive as an object or a JSON string. */
function parseTopologyDoc(
  raw: WireTopologyDoc | string | null | undefined,
): WireTopologyDoc | null {
  if (raw === null || raw === undefined) {
    return null;
  }
  if (typeof raw === "string") {
    try {
      const parsed: unknown = JSON.parse(raw);
      return isRecord(parsed) ? parsed : null;
    } catch {
      return null;
    }
  }
  return raw;
}

function mapTopologyHub(
  raw: WireTopologyHub | null | undefined,
): TopologyHub | null {
  if (!raw || typeof raw.instance !== "string" || raw.instance === "") {
    return null;
  }
  return {
    instance: raw.instance,
    dnsLabel: nullableString(raw.dns_label),
    remoteSpokes: (raw.remote_spokes ?? []).filter(
      (s): s is string => typeof s === "string" && s !== "",
    ),
  };
}

function mapTopologyEntry(
  key: string,
  entry: WireTopologyEntry,
): InstanceTopology {
  const doc = parseTopologyDoc(entry.topology);
  const spokes: TopologySpoke[] = (doc?.spokes ?? [])
    .map((s): TopologySpoke => ({
      name: typeof s.name === "string" ? s.name : "",
    }))
    .filter((s) => s.name !== "");
  return {
    instance: entry.instance && entry.instance !== "" ? entry.instance : key,
    hub: mapTopologyHub(doc?.hub),
    spokes,
  };
}

/**
 * Project the /api/topology record (instance -> entry) into the domain's topology
 * map. Every entry is parsed defensively: a malformed document degrades to an empty
 * topology for that instance rather than throwing, so one bad entry never blanks the
 * plane (data-architecture.spec §3.2 treats topology as data that may be stale/bad).
 */
export function mapTopology(raw: unknown): TopologyData {
  const out: Record<string, InstanceTopology> = {};
  if (!isRecord(raw)) {
    return out;
  }
  for (const [key, entry] of Object.entries(raw)) {
    out[key] = mapTopologyEntry(key, isRecord(entry) ? entry : {});
  }
  return out;
}

export function mapInstances(raw: unknown): InstancesData {
  const wire = (raw ?? {}) as WireInstances;
  return {
    instances: (wire.instances ?? []).map((name) => ({
      name,
      // Role/provider/region/health are not carried on /api/instances today;
      // they render as "none" until the server includes them.
      role: null,
      provider: null,
      region: null,
      health: null,
    })),
  };
}
