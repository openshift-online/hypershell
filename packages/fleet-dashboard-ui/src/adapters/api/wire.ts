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
  E2eEventName,
  E2eLogLine,
  E2eLogPage,
  E2eOutcome,
  E2ePlannedStep,
  E2eRunDetail,
  E2eRunEvent,
  E2eRunsData,
  E2eRunSummary,
  E2eStep,
  E2eStepState,
  E2eStreamState,
  E2eStreamStatus,
  E2eVerdict,
} from "../../domain/e2e-run";
import type {
  AgentSdlcBundleRow,
  AgentSdlcCard,
  AgentSdlcData,
  AgentSdlcHubCluster,
  AgentSdlcLabelEvent,
  AgentSdlcMatrix,
  AgentSdlcPhase,
  AgentSdlcPullRequest,
  AgentSdlcRework,
  AgentSdlcShippedIssue,
} from "../../domain/agent-sdlc";
import type {
  ClusterHistory,
  FleetData,
  GatewayClusterBreakdown,
  InstanceFleet,
  RateStats,
} from "../../domain/fleet";
import type {
  GitOpsRangeStatus,
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
import type {
  SdlcAgentsData,
  SdlcAgentMetrics,
  SdlcAgentSeries,
} from "../../domain/sdlc-agents";

/** A pull request within a release bundle, as the delivery GitHub App reports it. */
interface WirePR {
  readonly number?: number;
  readonly title?: string;
  readonly url?: string;
  readonly author?: string;
  readonly mergedAt?: string;
  // The owner/repo the PR belongs to (FLT-BPR-02); absent on older servers.
  readonly repository?: string;
  // Environments whose range includes a GitOps PR (FLT-BPR-06); absent otherwise.
  readonly environments?: readonly string[] | null;
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
  // The GitOps (deployment) pull-request group and its range state (FLT-BPR-04/10);
  // absent on older servers, which the UI renders as "no GitOps section".
  readonly gitopsPrs?: readonly WirePR[] | null;
  readonly gitopsStatus?: string;
}

/** A promotion gate on the wire: `{key, phase}` (the domain renames key -> name). */
interface WireGate {
  readonly key?: string;
  readonly phase?: string;
}

interface WirePromotionEnvironment {
  readonly verification?: {
    readonly revision: string;
    readonly cluster: string;
    readonly namespace: string;
  } | null;
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
  // Self-declared capability labels (`namespace/value`). The BFF prefers the
  // control plane's capability metric and falls back to the Argo delivery
  // annotation. Absent/null on servers that predate the field; treated as an
  // empty set.
  readonly capabilities?: readonly string[] | null;
  // Default gateway image the control plane provisions, from its info metric.
  // Absent when the control plane does not report it (FLT-CAP-07).
  readonly gatewayImage?: string;
  readonly argoHealth?: string;
  readonly argoSync?: string;
  // Argo CD sync-operation retry count. Omitted by the BFF when no operation
  // state exists (unknown), kept distinct from a genuine 0 (synced first try).
  readonly syncRetries?: number;
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
    verification: env.verification ?? null,
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
    // Carry capabilities verbatim, dropping any empty/non-string entry; absent ->
    // empty set. Each stays an opaque `namespace/value` string (FLT-CAP-01).
    capabilities: (env.capabilities ?? []).filter(
      (c): c is string => typeof c === "string" && c !== "",
    ),
    gatewayImage: nullableString(env.gatewayImage),
    argoHealth: nullableString(env.argoHealth),
    argoSync: nullableString(env.argoSync),
    // Preserve the 0-vs-unknown distinction: omitted -> null, a real 0 stays 0.
    syncRetries: env.syncRetries ?? null,
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
    // Repository identity for grouping/keys/links (FLT-BPR-02); "" on older servers.
    repository: raw.repository ?? "",
    // Environment attribution for GitOps PRs (FLT-BPR-06); drop any non-string entry.
    environments: (raw.environments ?? []).filter(
      (e): e is string => typeof e === "string" && e !== "",
    ),
  };
}

/** Narrow the wire GitOps status to the domain union; anything else hides the section. */
function mapGitOpsStatus(raw: string | undefined): GitOpsRangeStatus | null {
  return raw === "available" || raw === "unavailable" ? raw : null;
}

function mapRelease(raw: WireRelease): ReleaseBundle {
  return {
    version: raw.version ?? "",
    // Prefer the bundle digest (shared identity); fall back to the gitops SHA in
    // short-SHA mode so a bundle is still addressable.
    digest: nullableString(raw.digest) ?? nullableString(raw.sha),
    tag: nullableString(raw.tag),
    date: nullableString(raw.date),
    // Application PRs "since the previous build"; absent/null on older servers -> empty.
    prs: (raw.prs ?? []).map(mapPR),
    // GitOps (deployment) PR group and range state (FLT-BPR-04/10).
    gitopsPrs: (raw.gitopsPrs ?? []).map(mapPR),
    gitopsStatus: mapGitOpsStatus(raw.gitopsStatus),
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
  readonly historyByCluster?: readonly WireClusterHistory[] | null;
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

/** One managed cluster's gateway + sandbox history on the wire (/api/fleet). */
interface WireClusterHistory {
  readonly managedCluster?: string;
  readonly gatewayHistory?: readonly WireGatewayHistorySample[] | null;
  readonly sandboxHistory?: readonly number[] | null;
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
    // Per-spoke history: keep rows with a managed-cluster name; numify both aligned
    // series so each row is fully shaped (same keep-named-rows rule as the breakdowns).
    historyByCluster: (raw.historyByCluster ?? [])
      .map((c): ClusterHistory => ({
        managedCluster: c.managedCluster ?? "",
        gatewayHistory: (c.gatewayHistory ?? []).map((s) => ({
          running: num(s.running),
          provisioning: num(s.provisioning),
          failed: num(s.failed),
        })),
        sandboxHistory: (c.sandboxHistory ?? []).map((v) => num(v)),
      }))
      .filter((c) => c.managedCluster !== ""),
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

// --- Agent SDLC plane (/api/sdlc) ---------------------------------------------
// agent-sdlc-visibility-dashboard.spec. The BFF payload is already a small, domain
// -shaped projection; this mapper coerces each field defensively so a partial or
// malformed snapshot degrades to an empty board/lane/matrix rather than throwing.

interface WireSdlcPhase {
  readonly key?: string;
  readonly sleSeconds?: number;
}

interface WireSdlcRework {
  readonly count?: number;
  readonly threshold?: number;
}

interface WireSdlcLabelEvent {
  readonly label?: string;
  readonly action?: string;
  readonly at?: string;
}

interface WireSdlcCard {
  readonly number?: number;
  readonly title?: string;
  readonly url?: string;
  readonly phase?: string;
  readonly timeInPhaseSeconds?: number;
  readonly scope?: string;
  readonly escalation?: string;
  readonly rework?: WireSdlcRework | null;
  readonly history?: readonly WireSdlcLabelEvent[] | null;
}

interface WireSdlcHubCluster {
  readonly key?: string;
  readonly cluster?: string;
  readonly env?: string;
  readonly currentVersion?: string;
  readonly currentDigest?: string;
}

interface WireSdlcPullRequest {
  readonly number?: number;
  readonly title?: string;
  readonly url?: string;
  readonly author?: string;
  readonly mergedAt?: string;
}

interface WireSdlcShippedIssue {
  readonly number?: number;
  readonly title?: string;
  readonly url?: string;
  readonly mergedAt?: string;
  readonly mergedBy?: string;
  readonly prs?: readonly WireSdlcPullRequest[] | null;
}

interface WireSdlcBundleRow {
  readonly version?: string;
  readonly digest?: string;
  readonly date?: string;
  readonly unreleased?: boolean;
  readonly hubClusters?: readonly string[] | null;
  readonly issues?: readonly WireSdlcShippedIssue[] | null;
  readonly otherPRs?: readonly WireSdlcPullRequest[] | null;
}

interface WireSdlcMatrix {
  readonly hubClusters?: readonly WireSdlcHubCluster[] | null;
  readonly rows?: readonly WireSdlcBundleRow[] | null;
}

interface WireSdlc {
  readonly phases?: readonly WireSdlcPhase[] | null;
  readonly cards?: readonly WireSdlcCard[] | null;
  readonly lane?: readonly WireSdlcCard[] | null;
  readonly matrix?: WireSdlcMatrix | null;
}

function mapSdlcLabelEvent(e: WireSdlcLabelEvent): AgentSdlcLabelEvent {
  return { label: e.label ?? "", action: e.action ?? "", at: e.at ?? "" };
}

function mapSdlcRework(
  r: WireSdlcRework | null | undefined,
): AgentSdlcRework | null {
  if (r === null || r === undefined) {
    return null;
  }
  return { count: num(r.count), threshold: num(r.threshold) };
}

function mapSdlcCard(c: WireSdlcCard): AgentSdlcCard {
  return {
    number: num(c.number),
    title: c.title ?? "",
    url: c.url ?? "",
    phase: c.phase ?? "",
    timeInPhaseSeconds: num(c.timeInPhaseSeconds),
    scope: nullableString(c.scope),
    escalation: nullableString(c.escalation),
    rework: mapSdlcRework(c.rework),
    history: (c.history ?? []).map(mapSdlcLabelEvent),
  };
}

function mapSdlcHubCluster(h: WireSdlcHubCluster): AgentSdlcHubCluster {
  return {
    key: h.key ?? "",
    cluster: nullableString(h.cluster),
    env: nullableString(h.env),
    currentVersion: nullableString(h.currentVersion),
    currentDigest: nullableString(h.currentDigest),
  };
}

function mapSdlcPullRequest(p: WireSdlcPullRequest): AgentSdlcPullRequest {
  return {
    number: num(p.number),
    title: p.title ?? "",
    url: p.url ?? "",
    author: nullableString(p.author),
    mergedAt: nullableString(p.mergedAt),
  };
}

function mapSdlcShippedIssue(i: WireSdlcShippedIssue): AgentSdlcShippedIssue {
  return {
    number: num(i.number),
    title: i.title ?? "",
    url: i.url ?? "",
    mergedAt: nullableString(i.mergedAt),
    mergedBy: nullableString(i.mergedBy),
    prs: (i.prs ?? []).map(mapSdlcPullRequest),
  };
}

function mapSdlcBundleRow(r: WireSdlcBundleRow): AgentSdlcBundleRow {
  return {
    version: nullableString(r.version),
    digest: nullableString(r.digest),
    date: nullableString(r.date),
    unreleased: r.unreleased === true,
    hubClusters: (r.hubClusters ?? []).slice(),
    issues: (r.issues ?? []).map(mapSdlcShippedIssue),
    otherPRs: (r.otherPRs ?? []).map(mapSdlcPullRequest),
  };
}

function mapSdlcMatrix(m: WireSdlcMatrix | null | undefined): AgentSdlcMatrix {
  return {
    hubClusters: (m?.hubClusters ?? []).map(mapSdlcHubCluster),
    rows: (m?.rows ?? []).map(mapSdlcBundleRow),
  };
}

export function mapSdlc(raw: unknown): AgentSdlcData {
  const wire = (raw ?? {}) as WireSdlc;
  const phases: AgentSdlcPhase[] = (wire.phases ?? []).map((p) => ({
    key: p.key ?? "",
    sleSeconds: num(p.sleSeconds),
  }));
  return {
    phases,
    cards: (wire.cards ?? []).map(mapSdlcCard),
    lane: (wire.lane ?? []).map(mapSdlcCard),
    matrix: mapSdlcMatrix(wire.matrix),
  };
}

// --- E2E run plane (/api/e2e/*) -----------------------------------------------
// e2e-run-log-visibility.spec. The BFF serialises each plane's `data` in the shapes
// defined by components/fleet-dashboard/pkg/sources/e2e.go (+ e2e_stream.go). This
// mapper is the ONE place those wire shapes are known; it coerces each field
// defensively so a partial/malformed payload degrades to an empty run list or detail
// rather than throwing, and defaults missing arrays to [].

interface WireE2ePlanned {
  readonly stepId?: string;
  readonly name?: string;
}

interface WireE2eSummary {
  readonly bundle?: WireRelease | null;
  readonly id?: string;
  readonly runId?: string;
  readonly tenant?: string;
  readonly cluster?: string;
  readonly bodyCluster?: string;
  readonly namespace?: string;
  readonly revision?: string;
  readonly sourceRevision?: string;
  readonly startedAt?: string;
  readonly endedAt?: string;
  readonly durationMs?: number;
  readonly verdict?: string;
  readonly reportingComplete?: boolean;
  readonly hasStructuredSteps?: boolean;
  readonly stepCount?: number;
}

interface WireE2eStep {
  readonly stepId?: string;
  readonly name?: string;
  readonly state?: string;
  readonly startedAt?: string;
  readonly endedAt?: string;
  readonly durationMs?: number;
}

interface WireE2eLogLine {
  readonly sequence?: number;
  readonly timestamp?: string;
  readonly stepId?: string;
  readonly message?: string;
  readonly truncated?: boolean;
}

interface WireE2eDetail extends WireE2eSummary {
  readonly plannedSteps?: readonly WireE2ePlanned[] | null;
  readonly steps?: readonly WireE2eStep[] | null;
  readonly logs?: readonly WireE2eLogLine[] | null;
  readonly logsTruncated?: boolean;
}

interface WireE2eRuns {
  readonly enabled?: boolean;
  readonly runs?: readonly WireE2eSummary[] | null;
  readonly truncated?: boolean;
  readonly windowSeconds?: number;
  readonly limit?: number;
}

interface WireE2eStreamEvent {
  readonly event?: string;
  readonly runId?: string;
  readonly sequence?: number;
  readonly timestamp?: string;
  readonly stepId?: string;
  readonly name?: string;
  readonly outcome?: string;
  readonly durationMs?: number;
  readonly message?: string;
  readonly truncated?: boolean;
  readonly plannedSteps?: readonly WireE2ePlanned[] | null;
}

interface WireE2eStreamStatus {
  readonly state?: string;
  readonly droppedEntries?: number;
}

interface WireE2eLogPage {
  readonly entries?: readonly WireE2eLogLine[] | null;
  readonly nextCursor?: string;
  readonly hasMore?: boolean;
  readonly boundaryTruncated?: boolean;
}

/** Coerce an optional number to a nullable number, keeping a genuine 0. */
function nullableNum(v: number | undefined): number | null {
  return typeof v === "number" && Number.isFinite(v) ? v : null;
}

function mapVerdict(v: string | undefined): E2eVerdict {
  return v === "passed" || v === "failed" ? v : "unknown";
}

function mapStepState(v: string | undefined): E2eStepState {
  switch (v) {
    case "passed":
    case "failed":
    case "skipped":
    case "running":
    case "not-run":
      return v;
    default:
      return "not-run";
  }
}

function mapStreamState(v: string | undefined): E2eStreamState {
  return v === "live" || v === "gap" ? v : "disconnected";
}

function mapEventName(v: string | undefined): E2eEventName | null {
  switch (v) {
    case "run_started":
    case "step_started":
    case "step_finished":
    case "log":
    case "run_finished":
      return v;
    default:
      return null;
  }
}

function mapOutcome(v: string | undefined): E2eOutcome | null {
  return v === "passed" || v === "failed" || v === "skipped" ? v : null;
}

function mapPlanned(p: WireE2ePlanned): E2ePlannedStep {
  return { stepId: p.stepId ?? "", name: p.name ?? "" };
}

function mapE2eSummary(raw: WireE2eSummary): E2eRunSummary {
  return {
    bundle: raw.bundle ? mapRelease(raw.bundle) : null,
    id: raw.id ?? "",
    runId: raw.runId ?? "",
    tenant: raw.tenant ?? "",
    cluster: raw.cluster ?? "",
    bodyCluster: nullableString(raw.bodyCluster),
    namespace: raw.namespace ?? "",
    revision: raw.revision ?? "",
    sourceRevision: nullableString(raw.sourceRevision),
    startedAt: raw.startedAt ?? "",
    endedAt: nullableString(raw.endedAt),
    durationMs: nullableNum(raw.durationMs),
    verdict: mapVerdict(raw.verdict),
    reportingComplete: raw.reportingComplete === true,
    hasStructuredSteps: raw.hasStructuredSteps === true,
    stepCount: num(raw.stepCount),
  };
}

function mapE2eStep(raw: WireE2eStep): E2eStep {
  return {
    stepId: raw.stepId ?? "",
    name: raw.name ?? "",
    state: mapStepState(raw.state),
    startedAt: nullableString(raw.startedAt),
    endedAt: nullableString(raw.endedAt),
    durationMs: nullableNum(raw.durationMs),
  };
}

function mapE2eLogLine(raw: WireE2eLogLine): E2eLogLine {
  return {
    sequence: num(raw.sequence),
    timestamp: raw.timestamp ?? "",
    stepId: nullableString(raw.stepId),
    message: raw.message ?? "",
    truncated: raw.truncated === true,
  };
}

/** Map the /api/e2e/runs plane data. A disabled feature arrives as `{enabled:false}`. */
export function mapE2eRuns(raw: unknown): E2eRunsData {
  const wire = (raw ?? {}) as WireE2eRuns;
  return {
    enabled: wire.enabled === true,
    runs: (wire.runs ?? []).map(mapE2eSummary),
    truncated: wire.truncated === true,
    windowSeconds: num(wire.windowSeconds),
    limit: num(wire.limit),
  };
}

/** Map the /api/e2e/runs/{id} plane data: a summary plus steps and bounded logs. */
export function mapE2eRunDetail(raw: unknown): E2eRunDetail {
  const wire = (raw ?? {}) as WireE2eDetail;
  return {
    ...mapE2eSummary(wire),
    plannedSteps: (wire.plannedSteps ?? []).map(mapPlanned),
    steps: (wire.steps ?? []).map(mapE2eStep),
    logs: (wire.logs ?? []).map(mapE2eLogLine),
    logsTruncated: wire.logsTruncated === true,
  };
}

/**
 * Map one forwarded stream event. Returns null for an unknown event name so the
 * adapter skips a malformed frame without dropping the rest of the stream
 * (E2E-LOG-07).
 */
export function mapE2eStreamEvent(raw: unknown): E2eRunEvent | null {
  const wire = (raw ?? {}) as WireE2eStreamEvent;
  const event = mapEventName(wire.event);
  if (event === null) {
    return null;
  }
  return {
    event,
    runId: wire.runId ?? "",
    sequence: num(wire.sequence),
    timestamp: wire.timestamp ?? "",
    stepId: nullableString(wire.stepId),
    name: nullableString(wire.name),
    outcome: mapOutcome(wire.outcome),
    durationMs: nullableNum(wire.durationMs),
    message: nullableString(wire.message),
    truncated: wire.truncated === true,
    plannedSteps: (wire.plannedSteps ?? []).map(mapPlanned),
  };
}

/** Map one stream status frame. */
export function mapE2eStreamStatus(raw: unknown): E2eStreamStatus {
  const wire = (raw ?? {}) as WireE2eStreamStatus;
  return {
    state: mapStreamState(wire.state),
    droppedEntries: num(wire.droppedEntries),
  };
}

/** Map one paginated raw-log page (E2E-LOG-19). Entries are already chronological-
 *  ascending from the BFF; an absent cursor is normalized to null. */
export function mapE2eRunLogPage(raw: unknown): E2eLogPage {
  const wire = (raw ?? {}) as WireE2eLogPage;
  return {
    entries: (wire.entries ?? []).map(mapE2eLogLine),
    nextCursor: nullableString(wire.nextCursor),
    hasMore: wire.hasMore === true,
    boundaryTruncated: wire.boundaryTruncated === true,
  };
}

/** Map one live `log` frame into a reduced log line (E2E-LOG-21). Returns null when
 *  the frame carries no usable line, so the adapter skips it without dropping the
 *  rest of the stream (E2E-LOG-07). */
export function mapE2eStreamLog(raw: unknown): E2eLogLine | null {
  if (raw === null || typeof raw !== "object") {
    return null;
  }
  return mapE2eLogLine(raw);
}

// --- SDLC Agents (runtime telemetry) ----------------------------------------

interface WireSdlcAgentSeries {
  times?: readonly number[];
  queueDepth?: readonly number[];
  itemsProcessed?: readonly number[];
  itemsFailed?: readonly number[];
  provisionSeconds?: readonly number[];
}

interface WireSdlcAgentMetrics {
  name?: string;
  env?: string;
  replicasOn?: number;
  replicasTotal?: number;
  queueDepth?: number;
  itemsProcessed?: number;
  itemsFailed?: number;
  provisionSeconds?: number;
  lastReportedAt?: number;
  series?: WireSdlcAgentSeries;
}

interface WireSdlcAgents {
  envs?: readonly string[];
  agents?: readonly WireSdlcAgentMetrics[];
}

function mapSdlcAgentSeries(w?: WireSdlcAgentSeries): SdlcAgentSeries {
  return {
    times: w?.times ?? [],
    queueDepth: w?.queueDepth ?? [],
    itemsProcessed: w?.itemsProcessed ?? [],
    itemsFailed: w?.itemsFailed ?? [],
    provisionSeconds: w?.provisionSeconds ?? [],
  };
}

function mapSdlcAgentRow(w?: WireSdlcAgentMetrics): SdlcAgentMetrics {
  return {
    name: w?.name ?? "",
    env: w?.env ?? "",
    replicasOn: w?.replicasOn ?? 0,
    replicasTotal: w?.replicasTotal ?? 0,
    queueDepth: w?.queueDepth ?? 0,
    itemsProcessed: w?.itemsProcessed ?? 0,
    itemsFailed: w?.itemsFailed ?? 0,
    provisionSeconds: w?.provisionSeconds ?? 0,
    // Freshness fact (SDLC-AGT-15): defensively default to 0 when absent, matching
    // the BFF's own 0-when-absent contract.
    lastReportedAt: w?.lastReportedAt ?? 0,
    series: mapSdlcAgentSeries(w?.series),
  };
}

export function mapSdlcAgents(raw: unknown): SdlcAgentsData {
  const w = (raw ?? {}) as WireSdlcAgents;
  return {
    envs: Array.isArray(w.envs) ? w.envs.map(String) : [],
    agents: Array.isArray(w.agents) ? w.agents.map(mapSdlcAgentRow) : [],
  };
}
