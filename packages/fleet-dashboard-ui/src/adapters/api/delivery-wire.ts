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
  type DeliveryNow,
  type DeliveryNextEvent,
  type DeliveryPartly,
  type DeliveryRestart,
  type DeliveryTrainBundle,
  type MissingState,
  type NextEventKind,
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
  readonly waitingFor?: readonly string[] | null;
  readonly rebuiltIn?: {
    readonly sha?: string;
    readonly shortSha?: string;
    readonly pr?: number;
  } | null;
  readonly partly?: {
    readonly done?: number;
    readonly total?: number;
    readonly missing?:
      | readonly {
          readonly component?: string;
          readonly state?: string;
        }[]
      | null;
  } | null;
  readonly restart?: {
    readonly components?: readonly string[] | null;
    readonly version?: string;
    readonly digest?: string;
    readonly envs?: readonly string[] | null;
    readonly landsAt?: string;
  } | null;
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
  readonly supersededBy?: string;
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
  readonly waitingFor?: readonly string[] | null;
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

interface WireTrainBundle {
  readonly tag?: string;
  readonly version?: string;
  readonly digest?: string;
  readonly publishedAt?: string;
  readonly locked?: boolean;
  readonly lockedAt?: string;
  readonly changed?:
    | readonly {
        readonly component?: string;
        readonly revision?: string;
      }[]
    | null;
  readonly same?: boolean;
  readonly inferred?: boolean;
  readonly builtAt?: string;
  readonly policyAt?: string;
  readonly expectedAt?: string;
  readonly active?: readonly string[] | null;
  readonly proposed?: readonly string[] | null;
  readonly replaced?: readonly string[] | null;
  readonly restarts?: readonly string[] | null;
}

interface WireNow {
  readonly appHead?: {
    readonly sha?: string;
    readonly shortSha?: string;
    readonly pr?: number;
    readonly title?: string;
    readonly mergedAt?: string;
  } | null;
  readonly gitopsHead?: {
    readonly digest?: string;
    readonly version?: string;
    readonly publishedAt?: string;
    readonly lockedAt?: string;
    readonly pr?: number;
  } | null;
  readonly checks?:
    | readonly {
        readonly kind?: string;
        readonly component?: string;
        readonly startedAt?: string;
        readonly shortSha?: string;
        readonly pr?: number;
      }[]
    | null;
  readonly bundlePr?: {
    readonly number?: number;
    readonly url?: string;
    readonly checks?: string;
  } | null;
  readonly environments?:
    | readonly {
        readonly env?: string;
        readonly group?: string;
        readonly digest?: string;
        readonly version?: string;
        readonly state?: string;
        readonly gate?: string;
        readonly since?: string;
        readonly passedAt?: string;
        readonly attempt?: number;
        readonly attemptOf?: number;
        readonly proposedVersion?: string;
        readonly proposedDigest?: string;
        readonly waitingFor?: readonly string[] | null;
      }[]
    | null;
  readonly next?:
    | readonly {
        readonly kind?: string;
        readonly components?: readonly string[] | null;
        readonly version?: string;
        readonly digest?: string;
        readonly envs?: readonly string[] | null;
        readonly after?: readonly string[] | null;
        readonly at?: string;
      }[]
    | null;
}

interface WireDelivery {
  readonly enabled?: boolean;
  readonly stages?: readonly WireStageDef[] | null;
  readonly changes?: readonly WireChange[] | null;
  readonly train?: readonly WireTrainBundle[] | null;
  readonly now?: WireNow | null;
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
    waitingFor: raw.waitingFor ?? [],
    gate: str(raw.gate),
    done: int(raw.done),
    total: int(raw.total),
    pr: raw.pr && raw.pr > 0 ? raw.pr : null,
    checks: str(raw.checks),
    failedAttempts: int(raw.failedAttempts),
    estimateMs: secondsMs(raw.estimateSeconds),
    slowestMs: secondsMs(raw.slowestSeconds),
    rebuiltIn: raw.rebuiltIn?.sha
      ? {
          sha: raw.rebuiltIn.sha,
          shortSha: raw.rebuiltIn.shortSha ?? raw.rebuiltIn.sha.slice(0, 7),
          pr: positive(raw.rebuiltIn.pr),
        }
      : null,
    partly: mapPartly(raw.partly),
    restart: mapRestart(raw.restart),
  };
}

function positive(v: number | undefined): number | null {
  return typeof v === "number" && v > 0 ? v : null;
}

const MISSING: readonly MissingState[] = [
  "building",
  "policy",
  "releasing",
  "failed",
];

function mapPartly(raw: WireStage["partly"]): DeliveryPartly | null {
  if (!raw || int(raw.total) <= 0) {
    return null;
  }
  return {
    done: int(raw.done),
    total: int(raw.total),
    missing: (raw.missing ?? []).map((x) => ({
      component: x.component ?? "",
      // An unrecognized state reads as the earliest one, never as releasing.
      state: (MISSING as readonly string[]).includes(x.state ?? "")
        ? (x.state as MissingState)
        : "building",
    })),
  };
}

function mapRestart(raw: WireStage["restart"]): DeliveryRestart | null {
  if (!raw) {
    return null;
  }
  return {
    components: raw.components ?? [],
    version: str(raw.version),
    digest: str(raw.digest),
    envs: raw.envs ?? [],
    landsAt: str(raw.landsAt),
  };
}

function mapTrainBundle(raw: WireTrainBundle): DeliveryTrainBundle {
  return {
    tag: str(raw.tag),
    version: str(raw.version),
    digest: str(raw.digest),
    publishedAt: str(raw.publishedAt),
    locked: raw.locked === true,
    lockedAt: str(raw.lockedAt),
    changed: (raw.changed ?? []).map((c) => ({
      component: c.component ?? "",
      revision: str(c.revision),
    })),
    same: raw.same === true,
    inferred: raw.inferred === true,
    builtAt: str(raw.builtAt),
    policyAt: str(raw.policyAt),
    expectedAt: str(raw.expectedAt),
    active: raw.active ?? [],
    proposed: raw.proposed ?? [],
    replaced: raw.replaced ?? [],
    restarts: raw.restarts ?? [],
  };
}

const NEXT_KINDS: readonly NextEventKind[] = [
  "bundle-published",
  "locked",
  "restart",
  "promote",
];

function mapNow(raw: WireNow | null | undefined): DeliveryNow {
  const app = raw?.appHead;
  const gitops = raw?.gitopsHead;
  const pr = raw?.bundlePr;
  return {
    appHead: app?.sha
      ? {
          sha: app.sha,
          shortSha: app.shortSha ?? app.sha.slice(0, 7),
          pr: positive(app.pr),
          title: app.title ?? "",
          mergedAt: str(app.mergedAt),
        }
      : null,
    gitopsHead: gitops?.version
      ? {
          digest: gitops.digest ?? "",
          version: gitops.version,
          publishedAt: str(gitops.publishedAt),
          lockedAt: str(gitops.lockedAt),
          pr: positive(gitops.pr),
        }
      : null,
    checks: (raw?.checks ?? []).map((c) => ({
      kind: c.kind === "policy" ? "policy" : "build",
      component: c.component ?? "",
      startedAt: str(c.startedAt),
      shortSha: c.shortSha ?? "",
      pr: positive(c.pr),
    })),
    bundlePr: pr?.number
      ? { number: pr.number, url: str(pr.url), checks: str(pr.checks) }
      : null,
    environments: (raw?.environments ?? []).map((e) => ({
      env: e.env ?? "",
      group: e.group ?? "",
      digest: str(e.digest),
      version: str(e.version),
      state: mapState(e.state),
      gate: str(e.gate),
      since: str(e.since),
      passedAt: str(e.passedAt),
      attempt: positive(e.attempt),
      attemptOf: positive(e.attemptOf),
      proposedVersion: str(e.proposedVersion),
      proposedDigest: str(e.proposedDigest),
      waitingFor: e.waitingFor ?? [],
    })),
    next: (raw?.next ?? []).flatMap((e): DeliveryNextEvent[] =>
      (NEXT_KINDS as readonly string[]).includes(e.kind ?? "")
        ? [
            {
              kind: e.kind as NextEventKind,
              components: e.components ?? [],
              version: str(e.version),
              digest: str(e.digest),
              envs: e.envs ?? [],
              after: e.after ?? [],
              at: str(e.at),
            },
          ]
        : [],
    ),
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
    supersededBy: str(raw.supersededBy),
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
    waitingFor: raw.waitingFor ?? [],
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
    train: (w.train ?? []).map(mapTrainBundle),
    now: mapNow(w.now),
    releaseDiagnostics: mapReleaseDiagnostics(w.releaseDiagnostics),
    omitted: int(w.omitted),
    sources: mapSources(w.sources),
  };
}
