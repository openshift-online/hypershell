// Anti-corruption mapping for /api/delivery (fleet-dashboard-delivery-pipeline
// .spec FLT-DLV-12). The wire shape is known only here; everything downstream
// consumes the domain types in domain/delivery.ts.

import {
  DELIVERY_SOURCE_KEYS,
  type DeliveryBundle,
  type DeliveryChange,
  type DeliveryCommitLink,
  type DeliveryCheck,
  type DeliveryData,
  type DeliveryEnvState,
  type DeliveryImageCheck,
  type DeliveryPromotion,
  type DeliverySourceKey,
  type DeliverySourceStatus,
  type DeliveryStage,
  type DeliveryStageDef,
  type DeliveryStageState,
  type PromotionOutcome,
  type ReleaseDiagnostics,
} from "../../domain/delivery";

interface WireStageDef {
  readonly key?: string;
  readonly kind?: string;
  readonly group?: string;
}

interface WireStage {
  readonly key?: string;
  readonly state?: string;
  readonly startedAt?: string;
  readonly finishedAt?: string;
  readonly reason?: string;
  readonly components?: readonly string[] | null;
  readonly groups?: readonly string[] | null;
  readonly gate?: string;
  readonly done?: number;
  readonly total?: number;
  readonly pr?: number;
  readonly checks?: string;
  readonly failedAttempts?: number;
  readonly estimateSeconds?: number;
  readonly slowestSeconds?: number;
}

interface WireCheck {
  readonly component?: string;
  readonly state?: string;
  readonly startedAt?: string;
  readonly completedAt?: string;
  readonly url?: string;
  readonly outsideBundle?: boolean;
}

interface WireImageCheck {
  readonly state?: string;
  readonly url?: string;
  readonly failures?: readonly string[] | null;
}

interface WireBundle {
  readonly tag?: string;
  readonly version?: string;
  readonly digest?: string;
  readonly publishedAt?: string;
  readonly locked?: boolean;
  readonly releaseUrl?: string;
}

interface WirePromotion {
  readonly version?: string;
  readonly digest?: string;
  readonly gitopsPr?: number;
  readonly promotedAt?: string;
  readonly url?: string;
  readonly outcome?: string;
  readonly gate?: string;
  readonly gatesPassedAt?: string;
}

interface WireEnv {
  readonly env?: string;
  readonly group?: string;
  readonly state?: string;
  readonly contains?: boolean;
  readonly history?: readonly WirePromotion[] | null;
  readonly analysisUrl?: string;
  readonly argoUrl?: string;
}

interface WireChange {
  readonly sha?: string;
  readonly shortSha?: string;
  readonly mergedAt?: string;
  readonly title?: string;
  readonly author?: string;
  readonly pr?: number;
  readonly prUrl?: string;
  readonly commitUrl?: string;
  readonly delivered?: boolean;
  readonly deliveredAt?: string;
  readonly stages?: readonly WireStage[] | null;
  readonly builds?: readonly WireCheck[] | null;
  readonly policies?: readonly WireCheck[] | null;
  readonly bundle?: WireBundle | null;
  readonly gitopsPr?: {
    readonly number?: number;
    readonly url?: string;
  } | null;
  readonly environments?: readonly WireEnv[] | null;
  readonly imageCheck?: WireImageCheck | null;
}

interface WireSource {
  readonly stale?: boolean;
  readonly never?: boolean;
  readonly error?: string;
  readonly lastSuccess?: string;
}

interface WireCommitLink {
  readonly sha?: string;
  readonly shortSha?: string;
  readonly title?: string;
  readonly pr?: number;
  readonly url?: string;
  readonly mergedAt?: string;
}

interface WireReleaseDiagnostics {
  readonly lastGood?: WireBundle | null;
  readonly logUrl?: string;
  readonly appUrl?: string;
  readonly commits?: readonly WireCommitLink[] | null;
  readonly commitsError?: string;
  readonly registryTag?: string;
  readonly githubReleaseTag?: string;
  readonly releaseError?: string;
}

interface WireDelivery {
  readonly enabled?: boolean;
  readonly stages?: readonly WireStageDef[] | null;
  readonly changes?: readonly WireChange[] | null;
  readonly releaseDiagnostics?: WireReleaseDiagnostics | null;
  readonly omitted?: number;
  readonly sources?: Readonly<Record<string, WireSource>> | null;
}

const STATES: readonly DeliveryStageState[] = [
  "not-started",
  "waiting",
  "running",
  "passed",
  "failed",
  "not-needed",
  "unknown",
];

/** Unknown or missing states read as Unknown, never as Passed (FLT-DLV-04). */
function mapState(v: string | undefined): DeliveryStageState {
  return (STATES as readonly string[]).includes(v ?? "")
    ? (v as DeliveryStageState)
    : "unknown";
}

function str(v: string | undefined): string | null {
  return v === undefined || v === "" ? null : v;
}

function int(v: number | undefined): number {
  return typeof v === "number" && Number.isFinite(v) ? v : 0;
}

/** Seconds on the wire to milliseconds; absent means no estimate. */
function secondsMs(v: number | undefined): number | null {
  return typeof v === "number" && Number.isFinite(v) && v >= 0
    ? v * 1000
    : null;
}

function mapStage(raw: WireStage): DeliveryStage {
  return {
    key: raw.key ?? "",
    state: mapState(raw.state),
    startedAt: str(raw.startedAt),
    finishedAt: str(raw.finishedAt),
    reason: str(raw.reason),
    components: raw.components ?? [],
    groups: raw.groups ?? [],
    gate: str(raw.gate),
    done: int(raw.done),
    total: int(raw.total),
    pr: raw.pr && raw.pr > 0 ? raw.pr : null,
    checks: str(raw.checks),
    failedAttempts: int(raw.failedAttempts),
    estimateMs: secondsMs(raw.estimateSeconds),
    slowestMs: secondsMs(raw.slowestSeconds),
  };
}

function mapCheck(raw: WireCheck): DeliveryCheck {
  return {
    component: raw.component ?? "",
    state: mapState(raw.state),
    startedAt: str(raw.startedAt),
    completedAt: str(raw.completedAt),
    url: str(raw.url),
    outsideBundle: raw.outsideBundle === true,
  };
}

function mapImageCheck(
  raw: WireImageCheck | null | undefined,
): DeliveryImageCheck | null {
  if (!raw?.state) {
    return null;
  }
  return {
    state: mapState(raw.state),
    url: str(raw.url),
    failures: (raw.failures ?? []).filter((f) => f !== ""),
  };
}

function mapBundle(raw: WireBundle | null | undefined): DeliveryBundle | null {
  if (!raw?.digest) {
    return null;
  }
  return {
    tag: raw.tag ?? "",
    version: raw.version ?? "",
    digest: raw.digest,
    publishedAt: str(raw.publishedAt),
    locked: raw.locked === true,
    releaseUrl: str(raw.releaseUrl),
  };
}

const OUTCOMES: readonly PromotionOutcome[] = [
  "passed",
  "failed",
  "replaced",
  "running",
  "unknown",
];

function mapPromotion(raw: WirePromotion): DeliveryPromotion {
  return {
    version: raw.version ?? "",
    digest: raw.digest ?? "",
    gitopsPr: raw.gitopsPr && raw.gitopsPr > 0 ? raw.gitopsPr : null,
    promotedAt: raw.promotedAt ?? "",
    url: str(raw.url),
    // An unrecognized outcome reads as Unknown, never as Passed.
    outcome: (OUTCOMES as readonly string[]).includes(raw.outcome ?? "")
      ? (raw.outcome as PromotionOutcome)
      : "unknown",
    gate: str(raw.gate),
    gatesPassedAt: str(raw.gatesPassedAt),
  };
}

function mapEnv(raw: WireEnv): DeliveryEnvState {
  return {
    env: raw.env ?? "",
    group: raw.group ?? "",
    state: mapState(raw.state),
    contains: raw.contains === true,
    history: (raw.history ?? []).map(mapPromotion),
    analysisUrl: str(raw.analysisUrl),
    argoUrl: str(raw.argoUrl),
  };
}

function mapChange(raw: WireChange): DeliveryChange {
  const pr = raw.gitopsPr;
  return {
    sha: raw.sha ?? "",
    shortSha: raw.shortSha ?? (raw.sha ?? "").slice(0, 7),
    mergedAt: raw.mergedAt ?? "",
    title: raw.title ?? "",
    author: str(raw.author),
    pr: raw.pr && raw.pr > 0 ? raw.pr : null,
    prUrl: str(raw.prUrl),
    commitUrl: str(raw.commitUrl),
    delivered: raw.delivered === true,
    deliveredAt: str(raw.deliveredAt),
    stages: (raw.stages ?? []).map(mapStage),
    builds: (raw.builds ?? []).map(mapCheck),
    policies: (raw.policies ?? []).map(mapCheck),
    bundle: mapBundle(raw.bundle),
    gitopsPr:
      pr?.number && pr.number > 0
        ? { number: pr.number, url: str(pr.url) }
        : null,
    environments: (raw.environments ?? []).map(mapEnv),
    imageCheck: mapImageCheck(raw.imageCheck),
  };
}

function mapStageDef(raw: WireStageDef): DeliveryStageDef {
  return {
    key: raw.key ?? "",
    kind: raw.kind === "group" ? "group" : "pipeline",
    group: str(raw.group),
  };
}

function mapSources(
  raw: Readonly<Record<string, WireSource>> | null | undefined,
): Partial<Record<DeliverySourceKey, DeliverySourceStatus>> {
  const out: Partial<Record<DeliverySourceKey, DeliverySourceStatus>> = {};
  for (const key of DELIVERY_SOURCE_KEYS) {
    const s = raw?.[key];
    if (s === undefined) {
      continue;
    }
    out[key] = {
      stale: s.stale === true,
      never: s.never === true,
      error: str(s.error),
      lastSuccess: str(s.lastSuccess),
    };
  }
  return out;
}

function mapCommitLink(raw: WireCommitLink): DeliveryCommitLink {
  return {
    sha: raw.sha ?? "",
    shortSha: raw.shortSha ?? (raw.sha ?? "").slice(0, 7),
    title: raw.title ?? "",
    pr: raw.pr && raw.pr > 0 ? raw.pr : null,
    url: raw.url ?? "",
    mergedAt: str(raw.mergedAt),
  };
}

function mapReleaseDiagnostics(
  raw: WireReleaseDiagnostics | null | undefined,
): ReleaseDiagnostics | null {
  if (!raw) {
    return null;
  }
  return {
    lastGood: mapBundle(raw.lastGood),
    logUrl: str(raw.logUrl),
    appUrl: str(raw.appUrl),
    commits: (raw.commits ?? []).map(mapCommitLink),
    commitsError: str(raw.commitsError),
    registryTag: str(raw.registryTag),
    githubReleaseTag: str(raw.githubReleaseTag),
    releaseError: str(raw.releaseError),
  };
}

export function mapDelivery(raw: unknown): DeliveryData {
  const w = (raw ?? {}) as WireDelivery;
  return {
    enabled: w.enabled === true,
    stages: (w.stages ?? []).map(mapStageDef),
    changes: (w.changes ?? []).map(mapChange),
    releaseDiagnostics: mapReleaseDiagnostics(w.releaseDiagnostics),
    omitted: int(w.omitted),
    sources: mapSources(w.sources),
  };
}
