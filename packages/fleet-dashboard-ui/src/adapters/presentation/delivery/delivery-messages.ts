// Delivery tab copy (fleet-dashboard-delivery-pipeline.spec). Every entry has an
// explicit id, default message, and description. No fleet identity: environment
// groups, instances, and bundle names are data.

import { defineMessages } from "react-intl";

export const deliveryMessages = defineMessages({
  tabTitle: {
    id: "fleet.delivery.tab",
    defaultMessage: "Delivery",
    description:
      "Top-level Glass tab that follows changes from merge to every environment (FLT-DLV-01).",
  },
  pageTitle: {
    id: "fleet.delivery.title",
    defaultMessage: "Delivery",
    description: "Delivery page heading (FLT-DLV-01).",
  },
  pageDescription: {
    id: "fleet.delivery.description",
    defaultMessage:
      "Follow each change on main from merge, through build and release, to every environment.",
    description:
      "One-sentence description under the Delivery heading (FLT-DLV-01).",
  },
  asOf: {
    id: "fleet.delivery.asOf",
    defaultMessage: "Data as of {time}",
    description: "When the delivery data was generated.",
  },
  refresh: {
    id: "fleet.delivery.refresh",
    defaultMessage: "Refresh",
    description: "Accessible name of the Delivery refresh button.",
  },
  loading: {
    id: "fleet.delivery.loading",
    defaultMessage: "Loading delivery data",
    description: "Delivery loading state.",
  },
  notConfiguredTitle: {
    id: "fleet.delivery.notConfigured.title",
    defaultMessage: "Delivery source not configured",
    description: "Empty state when the delivery plane is off (FLT-DLV-01.02).",
  },
  notConfiguredBody: {
    id: "fleet.delivery.notConfigured.body",
    defaultMessage:
      "Set the bundle registry repository for this dashboard to follow changes from merge to every environment. The other tabs are not affected.",
    description:
      "Empty state body when the delivery plane is off (FLT-DLV-01.02).",
  },
  errorTitle: {
    id: "fleet.delivery.error.title",
    defaultMessage: "Unable to load delivery data",
    description: "Danger alert title when /api/delivery fails (FLT-DLV-13).",
  },
  errorBody: {
    id: "fleet.delivery.error.body",
    defaultMessage:
      "The delivery data could not be loaded. Try refreshing the page.",
    description: "Danger alert body when /api/delivery fails (FLT-DLV-13).",
  },
  noChangesTitle: {
    id: "fleet.delivery.noChanges.title",
    defaultMessage: "No changes to show",
    description:
      "Empty state when the change list cannot be built (FLT-DLV-13).",
  },
  noChangesBody: {
    id: "fleet.delivery.noChanges.body",
    defaultMessage: "The application repository has not been read yet.",
    description:
      "Empty state body when the application repository never succeeded (FLT-DLV-13).",
  },
  // Sources (FLT-DLV-13).
  sourceUpstream: {
    id: "fleet.delivery.source.upstream",
    defaultMessage: "Application repository",
    description: "Name of the application repository source.",
  },
  sourceRegistry: {
    id: "fleet.delivery.source.registry",
    defaultMessage: "Bundle registry",
    description: "Name of the release bundle registry source.",
  },
  sourceGitops: {
    id: "fleet.delivery.source.gitops",
    defaultMessage: "GitOps repository",
    description: "Name of the GitOps repository source.",
  },
  sourcePromotion: {
    id: "fleet.delivery.source.promotion",
    defaultMessage: "Promotion data",
    description: "Name of the promotion source.",
  },
  sourceHistory: {
    id: "fleet.delivery.source.history",
    defaultMessage: "Promotion history",
    description: "Name of the per-environment promotion history source.",
  },
  historyStages: {
    id: "fleet.delivery.source.historyStages",
    defaultMessage: "Promotion times and attempts",
    description:
      "Names what the promotion history source feeds, in a source alert (FLT-DLV-13).",
  },
  failedAttempts: {
    id: "fleet.delivery.failedAttempts",
    defaultMessage:
      "{count, plural, one {# failed attempt} other {# failed attempts}}",
    description:
      "A Passed environment group that needed more than one bundle (FLT-DLV-18).",
  },
  outcomeReplaced: {
    id: "fleet.delivery.outcome.replaced",
    defaultMessage: "Replaced",
    description:
      "A promotion that a newer one replaced before its gates finished (FLT-DLV-18).",
  },
  attempt: {
    id: "fleet.delivery.attempt",
    defaultMessage: "{outcome}: {bundle} ({gitops}), promoted {time}",
    description:
      "One promotion in an environment's history: outcome, bundle, GitOps update, time.",
  },
  attemptNoGitops: {
    id: "fleet.delivery.attempt.noGitops",
    defaultMessage: "{outcome}: {bundle}, promoted {time}",
    description:
      "One promotion in an environment's history when its GitOps update is unknown.",
  },
  attemptGitops: {
    id: "fleet.delivery.attempt.gitops",
    defaultMessage: "GitOps #{number}",
    description: "The GitOps update pull request of one promotion.",
  },
  attemptGateFailed: {
    id: "fleet.delivery.attempt.gateFailed",
    defaultMessage: "{gate} failed",
    description: "Why a promotion failed.",
  },
  attemptGateRunning: {
    id: "fleet.delivery.attempt.gateRunning",
    defaultMessage: "{gate} was running",
    description:
      "The gate that was still running when a promotion was replaced.",
  },
  attemptGateRunningNow: {
    id: "fleet.delivery.attempt.gateRunningNow",
    defaultMessage: "{gate} is running",
    description: "The gate that is running on the active promotion.",
  },
  promotionHistoryLabel: {
    id: "fleet.delivery.promotionHistory",
    defaultMessage: "Promotions in {env}",
    description: "Accessible name of an environment's promotion history list.",
  },
  sourceNeverTitle: {
    id: "fleet.delivery.source.never.title",
    defaultMessage: "{source} unavailable",
    description: "Warning alert title for a source that has never been read.",
  },
  sourceNeverBody: {
    id: "fleet.delivery.source.never.body",
    defaultMessage:
      "{stages} show Unknown until it can be read. Other stages are current.",
    description: "Warning alert body for a source that has never been read.",
  },
  sourceStaleTitle: {
    id: "fleet.delivery.source.stale.title",
    defaultMessage: "{source} data is from {time}",
    description: "Warning alert title for a stale source.",
  },
  sourceStaleBody: {
    id: "fleet.delivery.source.stale.body",
    defaultMessage:
      "{stages} show their last known state. Other stages are current.",
    description: "Warning alert body for a stale source.",
  },
  environmentStages: {
    id: "fleet.delivery.source.environmentStages",
    defaultMessage: "Environment stages",
    description: "Names the environment-group stages in a source alert.",
  },
  omitted: {
    id: "fleet.delivery.omitted",
    defaultMessage:
      "{count, plural, one {# older undelivered change is} other {# older undelivered changes are}} not shown.",
    description: "Count of undelivered changes beyond the window (FLT-DLV-02).",
  },
  changeOutsideWindow: {
    id: "fleet.delivery.changeOutsideWindow",
    defaultMessage: "The selected change is outside the current window.",
    description:
      "Info alert for a change parameter not in the payload (FLT-DLV-14).",
  },
  // Stages (FLT-DLV-03).
  stageMerged: {
    id: "fleet.delivery.stage.merged",
    defaultMessage: "Merged",
    description: "Stage column: the change merged to main.",
  },
  stageBuild: {
    id: "fleet.delivery.stage.build",
    defaultMessage: "Build",
    description: "Stage column: component image builds.",
  },
  stagePolicy: {
    id: "fleet.delivery.stage.policy",
    defaultMessage: "Policy check",
    description: "Stage column: release policy evaluation.",
  },
  stageRelease: {
    id: "fleet.delivery.stage.release",
    defaultMessage: "Release",
    description: "Stage column: release bundle publication.",
  },
  stageGitops: {
    id: "fleet.delivery.stage.gitops",
    defaultMessage: "GitOps update",
    description: "Stage column: the bundle reaching the GitOps main branch.",
  },
  // States (FLT-DLV-04).
  stateNotStarted: {
    id: "fleet.delivery.state.notStarted",
    defaultMessage: "Not started",
    description: "Stage state: an earlier stage has not finished.",
  },
  stateWaiting: {
    id: "fleet.delivery.state.waiting",
    defaultMessage: "Waiting",
    description: "Stage state: waiting on a named condition.",
  },
  stateRunning: {
    id: "fleet.delivery.state.running",
    defaultMessage: "Running",
    description: "Stage state: in progress.",
  },
  statePassed: {
    id: "fleet.delivery.state.passed",
    defaultMessage: "Passed",
    description: "Stage state: finished successfully.",
  },
  stateFailed: {
    id: "fleet.delivery.state.failed",
    defaultMessage: "Failed",
    description: "Stage state: finished unsuccessfully.",
  },
  stateNotNeeded: {
    id: "fleet.delivery.state.notNeeded",
    defaultMessage: "Not needed",
    description: "Stage state: does not apply to this change.",
  },
  stateUnknown: {
    id: "fleet.delivery.state.unknown",
    defaultMessage: "Unknown",
    description: "Stage state: the source is unavailable.",
  },
  stateReleasingInferred: {
    id: "fleet.delivery.state.releasingInferred",
    defaultMessage: "Releasing (inferred)",
    description:
      "Release stage label while the managed release is not observable (FLT-DLV-06).",
  },
  overdue: {
    id: "fleet.delivery.overdue",
    defaultMessage: "Overdue",
    description:
      "Warning on a stage running longer than the slowest of its last 5 runs plus 5 minutes (FLT-DLV-04).",
  },
  // Reasons.
  reasonNoComponentChanged: {
    id: "fleet.delivery.reason.noComponentChanged",
    defaultMessage: "No component changed",
    description: "Build and policy check do not apply (FLT-DLV-05).",
  },
  reasonBuildReplaced: {
    id: "fleet.delivery.reason.buildReplaced",
    defaultMessage: "Replaced by a newer build",
    description: "The build was cancelled because a newer push replaced it.",
  },
  reasonWaitingPolicy: {
    id: "fleet.delivery.reason.waitingPolicy",
    defaultMessage: "Waiting for the policy check to start",
    description: "Policy check has not started yet.",
  },
  reasonOutsideBundle: {
    id: "fleet.delivery.reason.outsideBundle",
    defaultMessage: "Ships outside the bundle",
    description:
      "Every component the change built is not in the release bundle (FLT-DLV-06.06).",
  },
  reasonNextBundle: {
    id: "fleet.delivery.reason.nextBundle",
    defaultMessage: "Rides the next bundle",
    description: "A change with no build ships with the next release bundle.",
  },
  reasonWaitingBundlePR: {
    id: "fleet.delivery.reason.waitingBundlePR",
    defaultMessage: "Waiting for the bundle update pull request",
    description:
      "GitOps update waiting for the automated pull request (FLT-DLV-07).",
  },
  reasonWaitingHydration: {
    id: "fleet.delivery.reason.waitingHydration",
    defaultMessage: "Waiting for hydration",
    description: "Environment group waiting for the hydrator (FLT-DLV-08).",
  },
  reasonWaitingGroups: {
    id: "fleet.delivery.reason.waitingGroups",
    defaultMessage: "Waiting for {groups}",
    description: "Environment group waiting for upstream groups (FLT-DLV-08).",
  },
  reasonGateRunning: {
    id: "fleet.delivery.reason.gateRunning",
    defaultMessage: "{gate} running",
    description:
      "An environment group's gate check is in progress (FLT-DLV-08).",
  },
  reasonGateFailed: {
    id: "fleet.delivery.reason.gateFailed",
    defaultMessage: "{gate} failed",
    description: "An environment group's gate check failed (FLT-DLV-08).",
  },
  gateAnalysis: {
    id: "fleet.delivery.gate.analysis",
    defaultMessage: "Release analysis",
    description: "Name of the release analysis gate.",
  },
  gateHealth: {
    id: "fleet.delivery.gate.health",
    defaultMessage: "Health check",
    description: "Name of the health check gate.",
  },
  partial: {
    id: "fleet.delivery.partial",
    defaultMessage: "{done} of {total}",
    description: "Environments in a group that run the change (FLT-DLV-08).",
  },
  pullRequest: {
    id: "fleet.delivery.pullRequest",
    defaultMessage: "PR #{number}",
    description: "Pull request reference.",
  },
  checksRunning: {
    id: "fleet.delivery.checks.running",
    defaultMessage: "checks running",
    description: "The bundle pull request's checks are running.",
  },
  checksPassed: {
    id: "fleet.delivery.checks.passed",
    defaultMessage: "checks passed",
    description: "The bundle pull request's checks passed.",
  },
  checksFailed: {
    id: "fleet.delivery.checks.failed",
    defaultMessage: "checks failed",
    description: "The bundle pull request's checks failed.",
  },
  // Durations.
  durationLessThanMinute: {
    id: "fleet.delivery.duration.lessThanMinute",
    defaultMessage: "under 1 min",
    description: "A duration under one minute.",
  },
  durationMinutes: {
    id: "fleet.delivery.duration.minutes",
    defaultMessage: "{minutes} min",
    description: "A duration in minutes.",
  },
  durationHours: {
    id: "fleet.delivery.duration.hours",
    defaultMessage: "{hours} h {minutes} min",
    description: "A duration in hours and minutes.",
  },
  durationDays: {
    id: "fleet.delivery.duration.days",
    defaultMessage: "{days} d {hours} h",
    description: "A duration in days and hours.",
  },
  // Now and next (FLT-DLV-10).
  nowNextTitle: {
    id: "fleet.delivery.nowNext.title",
    defaultMessage: "Now and next",
    description: "Heading of the Now and next summary.",
  },
  nowNextList: {
    id: "fleet.delivery.nowNext.list",
    defaultMessage: "Changes in flight",
    description: "Accessible name of the Now and next list.",
  },
  now: {
    id: "fleet.delivery.nowNext.now",
    defaultMessage: "Now",
    description: "Label of the current stage in a Now and next row.",
  },
  next: {
    id: "fleet.delivery.nowNext.next",
    defaultMessage: "Next",
    description: "Label of the following stage in a Now and next row.",
  },
  nowText: {
    id: "fleet.delivery.nowNext.nowText",
    defaultMessage: "{stage}: {state}{detail}{elapsed}{typical}",
    description:
      "Now text: stage, state, optional detail, elapsed and typical time.",
  },
  nowBuilding: {
    id: "fleet.delivery.nowNext.building",
    defaultMessage: "Building {components}{elapsed}{typical}",
    description: "Now text while components build (FLT-DLV-10.01).",
  },
  nowBuildFailed: {
    id: "fleet.delivery.nowNext.buildFailed",
    defaultMessage: "Build failed: {components}{at}",
    description: "Now text for a failed build.",
  },
  usually: {
    id: "fleet.delivery.nowNext.usually",
    defaultMessage: "usually {duration}",
    description: "Typical duration of a stage.",
  },
  nextStageAt: {
    id: "fleet.delivery.nowNext.nextStageAt",
    defaultMessage: "{stage} - usually about {time}",
    description: "Next text with an expected time (FLT-DLV-10).",
  },
  nextRestartBuild: {
    id: "fleet.delivery.nowNext.restartBuild",
    defaultMessage:
      "Re-run the failed pipeline, or merge a change to the same component",
    description:
      "Next text after a failed build or policy check. Builds run only for changes under their component's paths (FLT-DLV-10).",
  },
  nextRestartGate: {
    id: "fleet.delivery.nowNext.restartGate",
    defaultMessage: "A passing re-check or a new change resumes promotion",
    description: "Next text after a failed environment gate.",
  },
  nextNone: {
    id: "fleet.delivery.nowNext.none",
    defaultMessage: "Delivered when this stage passes",
    description: "Next text when the current stage is the last one.",
  },
  moreInTable: {
    id: "fleet.delivery.nowNext.more",
    defaultMessage:
      "{count, plural, one {# more change} other {# more changes}} in the table below",
    description: "Link from the summary to the full table (FLT-DLV-10).",
  },
  allDelivered: {
    id: "fleet.delivery.nowNext.allDelivered",
    defaultMessage: "Everything on main is delivered.",
    description:
      "Summary when every listed change is delivered (FLT-DLV-10.02).",
  },
  allDeliveredDetail: {
    id: "fleet.delivery.nowNext.allDeliveredDetail",
    defaultMessage:
      "The newest change, {change}, reached every environment at {time}, {total} after merge.",
    description: "Detail of the all-delivered summary (FLT-DLV-10.02).",
  },
  allDeliveredNoTime: {
    id: "fleet.delivery.nowNext.allDeliveredNoTime",
    defaultMessage: "The newest change, {change}, runs in every environment.",
    description: "All-delivered detail when the delivery time is unknown.",
  },
  // Table (FLT-DLV-03).
  tableTitle: {
    id: "fleet.delivery.table.title",
    defaultMessage: "Changes on main",
    description: "Heading of the change table.",
  },
  tableLabel: {
    id: "fleet.delivery.table.label",
    defaultMessage: "Changes on main and their delivery stages",
    description: "Accessible name of the change table.",
  },
  columnChange: {
    id: "fleet.delivery.table.change",
    defaultMessage: "Change",
    description: "First column of the change table.",
  },
  byAuthor: {
    id: "fleet.delivery.byAuthor",
    defaultMessage: "by {author}",
    description: "Change author.",
  },
  // Drawer (FLT-DLV-11).
  drawerStages: {
    id: "fleet.delivery.drawer.stages",
    defaultMessage: "Stages",
    description: "Drawer section: every stage.",
  },
  drawerStagesLabel: {
    id: "fleet.delivery.drawer.stagesLabel",
    defaultMessage: "Delivery stages",
    description: "Accessible name of the drawer stage list.",
  },
  stepRange: {
    id: "fleet.delivery.drawer.stepRange",
    defaultMessage: "{start} to {end} - {duration}",
    description: "A finished stage's start, end, and duration.",
  },
  drawerChecks: {
    id: "fleet.delivery.drawer.checks",
    defaultMessage: "Builds and policy checks",
    description: "Drawer section: per-component checks.",
  },
  checkLine: {
    id: "fleet.delivery.drawer.checkLine",
    defaultMessage: "Build: {build}. Policy check: {policy}.",
    description: "One component's build and policy check states.",
  },
  viewCheckRuns: {
    id: "fleet.delivery.drawer.viewCheckRuns",
    defaultMessage: "View check runs",
    description: "Link to a component's GitHub check runs.",
  },
  noChecks: {
    id: "fleet.delivery.drawer.noChecks",
    defaultMessage: "No component was built for this change.",
    description: "Drawer text when there are no builds.",
  },
  drawerBundle: {
    id: "fleet.delivery.drawer.bundle",
    defaultMessage: "Bundle",
    description: "Drawer section: the first bundle.",
  },
  bundleUnknown: {
    id: "fleet.delivery.drawer.bundleUnknown",
    defaultMessage: "Unknown until the bundle sources can be read.",
    description:
      "Drawer text when the Release stage is Unknown, so the bundle holding the change cannot be known.",
  },
  bundleNone: {
    id: "fleet.delivery.drawer.bundleNone",
    defaultMessage: "No bundle contains this change yet.",
    description: "Drawer text before a bundle contains the change.",
  },
  bundleAlias: {
    id: "fleet.delivery.drawer.alias",
    defaultMessage: "Version",
    description: "Bundle version term.",
  },
  bundleTag: {
    id: "fleet.delivery.drawer.tag",
    defaultMessage: "Tag",
    description: "Bundle tag term.",
  },
  bundleDigest: {
    id: "fleet.delivery.drawer.digest",
    defaultMessage: "Digest",
    description: "Bundle digest term.",
  },
  bundlePublished: {
    id: "fleet.delivery.drawer.published",
    defaultMessage: "Published",
    description: "Bundle publication term.",
  },
  bundleLocked: {
    id: "fleet.delivery.drawer.locked",
    defaultMessage: "In the GitOps release lock",
    description: "The bundle is locked.",
  },
  bundlePending: {
    id: "fleet.delivery.drawer.pending",
    defaultMessage: "Not in the GitOps release lock yet",
    description: "The bundle is pending.",
  },
  downloads: {
    id: "fleet.delivery.drawer.downloads",
    defaultMessage: "Downloads",
    description: "Link to the bundle's GitHub release.",
  },
  downloadsPending: {
    id: "fleet.delivery.drawer.downloadsPending",
    defaultMessage: "Downloads not published yet",
    description:
      "The bundle's GitHub release is a draft or missing (FLT-DLV-06).",
  },
  gitopsUpdate: {
    id: "fleet.delivery.drawer.gitopsUpdate",
    defaultMessage: "GitOps update",
    description: "Term for the GitOps update pull request.",
  },
  drawerEnvironments: {
    id: "fleet.delivery.drawer.environments",
    defaultMessage: "Environments",
    description: "Drawer section: environments per group.",
  },
  openNode: {
    id: "fleet.delivery.drawer.openNode",
    defaultMessage: "Open {env} on Fleet Map",
    description: "Accessible name of an environment link.",
  },
  showOnMap: {
    id: "fleet.delivery.drawer.showOnMap",
    defaultMessage: "Show bundle on Fleet Map",
    description:
      "Opens the Fleet Map with the first bundle selected (FLT-DLV-11.01).",
  },
  showInSdlc: {
    id: "fleet.delivery.drawer.showInSdlc",
    defaultMessage: "Show bundle in Agent SDLC",
    description: "Opens the bundle's Shipped row (FLT-DLV-11.02).",
  },
  viewCommit: {
    id: "fleet.delivery.drawer.viewCommit",
    defaultMessage: "View commit",
    description: "Link to the change's commit.",
  },
  viewPR: {
    id: "fleet.delivery.drawer.viewPR",
    defaultMessage: "View pull request",
    description: "Link to the change's pull request.",
  },
  lookCommitWithPr: {
    id: "fleet.delivery.look.commitWithPr",
    defaultMessage: "{sha} (#{pr})",
    description:
      "A publisher-path commit with its pull request number (FLT-DLV-20).",
  },
  nowOverdue: {
    id: "fleet.delivery.nowNext.overdue",
    defaultMessage: "Overdue",
    description:
      "Added after the Now text when the stage is Overdue (FLT-DLV-04).",
  },
  nextOverdue: {
    id: "fleet.delivery.nowNext.nextOverdue",
    defaultMessage: "{stage} - no arrival time while a stage is overdue",
    description: "Next text while a stage is Overdue (FLT-DLV-10.06).",
  },
  progressTitle: {
    id: "fleet.delivery.progress.title",
    defaultMessage: "Progress",
    description:
      "Progress bar heading when no percent can be told (FLT-DLV-19).",
  },
  progressPercent: {
    id: "fleet.delivery.progress.percent",
    defaultMessage: "{percent}% complete",
    description: "Percent complete of a change's delivery (FLT-DLV-19).",
  },
  progressLabel: {
    id: "fleet.delivery.progress.label",
    defaultMessage: "Delivery progress of {change}",
    description: "Accessible name of a change's progress bar (FLT-DLV-19).",
  },
  progressLabelPercent: {
    id: "fleet.delivery.progress.labelPercent",
    defaultMessage: "Delivery progress of {change}: {percent}% complete",
    description:
      "Accessible name of a change's progress bar with its percent (FLT-DLV-19).",
  },
  arrivalRange: {
    id: "fleet.delivery.arrival.range",
    defaultMessage: "{early} to {late}",
    description:
      "Expected arrival range before the bundle is published (FLT-DLV-10).",
  },
  arrivalTime: {
    id: "fleet.delivery.arrival.time",
    defaultMessage: "about {time}",
    description: "Expected arrival after the bundle is published (FLT-DLV-10).",
  },
  whereToLook: {
    id: "fleet.delivery.look.trigger",
    defaultMessage: "Where to look",
    description: "Opens hints for an Overdue or Failed stage (FLT-DLV-20).",
  },
  whereToLookTitle: {
    id: "fleet.delivery.look.title",
    defaultMessage: "Where to look: {stage}",
    description: "Heading of the Where to look popup (FLT-DLV-20).",
  },
  lookBuildFailed: {
    id: "fleet.delivery.look.buildFailed",
    defaultMessage:
      "Re-run the failed pipeline, or merge a change to the same component.",
    description: "What restarts a failed build (FLT-DLV-20.02).",
  },
  lookBuildWhy: {
    id: "fleet.delivery.look.buildWhy",
    defaultMessage:
      "A component builds only for changes under its own paths, so an unrelated change does not rebuild it.",
    description: "Why a later unrelated change does not fix a failed build.",
  },
  lookChecksRunning: {
    id: "fleet.delivery.look.checksRunning",
    defaultMessage:
      "These runs have taken longer than any of their last 5 runs:",
    description: "Lead-in for Overdue check runs (FLT-DLV-20).",
  },
  lookViewCheckRun: {
    id: "fleet.delivery.look.viewCheckRun",
    defaultMessage: "View check run",
    description: "Link to a check run.",
  },
  lookImageCheck: {
    id: "fleet.delivery.look.imageCheck",
    defaultMessage: "Image check",
    description:
      "Term for the check run that confirms the Konflux images are published (FLT-DLV-20).",
  },
  lookViewImageCheck: {
    id: "fleet.delivery.look.viewImageCheck",
    defaultMessage: "View image check",
    description: "Link to the image check run (FLT-DLV-20).",
  },
  lookReleaseSummary: {
    id: "fleet.delivery.look.releaseSummary",
    defaultMessage:
      "The release has run longer than any of the last 5 releases.",
    description: "Lead-in of the Release popup (FLT-DLV-20).",
  },
  lookNotLoaded: {
    id: "fleet.delivery.look.notLoaded",
    defaultMessage: "Release details load on the next refresh.",
    description: "Release diagnostics are not in the payload yet.",
  },
  lookLastGood: {
    id: "fleet.delivery.look.lastGood",
    defaultMessage: "Last good release",
    description: "The newest published bundle.",
  },
  lookLastGoodValue: {
    id: "fleet.delivery.look.lastGoodValue",
    defaultMessage: "{bundle}, published {time}",
    description: "The newest published bundle and its time.",
  },
  lookKonflux: {
    id: "fleet.delivery.look.konflux",
    defaultMessage: "Konflux",
    description: "Term for the Konflux release log link.",
  },
  lookOpenLog: {
    id: "fleet.delivery.look.openLog",
    defaultMessage: "Open the release log",
    description: "Link to the Konflux release log.",
  },
  lookPublisherChanges: {
    id: "fleet.delivery.look.publisherChanges",
    defaultMessage: "Publisher changes",
    description:
      "Commits that touched the bundle publisher since the last good release.",
  },
  lookNoPublisherChanges: {
    id: "fleet.delivery.look.noPublisherChanges",
    defaultMessage: "None since the last good release",
    description: "No commit touched the bundle publisher.",
  },
  lookReadFailed: {
    id: "fleet.delivery.look.readFailed",
    defaultMessage: "Could not be read",
    description: "A release diagnostic read failed.",
  },
  lookReleaseApp: {
    id: "fleet.delivery.look.releaseApp",
    defaultMessage: "Release app",
    description: "Term for the release GitHub App link.",
  },
  lookReleaseAppText: {
    id: "fleet.delivery.look.releaseAppText",
    defaultMessage: "Pending permission requests block publication",
    description:
      "Why to check the release app; the dashboard cannot read pending permissions.",
  },
  lookCheckApp: {
    id: "fleet.delivery.look.checkApp",
    defaultMessage: "Check the app's permissions",
    description: "Link to the release GitHub App's settings.",
  },
  lookRegistryTag: {
    id: "fleet.delivery.look.registryTag",
    defaultMessage: "Registry tag",
    description: "Newest bundle tag in the registry.",
  },
  lookGithubRelease: {
    id: "fleet.delivery.look.githubRelease",
    defaultMessage: "GitHub release",
    description: "Newest published GitHub release tag.",
  },
  lookTagsDiffer: {
    id: "fleet.delivery.look.tagsDiffer",
    defaultMessage: "Differs from the registry",
    description: "The registry and GitHub release tags differ.",
  },
  lookBundlePR: {
    id: "fleet.delivery.look.bundlePR",
    defaultMessage: "Bundle update pull request",
    description: "Term for the GitOps update pull request.",
  },
  lookNoBundlePR: {
    id: "fleet.delivery.look.noBundlePR",
    defaultMessage: "No bundle update pull request yet",
    description: "The GitOps update pull request does not exist yet.",
  },
  lookAnalysis: {
    id: "fleet.delivery.look.analysis",
    defaultMessage: "Analysis",
    description: "Link to an environment's analysis check run.",
  },
  lookArgo: {
    id: "fleet.delivery.look.argo",
    defaultMessage: "Argo CD",
    description: "Link to an environment's Argo CD application.",
  },
  outsideBundle: {
    id: "fleet.delivery.drawer.outsideBundle",
    defaultMessage: "Ships outside the bundle",
    description:
      "A component that no release bundle lists (FLT-DLV-05, FLT-DLV-06).",
  },
});
