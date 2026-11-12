// Deterministic fixtures for the Glass Delivery tab mockup
// (fleet-dashboard-delivery-pipeline.spec, FLT-DLV-16). Every value is a
// pre-formatted example. Environment groups, instance names, bundle aliases,
// and pull requests are illustrative; production derives them at runtime
// (FLT-DLV-03, FLT-DLV-15).

export type DeliveryState =
  | "in-flight"
  | "change-selected"
  | "build-failed"
  | "slower-than-usual"
  | "all-delivered"
  | "promotion-retried"
  | "source-unavailable"
  | "loading"
  | "not-configured"
  | "error";

export type StageState =
  | "not-started"
  | "waiting"
  | "running"
  | "passed"
  | "failed"
  | "not-needed"
  | "unknown";

export type StageCell = {
  readonly state: StageState;
  /** Time, elapsed duration, or "n of m". */
  readonly primary?: string;
  /** Reason, components, or bundle. */
  readonly detail?: string;
  /** Running or Waiting for more than twice the typical duration (FLT-DLV-04). */
  readonly slow?: boolean;
  /** Failed promotion attempts in a Passed group (FLT-DLV-18). */
  readonly failedAttempts?: number;
};

export type StageColumn = { readonly key: string; readonly label: string };

// Fixed pipeline stages, then environment groups derived from the promotion
// graph at runtime (FLT-DLV-03).
export const STAGES: readonly StageColumn[] = [
  { key: "merged", label: "Merged" },
  { key: "build", label: "Build" },
  { key: "policy", label: "Policy check" },
  { key: "release", label: "Release" },
  { key: "gitops", label: "GitOps update" },
  { key: "int", label: "Int" },
  { key: "stage", label: "Stage" },
  { key: "prod", label: "Prod" },
];

export type Change = {
  readonly sha: string;
  readonly shortSha: string;
  readonly pr?: number;
  readonly title: string;
  readonly author: string;
  readonly stages: Readonly<Record<string, StageCell>>;
};

export type Severity = "info" | "warning" | "danger" | "good";

export type NowNextRow = {
  readonly shortSha: string;
  readonly pr?: number;
  readonly title: string;
  readonly now: string;
  readonly next: string;
  readonly severity: Severity;
};

export type DeliveryFixture = {
  readonly asOf: string;
  readonly summarySeverity: Severity;
  readonly summary: readonly NowNextRow[];
  /** Shown instead of rows when nothing is in flight (FLT-DLV-10.02). */
  readonly deliveredSummary?: string;
  readonly changes: readonly Change[];
  readonly sourceAlert?: { readonly title: string; readonly body: string };
};

const passed = (primary: string, detail?: string): StageCell => ({
  state: "passed",
  primary,
  detail,
});
const notStarted: StageCell = { state: "not-started" };

const delivered = (
  merged: string,
  build: StageCell,
  policy: StageCell,
  release: string,
  bundle: string,
  gitops: string,
  gitopsPr: string,
  int: string,
  stage: string,
  prod: string,
): Record<string, StageCell> => ({
  merged: passed(merged),
  build,
  policy,
  release: passed(release, bundle),
  gitops: passed(gitops, gitopsPr),
  int: passed(int),
  stage: passed(stage),
  prod: passed(prod),
});

const OLDER: readonly Change[] = [
  {
    sha: "4b7c0de1f2a3b4c5d6e7f8091a2b3c4d5e6f7081",
    shortSha: "4b7c0de",
    pr: 990,
    title: "fix(api-server): return 404 for a deleted gateway",
    author: "kim",
    stages: delivered(
      "10:58",
      passed("11:08", "api-server"),
      passed("11:11", "api-server"),
      "11:15",
      "quiet heron",
      "11:19",
      "GitOps #2329",
      "11:36",
      "11:41",
      "11:52",
    ),
  },
  {
    sha: "7f3e9ab0c1d2e3f405162738495a6b7c8d9e0f12",
    shortSha: "7f3e9ab",
    pr: 988,
    title: "chore(deps): bump OpenShell to the latest patch release",
    author: "dana",
    stages: delivered(
      "09:40",
      passed("09:52", "control-plane, agent-runtime"),
      passed("09:55", "control-plane, agent-runtime"),
      "10:00",
      "lucky otter",
      "10:04",
      "GitOps #2327",
      "10:22",
      "10:31",
      "10:44",
    ),
  },
];

const INT_DONE_STAGE_ANALYSIS: Record<string, StageCell> = {
  int: passed("15:13"),
  // One of the two Stage environments already runs the change (FLT-DLV-08).
  stage: {
    state: "running",
    primary: "1 of 2",
    detail: "Release analysis running - 4 min",
  },
  prod: { state: "waiting", detail: "Waiting for Stage" },
};

const C995 = (overrides: Partial<Record<string, StageCell>> = {}): Change => ({
  sha: "c53fb9bcc1d0adde8c4b5138071d31d3b6ba89f3",
  shortSha: "c53fb9b",
  pr: 995,
  title: "feat(web-console): page the add-users picker",
  author: "dana",
  stages: {
    merged: passed("14:31"),
    build: passed("14:43", "web-console, cli"),
    policy: passed("14:46", "web-console, cli"),
    release: passed("14:51", "amber pickle"),
    gitops: passed("14:55", "GitOps #2331"),
    ...INT_DONE_STAGE_ANALYSIS,
    ...overrides,
  } as Record<string, StageCell>,
});

const C994 = (overrides: Partial<Record<string, StageCell>> = {}): Change => ({
  sha: "9e1d2aa4b5c6d7e8f90a1b2c3d4e5f6a7b8c9d0e",
  shortSha: "9e1d2aa",
  pr: 994,
  title: "docs(spec): clarify bundle containment",
  author: "kim",
  stages: {
    merged: passed("14:20"),
    build: { state: "not-needed", detail: "No component changed" },
    policy: { state: "not-needed", detail: "No component changed" },
    release: passed("14:51", "amber pickle, with #995"),
    gitops: passed("14:55", "GitOps #2331"),
    ...INT_DONE_STAGE_ANALYSIS,
    ...overrides,
  } as Record<string, StageCell>,
});

const C997 = (build: StageCell): Change => ({
  sha: "a15cc9192184c24137c9e944bfe93eb5d5ec6803",
  shortSha: "a15cc91",
  pr: 997,
  title: "fix(control-plane): converge a new gateway in one provisioning pass",
  author: "kim",
  stages: {
    merged: passed("15:10"),
    build,
    policy: notStarted,
    release: notStarted,
    gitops: notStarted,
    int: notStarted,
    stage: notStarted,
    prod: notStarted,
  },
});

const ROW_997_BUILDING: NowNextRow = {
  shortSha: "a15cc91",
  pr: 997,
  title: "fix(control-plane): converge a new gateway in one provisioning pass",
  now: "Building control-plane - 6 min, usually 10 min",
  next: "Policy check - usually about 15:24",
  severity: "info",
};

const stageAnalysisRow = (
  shortSha: string,
  pr: number,
  title: string,
): NowNextRow => ({
  shortSha,
  pr,
  title,
  now: "Stage: release analysis running - 4 min, usually 15 min",
  next: "Prod promotes after Stage passes - usually about 15:40",
  severity: "info",
});

const IN_FLIGHT: DeliveryFixture = {
  asOf: "15:17",
  summarySeverity: "info",
  summary: [
    ROW_997_BUILDING,
    stageAnalysisRow(
      "c53fb9b",
      995,
      "feat(web-console): page the add-users picker",
    ),
    stageAnalysisRow("9e1d2aa", 994, "docs(spec): clarify bundle containment"),
  ],
  changes: [
    C997({ state: "running", primary: "6 min", detail: "control-plane" }),
    C995(),
    C994(),
    ...OLDER,
  ],
};

const BUILD_FAILED: DeliveryFixture = {
  asOf: "15:21",
  summarySeverity: "danger",
  summary: [
    {
      ...ROW_997_BUILDING,
      now: "Build failed: control-plane at 15:19",
      next: "A new change to main starts a new build",
      severity: "danger",
    },
    stageAnalysisRow(
      "c53fb9b",
      995,
      "feat(web-console): page the add-users picker",
    ),
    stageAnalysisRow("9e1d2aa", 994, "docs(spec): clarify bundle containment"),
  ],
  changes: [
    C997({ state: "failed", primary: "15:19", detail: "control-plane" }),
    C995(),
    C994(),
    ...OLDER,
  ],
};

const WAITING_FOR_PR: Partial<Record<string, StageCell>> = {
  gitops: {
    state: "waiting",
    primary: "14 min",
    detail: "Waiting for the bundle update pull request",
    slow: true,
  },
  int: notStarted,
  stage: notStarted,
  prod: notStarted,
};

const slowRow = (shortSha: string, pr: number, title: string): NowNextRow => ({
  shortSha,
  pr,
  title,
  now: "GitOps update: waiting for the bundle update pull request - 14 min, usually 4 min",
  next: "Int - usually about 15:23",
  severity: "warning",
});

const SLOWER_THAN_USUAL: DeliveryFixture = {
  asOf: "15:05",
  summarySeverity: "warning",
  summary: [
    slowRow("c53fb9b", 995, "feat(web-console): page the add-users picker"),
    slowRow("9e1d2aa", 994, "docs(spec): clarify bundle containment"),
  ],
  changes: [C995(WAITING_FOR_PR), C994(WAITING_FOR_PR), ...OLDER],
};

const ALL_DELIVERED: DeliveryFixture = {
  asOf: "16:10",
  summarySeverity: "good",
  summary: [],
  deliveredSummary:
    "Everything on main is delivered. The newest change, a15cc91 (#997), reached every environment at 16:02, 51 min after merge.",
  changes: [
    {
      ...C997(passed("15:21", "control-plane")),
      stages: delivered(
        "15:10",
        passed("15:21", "control-plane"),
        passed("15:25", "control-plane"),
        "15:29",
        "brave walnut",
        "15:34",
        "GitOps #2334",
        "15:36",
        "15:52",
        "16:02",
      ),
    },
    {
      ...C995(),
      stages: delivered(
        "14:31",
        passed("14:43", "web-console, cli"),
        passed("14:46", "web-console, cli"),
        "14:51",
        "amber pickle",
        "14:55",
        "GitOps #2331",
        "15:13",
        "15:27",
        "15:38",
      ),
    },
    ...OLDER,
  ],
};

const unknownRelease = (change: Change): Change => ({
  ...change,
  stages: { ...change.stages, release: { state: "unknown" } },
});

const SOURCE_UNAVAILABLE: DeliveryFixture = {
  ...IN_FLIGHT,
  changes: IN_FLIGHT.changes.map(unknownRelease),
  sourceAlert: {
    title: "Bundle registry unavailable",
    body: "The Release stage shows Unknown until the registry can be read. Other stages are current.",
  },
};

// A change whose first bundle failed the Int hub's release analysis; a later
// bundle carried it through (FLT-DLV-18).
const C985: Change = {
  sha: "5d2e8c1a9b0f4e3d2c1b0a9f8e7d6c5b4a392817",
  shortSha: "5d2e8c1",
  pr: 985,
  title: "docs: clarify behavior and configuration contracts",
  author: "kim",
  stages: {
    merged: passed("20:19"),
    build: { state: "not-needed", detail: "No component changed" },
    policy: { state: "not-needed", detail: "No component changed" },
    release: passed("22:52", "brisk falcon"),
    gitops: passed("22:56", "GitOps #2310"),
    int: { state: "passed", primary: "23:00", failedAttempts: 1 },
    stage: passed("06:12"),
    prod: passed("06:21"),
  },
};

const PROMOTION_RETRIED: DeliveryFixture = {
  asOf: "09:30",
  summarySeverity: "good",
  summary: [],
  deliveredSummary:
    "Everything on main is delivered. The newest change, 5d2e8c1 (#985), reached every environment at 06:21.",
  changes: [C985, ...OLDER],
};

export const FIXTURES: Readonly<
  Record<
    Exclude<DeliveryState, "loading" | "not-configured" | "error">,
    DeliveryFixture
  >
> = {
  "in-flight": IN_FLIGHT,
  "change-selected": IN_FLIGHT,
  "build-failed": BUILD_FAILED,
  "slower-than-usual": SLOWER_THAN_USUAL,
  "all-delivered": ALL_DELIVERED,
  "promotion-retried": PROMOTION_RETRIED,
  "source-unavailable": SOURCE_UNAVAILABLE,
};

// Details for the change opened in the ChangeSelected state (FLT-DLV-11).
export type TimelineStep = {
  readonly label: string;
  readonly state: StageState;
  readonly description: string;
};

export type ComponentCheck = {
  readonly component: string;
  readonly build: string;
  readonly policy: string;
};

export type PromotionOutcome =
  "passed" | "failed" | "replaced" | "running" | "unknown";

/** One promotion of a bundle containing the change (FLT-DLV-18). */
export type PromotionAttempt = {
  readonly bundle: string;
  readonly gitopsPr: string;
  readonly promoted: string;
  readonly outcome: PromotionOutcome;
  /** The gate that failed or was still running. */
  readonly detail?: string;
};

export type EnvironmentRow = {
  readonly group: string;
  readonly instance: string;
  readonly cell: StageCell;
  /** Oldest first; omitted when the change reached it on the first try. */
  readonly history?: readonly PromotionAttempt[];
};

export type ChangeDetail = {
  readonly shortSha: string;
  readonly pr: number;
  readonly title: string;
  readonly author: string;
  readonly timeline: readonly TimelineStep[];
  readonly checks: readonly ComponentCheck[];
  readonly bundle: {
    readonly alias: string;
    readonly tag: string;
    readonly digest: string;
    readonly published: string;
    readonly downloads: string;
  };
  readonly gitopsPr: string;
  readonly environments: readonly EnvironmentRow[];
};

export const SELECTED_SHA = "c53fb9bcc1d0adde8c4b5138071d31d3b6ba89f3";

export const SELECTED_DETAIL: ChangeDetail = {
  shortSha: "c53fb9b",
  pr: 995,
  title: "feat(web-console): page the add-users picker",
  author: "dana",
  timeline: [
    { label: "Merged", state: "passed", description: "14:31" },
    { label: "Build", state: "passed", description: "14:31 to 14:43 - 12 min" },
    {
      label: "Policy check",
      state: "passed",
      description: "14:43 to 14:46 - 3 min",
    },
    {
      label: "Release",
      state: "passed",
      description: "14:46 to 14:51 - 5 min",
    },
    {
      label: "GitOps update",
      state: "passed",
      description: "14:51 to 14:55 - 4 min",
    },
    { label: "Int", state: "passed", description: "14:55 to 15:13 - 18 min" },
    {
      label: "Stage",
      state: "running",
      description: "Release analysis running - 4 min, usually 15 min",
    },
    { label: "Prod", state: "waiting", description: "Waiting for Stage" },
  ] satisfies readonly TimelineStep[],
  checks: [
    {
      component: "web-console",
      build: "Passed - 12 min",
      policy: "Passed - 3 min",
    },
    { component: "cli", build: "Passed - 9 min", policy: "Passed - 2 min" },
  ] satisfies readonly ComponentCheck[],
  bundle: {
    alias: "amber pickle",
    tag: "release-bundle-20261109T144705000000Z-3c1d9e0f7a2b4c68",
    digest:
      "sha256:9b2f4c7e1a0d3f6b8c5e2a9d7f4b1c8e5a2d9f6c3b0e7a4d1f8c5b2e9a6d3f0c",
    published: "14:51",
    downloads: "Downloads published 14:53",
  },
  gitopsPr: "GitOps #2331 - merged 14:55",
  environments: [
    { group: "Int", instance: "int-hub", cell: passed("15:13") },
    { group: "Int", instance: "int-spoke-a", cell: passed("15:11") },
    { group: "Int", instance: "int-spoke-b", cell: passed("15:12") },
    {
      group: "Stage",
      instance: "stage-hub",
      cell: {
        state: "running",
        primary: "4 min",
        detail: "Release analysis running",
      },
    },
    { group: "Stage", instance: "stage-spoke-a", cell: passed("15:14") },
    {
      group: "Prod",
      instance: "prod-hub",
      cell: { state: "waiting", detail: "Waiting for Stage" },
    },
    {
      group: "Prod",
      instance: "prod-spoke-a",
      cell: { state: "waiting", detail: "Waiting for Stage" },
    },
    {
      group: "Prod",
      instance: "prod-spoke-b",
      cell: { state: "waiting", detail: "Waiting for Stage" },
    },
  ] satisfies readonly EnvironmentRow[],
};

export const RETRIED_SHA = C985.sha;

const attempt = (
  bundle: string,
  gitopsPr: string,
  promoted: string,
  outcome: PromotionOutcome,
  detail?: string,
): PromotionAttempt => ({ bundle, gitopsPr, promoted, outcome, detail });

// The change opened in the PromotionRetried state (FLT-DLV-18).
export const RETRIED_DETAIL: ChangeDetail = {
  shortSha: "5d2e8c1",
  pr: 985,
  title: "docs: clarify behavior and configuration contracts",
  author: "kim",
  timeline: [
    { label: "Merged", state: "passed", description: "20:19" },
    {
      label: "Build",
      state: "not-needed",
      description: "No component changed",
    },
    {
      label: "Policy check",
      state: "not-needed",
      description: "No component changed",
    },
    { label: "Release", state: "passed", description: "22:52 - brisk falcon" },
    {
      label: "GitOps update",
      state: "passed",
      description: "22:52 to 22:56 - 4 min",
    },
    {
      label: "Int",
      state: "passed",
      description: "22:56 to 23:00 - 1 failed attempt",
    },
    { label: "Stage", state: "passed", description: "06:12" },
    { label: "Prod", state: "passed", description: "06:21" },
  ],
  checks: [],
  bundle: {
    alias: "brisk falcon",
    tag: "release-bundle-20261106T224836000000Z-bcd66abfb4e7129a",
    digest:
      "sha256:4e1a7c9d2b5f8e0a3c6d9f2b5e8a1c4d7f0b3e6a9c2d5f8b1e4a7c0d3f6b9e2a",
    published: "22:52",
    downloads: "Downloads published 22:54",
  },
  gitopsPr: "GitOps #2310 - merged 22:56",
  environments: [
    {
      group: "Int",
      instance: "int-hub",
      cell: passed("23:00"),
      history: [
        attempt(
          "brisk falcon",
          "GitOps #2310",
          "23:00",
          "failed",
          "Release analysis failed",
        ),
        attempt("calm meadow", "GitOps #2314", "23:10", "passed"),
      ],
    },
    { group: "Int", instance: "int-spoke-a", cell: passed("23:00") },
    { group: "Stage", instance: "stage-hub", cell: passed("06:12") },
    {
      group: "Prod",
      instance: "prod-hub",
      cell: passed("06:20"),
      history: [
        attempt(
          "calm meadow",
          "GitOps #2314",
          "06:20",
          "replaced",
          "Release analysis was running",
        ),
        attempt("swift harbor", "GitOps #2319", "06:31", "passed"),
      ],
    },
    { group: "Prod", instance: "prod-spoke-a", cell: passed("06:21") },
  ],
};
