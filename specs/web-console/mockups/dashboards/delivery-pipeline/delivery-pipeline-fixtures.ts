// Deterministic fixtures for the Glass Delivery tab mockup
// (fleet-dashboard-delivery-pipeline.spec, FLT-DLV-16). Every value is a
// pre-formatted example. Environment groups, instance names, bundle aliases,
// and pull requests are illustrative; production derives them at runtime
// (FLT-DLV-03, FLT-DLV-15).

export type DeliveryState =
  | "in-flight"
  | "change-selected"
  | "build-failed"
  | "release-overdue"
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
  /**
   * Running or Waiting longer than the slowest of the last 5 runs plus 5
   * minutes (FLT-DLV-04).
   */
  readonly overdue?: boolean;
  /** Where to look, for an Overdue or Failed stage (FLT-DLV-20). */
  readonly whereToLook?: WhereToLook;
  /** Failed promotion attempts in a Passed group (FLT-DLV-18). */
  readonly failedAttempts?: number;
};

/** One line of the Where to look popup (FLT-DLV-20). */
export type WhereToLookItem = {
  readonly term: string;
  readonly text: string;
  /** Link text; the mockup links to "#". */
  readonly link?: string;
  /** Flag the item, for example when two tags differ. */
  readonly warning?: string;
};

export type WhereToLook = {
  readonly phase: string;
  readonly summary: string;
  readonly items: readonly WhereToLookItem[];
};

/** One progress-bar cell and its label (FLT-DLV-19). */
export type ProgressCell = {
  readonly label: string;
  readonly state: StageState;
  /** Finish time, elapsed time, or arrival time or range. */
  readonly time?: string;
  readonly overdue?: boolean;
  readonly whereToLook?: WhereToLook;
};

export type Progress = {
  /** Omitted when an estimate is missing or a stage is Failed or Overdue. */
  readonly percent?: number;
  readonly cells: readonly ProgressCell[];
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
  readonly progress: Progress;
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

// Stage and Prod finished for the two older in-flight changes by 16:20.
const DELIVERED_TAIL: Record<string, StageCell> = {
  int: passed("15:13"),
  stage: passed("15:30"),
  prod: passed("15:41"),
};

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

const PHASES = [
  "Build",
  "Policy check",
  "Release",
  "GitOps update",
  "Int",
  "Stage",
  "Prod",
] as const;

// Builds the seven progress cells from per-phase state and time (FLT-DLV-19).
const progress = (
  cells: readonly (Omit<ProgressCell, "label"> | undefined)[],
  percent?: number,
): Progress => ({
  percent,
  cells: PHASES.map((label, i) => ({
    label,
    ...(cells[i] ?? { state: "not-started" as const }),
  })),
});

// Estimates in this fixture: Build 10, Policy check 3, Release 25, GitOps
// update 6, Int 18, Stage 15, Prod 12 minutes. Slowest of 5: 14, 5, 40, 10,
// 25, 22, 18. Before publication each group shows a range (FLT-DLV-10).
const ROW_997_BUILDING: NowNextRow = {
  shortSha: "a15cc91",
  pr: 997,
  title: "fix(control-plane): converge a new gateway in one provisioning pass",
  now: "Building control-plane - 6 min, usually 10 min",
  next: "Policy check - usually about 15:24",
  severity: "info",
  progress: progress(
    [
      { state: "running", time: "6 min" },
      undefined,
      undefined,
      undefined,
      { state: "not-started", time: "16:02 to 16:45" },
      { state: "not-started", time: "16:14 to 17:07" },
      { state: "not-started", time: "16:23 to 17:25" },
    ],
    8,
  ),
};

const BUILD_FAILED_LOOK: WhereToLook = {
  phase: "Build",
  summary:
    "Re-run the failed pipeline, or merge a change to the same component.",
  items: [
    {
      term: "Why",
      text: "A component builds only for changes under its own paths, so an unrelated change does not rebuild it.",
    },
    {
      term: "control-plane",
      text: "Failed at 15:19",
      link: "View failed check run",
    },
    // FLT-DLV-20.03: the image check run and its failure annotation.
    {
      term: "Image check",
      text: "Failed: No usable image for: control-plane.",
      link: "View image check",
    },
  ],
};

// The bundle publisher has not produced a bundle for 56 minutes; the slowest
// of the last 5 releases took 40 (FLT-DLV-04, FLT-DLV-20).
export const RELEASE_OVERDUE_LOOK: WhereToLook = {
  phase: "Release",
  summary: "The release has run longer than any of the last 5 releases.",
  items: [
    { term: "Last good release", text: "amber pickle, published 14:51" },
    {
      term: "Konflux",
      text: "Release pipeline runs for this application",
      link: "Open the release log",
    },
    {
      term: "Publisher changes",
      text: "3f9a1c2 (#996) fix(release): read the snapshot from the release",
      link: "View commit",
    },
    {
      term: "Release app",
      text: "Pending permission requests block publication",
      link: "Check the app's permissions",
    },
    {
      term: "Registry tag",
      text: "release-bundle-20261109T144705000000Z",
    },
    {
      term: "GitHub release",
      text: "release-bundle-20261109T092114000000Z",
      warning: "Differs from the registry",
    },
  ],
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
  progress: progress(
    [
      { state: "passed", time: "14:43" },
      { state: "passed", time: "14:46" },
      { state: "passed", time: "14:51" },
      { state: "passed", time: "14:55" },
      { state: "passed", time: "15:13" },
      { state: "running", time: "4 min" },
      { state: "waiting", time: "about 15:40" },
    ],
    shortSha === "c53fb9b" ? 67 : 71,
  ),
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
      next: "Re-run the failed pipeline, or merge a change to the same component",
      severity: "danger",
      progress: progress([
        { state: "failed", time: "15:19", whereToLook: BUILD_FAILED_LOOK },
      ]),
    },
    stageAnalysisRow(
      "c53fb9b",
      995,
      "feat(web-console): page the add-users picker",
    ),
    stageAnalysisRow("9e1d2aa", 994, "docs(spec): clarify bundle containment"),
  ],
  changes: [
    C997({
      state: "failed",
      primary: "15:19",
      detail: "control-plane",
      whereToLook: BUILD_FAILED_LOOK,
    }),
    C995(),
    C994(),
    ...OLDER,
  ],
};

const RELEASE_OVERDUE_CHANGE: Change = {
  ...C997(passed("15:21", "control-plane")),
  stages: {
    merged: passed("15:10"),
    build: passed("15:21", "control-plane"),
    policy: passed("15:24", "control-plane"),
    release: {
      state: "running",
      primary: "56 min",
      detail: "Releasing (inferred)",
      overdue: true,
      whereToLook: RELEASE_OVERDUE_LOOK,
    },
    gitops: notStarted,
    int: notStarted,
    stage: notStarted,
    prod: notStarted,
  },
};

const RELEASE_OVERDUE: DeliveryFixture = {
  asOf: "16:20",
  summarySeverity: "warning",
  summary: [
    {
      shortSha: "a15cc91",
      pr: 997,
      title:
        "fix(control-plane): converge a new gateway in one provisioning pass",
      now: "Releasing (inferred) - 56 min, usually 25 min - Overdue",
      next: "GitOps update - no arrival time while the release is overdue",
      severity: "warning",
      progress: progress([
        { state: "passed", time: "15:21" },
        { state: "passed", time: "15:24" },
        {
          state: "running",
          time: "56 min",
          overdue: true,
          whereToLook: RELEASE_OVERDUE_LOOK,
        },
      ]),
    },
  ],
  changes: [RELEASE_OVERDUE_CHANGE, C995(), C994(), ...OLDER].map((c) =>
    c.sha === RELEASE_OVERDUE_CHANGE.sha
      ? c
      : { ...c, stages: { ...c.stages, ...DELIVERED_TAIL } },
  ),
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
  "release-overdue": RELEASE_OVERDUE,
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
