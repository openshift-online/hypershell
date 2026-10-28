// The E2E run view (e2e-run-log-visibility.spec E2E-LOG-12): the recent release
// end-to-end test runs, newest-first, with each run's ordered steps and a selectable
// inline log pane. New runs, steps, and log lines arrive live over the per-run
// stream; the framework-free domain (../../../domain/e2e-run) does all event
// reduction (ordering, dedup, step-state derivation), so this component only renders.
//
// Every status signal pairs an icon + text with any colour so colour is never the
// sole channel (E2E-LOG-14). The four degraded states are kept distinct (E2E-LOG-10):
// test-failed, reporting-incomplete, backend-unavailable, and live-disconnected. Log
// content is rendered as text (React escapes it) so a record cannot inject markup.
//
// FIREWALL: no fleet identity here. Cluster, tenant, revision, run ids, and step ids
// are all runtime data from /api/e2e/*.

import type { PromotionData, ReleaseBundle } from "../../../domain/promotion";
import { Identicon } from "../map/identicon";
import { identiName } from "../../../domain/map/identiname";
import {
  Drawer,
  DrawerContent,
  DrawerContentBody,
  DrawerPanelContent,
  DrawerPanelBody,
  DrawerHead,
  DrawerActions,
  DrawerCloseButton,
  Title,
  Alert,
  AlertActionLink,
  Button,
  Content,
  Label,
  Stack,
  StackItem,
  Tabs,
  Tab,
  TabTitleText,
  FormSelect,
  FormSelectOption,
  SearchInput,
  Checkbox,
  Modal,
  ModalBody,
  ModalHeader,
  ModalVariant,
  Spinner,
} from "@patternfly/react-core";
import {
  type ThProps,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Tr,
} from "@patternfly/react-table";
import BanIcon from "@patternfly/react-icons/dist/esm/icons/ban-icon";
import CheckCircleIcon from "@patternfly/react-icons/dist/esm/icons/check-circle-icon";
import ExclamationCircleIcon from "@patternfly/react-icons/dist/esm/icons/exclamation-circle-icon";
import ExclamationTriangleIcon from "@patternfly/react-icons/dist/esm/icons/exclamation-triangle-icon";
import InProgressIcon from "@patternfly/react-icons/dist/esm/icons/in-progress-icon";
import OutlinedCircleIcon from "@patternfly/react-icons/dist/esm/icons/outlined-circle-icon";
import PauseIcon from "@patternfly/react-icons/dist/esm/icons/pause-icon";
import PlayIcon from "@patternfly/react-icons/dist/esm/icons/play-icon";
import QuestionCircleIcon from "@patternfly/react-icons/dist/esm/icons/question-circle-icon";
import type { SVGIconProps } from "@patternfly/react-icons/dist/esm/createIcon";
import type { ComponentType } from "react";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { IntlShape, MessageDescriptor } from "react-intl";
import { FormattedMessage, useIntl } from "react-intl";

import {
  applyEvents,
  boundLogBuffer,
  elapsedSeconds,
  isRunning,
  logsForStep,
  mergeLogLines,
  reduceRun,
  runDurationMs,
  runBundle,
  runTargetInstance,
  runsNewestFirst,
  MAX_DETAIL_LOG_LINES,
  MAX_LOG_BUFFER,
  type E2eLogLine,
  type E2eRunDetail,
  type E2eRunEvent,
  type E2eRunsData,
  type E2eRunSummary,
  type E2eStepState,
  type E2eStreamStatus,
  type E2eVerdict,
} from "../../../domain/e2e-run";
import {
  e2eStreamFact,
  type ObservabilityPort,
} from "../../../application/observability";
import type { ExecState, SortState } from "../../../domain/navigation/route";
import { messages } from "../../../messages";
import { useFleetApi } from "../../query/api-context";
import { useObservability } from "../../observability/observability-context";
import { useRoute, useRouter } from "../../routing/router";
import { NavLinkButton } from "../nav-link-button";
import { TONE_COLOR } from "../map/colors";
import styles from "./e2e-run-view.module.css";

/**
 * The shared execution-detail controls (FLT-NAV-11): one bundle of route-derived
 * state plus typed dispatchers, wired by the E2E run container to route.e2e.exec and
 * by the Fleet Map gate drawer to route.map.exec. The presentational pane and its
 * tables stay router-agnostic and fully controlled, so a single stream per run is
 * preserved across sub-tab, step, sort, and log-search changes (FLT-NAV-18).
 */
export interface ExecControls {
  readonly exec: ExecState;
  readonly onTab: (tab: ExecState["tab"]) => void;
  readonly onStep: (step: string | null) => void;
  readonly onStepSort: (sort: SortState | null) => void;
  readonly onLogExpanded: (value: boolean) => void;
  readonly onLogQuery: (value: string) => void;
}

/**
 * How many `log` events the live buffer retains. Older log events fall outside the
 * domain's per-run log window (MAX_DETAIL_LOG_LINES) and the rendered window, so the
 * buffer need not keep them; structural (run/step) events are always kept so the
 * reduction stays correct. This bounds the buffer and keeps the per-event fold cost
 * constant instead of re-folding an ever-growing array (the old O(n squared) path).
 */
const MAX_BUFFERED_LOG_EVENTS = MAX_DETAIL_LOG_LINES;

/**
 * Append one streamed event to the live buffer, deduplicated by sequence (a
 * reconnect backfill re-delivers overlap), keeping every structural event but
 * capping retained `log` events to the most recent window. Returns the previous
 * reference unchanged when the event is a duplicate, so React skips a re-render.
 */
function appendBoundedEvents(
  prev: readonly E2eRunEvent[],
  event: E2eRunEvent,
): readonly E2eRunEvent[] {
  for (const e of prev) {
    if (e.sequence === event.sequence) {
      return prev;
    }
  }
  const next = [...prev, event];
  let logCount = 0;
  for (const e of next) {
    if (e.event === "log") {
      logCount++;
    }
  }
  if (logCount <= MAX_BUFFERED_LOG_EVENTS) {
    return next;
  }
  // Drop the oldest log events (earliest in arrival order) until within the window;
  // structural events are never dropped. The domain fold re-sorts by sequence, so
  // the retained newest log events still render in order.
  let toDrop = logCount - MAX_BUFFERED_LOG_EVENTS;
  const trimmed: E2eRunEvent[] = [];
  for (const e of next) {
    if (toDrop > 0 && e.event === "log") {
      toDrop--;
      continue;
    }
    trimmed.push(e);
  }
  return trimmed;
}

/**
 * Resolve the reference instant. Kept in a plain helper (not the component body) so
 * the fallback clock read stays out of render purity analysis, matching the map's
 * releaseTimeParts pattern; tests inject a fixed nowMs.
 */
function resolveNowMs(nowMs: number | undefined): number {
  return nowMs ?? Date.now();
}

export interface E2eRunViewProps {
  readonly promotion?: PromotionData;
  readonly data: E2eRunsData;
  /** Reference instant for relative timestamps; injectable for deterministic tests. */
  readonly nowMs?: number;
}

type IconComp = ComponentType<SVGIconProps>;

interface Visual {
  readonly icon: IconComp;
  readonly tone: string;
  readonly message: MessageDescriptor;
}

function verdictVisual(verdict: E2eVerdict): Visual {
  switch (verdict) {
    case "passed":
      return {
        icon: CheckCircleIcon,
        tone: TONE_COLOR.success,
        message: messages.e2eVerdictPassed,
      };
    case "failed":
      return {
        icon: ExclamationCircleIcon,
        tone: TONE_COLOR.danger,
        message: messages.e2eVerdictFailed,
      };
    default:
      return {
        icon: QuestionCircleIcon,
        tone: TONE_COLOR.unknown,
        message: messages.e2eVerdictUnknown,
      };
  }
}

function stepStateVisual(state: E2eStepState): Visual {
  switch (state) {
    case "passed":
      return {
        icon: CheckCircleIcon,
        tone: TONE_COLOR.success,
        message: messages.e2eStatePassed,
      };
    case "failed":
      return {
        icon: ExclamationCircleIcon,
        tone: TONE_COLOR.danger,
        message: messages.e2eStateFailed,
      };
    case "skipped":
      return {
        icon: BanIcon,
        tone: TONE_COLOR.unknown,
        message: messages.e2eStateSkipped,
      };
    case "running":
      return {
        icon: InProgressIcon,
        tone: TONE_COLOR.info,
        message: messages.e2eStateRunning,
      };
    default:
      return {
        icon: OutlinedCircleIcon,
        tone: TONE_COLOR.unknown,
        message: messages.e2eStateNotRun,
      };
  }
}

/** A status chip: icon + localized text, with colour as accent only (E2E-LOG-14). */
function StatusChip({ visual }: { visual: Visual }): React.ReactElement {
  const Icon = visual.icon;
  return (
    <span className={styles.inline}>
      <Icon style={{ color: visual.tone }} aria-hidden />
      <FormattedMessage {...visual.message} />
    </span>
  );
}

function formatDuration(intl: IntlShape, ms: number | null): string {
  if (ms === null) {
    return intl.formatMessage(messages.valueNone);
  }
  const totalSeconds = Math.floor(ms / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return intl.formatMessage(messages.e2eDuration, { minutes, seconds });
}

function formatAgo(intl: IntlShape, iso: string, nowMs: number): string {
  const secs = elapsedSeconds(iso, nowMs);
  if (secs === null) {
    return intl.formatMessage(messages.valueNone);
  }
  if (secs < 60) {
    return intl.formatMessage(messages.e2eAgo, { unit: "now", count: 0 });
  }
  if (secs < 3600) {
    return intl.formatMessage(messages.e2eAgo, {
      unit: "m",
      count: Math.floor(secs / 60),
    });
  }
  if (secs < 86_400) {
    return intl.formatMessage(messages.e2eAgo, {
      unit: "h",
      count: Math.floor(secs / 3600),
    });
  }
  return intl.formatMessage(messages.e2eAgo, {
    unit: "d",
    count: Math.floor(secs / 86_400),
  });
}

/**
 * Subscribe to a run's live stream and fold incoming events onto the snapshot detail
 * in the framework-free domain (E2E-LOG-06/11). Returns the merged detail and the
 * live connection status; publishes connect/disconnect/reconnect/dropped-entry facts
 * through the observability port (E2E-LOG-13).
 */
function useLiveRunDetail(
  id: string,
  snapshot: E2eRunDetail | null,
  live: boolean,
  onLog: (line: E2eLogLine) => void,
  observability: ObservabilityPort,
): { detail: E2eRunDetail | null; status: E2eStreamStatus | null } {
  const api = useFleetApi();
  const [events, setEvents] = useState<readonly E2eRunEvent[]>([]);
  const [status, setStatus] = useState<E2eStreamStatus | null>(null);

  useEffect(() => {
    // The live tail is opened ONLY for an in-progress run (E2E-LOG-21): a completed
    // run is read by pagination alone, because a Loki tail replays no history and
    // would render nothing. `live` is derived from the loaded snapshot, so the
    // subscription starts once the run is known to be in progress.
    if (!live) {
      return;
    }
    // This hook lives inside a run-keyed pane, so `id` is stable for the pane's life.
    let prevState: E2eStreamStatus["state"] | null = null;
    observability.publishE2eStream(e2eStreamFact("connect"));
    const unsubscribe = api.streamE2eRun(id, {
      onEvent: (event) => {
        setEvents((prev) => appendBoundedEvents(prev, event));
      },
      onLog: (line) => {
        // Live log lines ride their own frame and append into the same rendered log
        // list the pane paginates, de-duplicated by sequence at the history-to-live
        // seam (E2E-LOG-21).
        onLog(line);
      },
      onStatus: (next) => {
        if (next.state === "disconnected" && prevState !== "disconnected") {
          observability.publishE2eStream(e2eStreamFact("disconnect"));
        } else if (next.state === "live" && prevState === "disconnected") {
          observability.publishE2eStream(e2eStreamFact("reconnect"));
        } else if (next.state === "gap" && next.droppedEntries > 0) {
          observability.publishE2eStream(
            e2eStreamFact("dropped-entry", next.droppedEntries),
          );
        }
        prevState = next.state;
        setStatus(next);
      },
    });
    return () => {
      unsubscribe();
    };
  }, [id, api, live, onLog, observability]);

  const detail = useMemo<E2eRunDetail | null>(() => {
    if (snapshot !== null) {
      return applyEvents(snapshot, events);
    }
    if (events.length > 0) {
      return reduceRun(events);
    }
    return null;
  }, [snapshot, events]);

  return { detail, status };
}

/**
 * Manage one run's paginated raw-log history (E2E-LOG-19) and the live lines that
 * append to it (E2E-LOG-21), scoped to the pair `(run id, selected step)`
 * (E2E-LOG-23). When a step is selected the browser requests that step's logs
 * directly (the backend filters in Loki before the page limit, E2E-LOG-24), rather
 * than keeping a run-wide buffer and filtering at render time. On mount, and on any
 * scope change, it loads the newest page for the scope, aborts in-flight requests
 * from the previous scope, and ignores their late results so one step's history can
 * never populate another's. loadOlder() fetches the next older page by the
 * oldest-timestamp cursor; appendLive() merges a live line that belongs to the
 * scope. The buffer is ordered and de-duplicated by sequence and bounded to
 * MAX_LOG_BUFFER so the heap stays bounded on a very long run (E2E-LOG-22).
 *
 * A timed-out, rejected, or stale request sets `failed`: the already-loaded lines
 * are preserved, observer-driven fetching stops, and the pane exposes an explicit
 * Retry action (retry()) rather than re-firing against a still-visible sentinel
 * (E2E-LOG-26). `locallyLimited` distinguishes the in-memory buffer ceiling from the
 * true end of history.
 */
interface RunLogs {
  readonly lines: readonly E2eLogLine[];
  readonly hasMoreOlder: boolean;
  readonly loadingOlder: boolean;
  readonly loadingInitial: boolean;
  readonly failed: boolean;
  readonly locallyLimited: boolean;
  readonly loadOlder: () => void;
  readonly retry: () => void;
  readonly appendLive: (line: E2eLogLine) => void;
}

function useRunLogs(
  id: string,
  selectedStepId: string | null,
  inertWhenRunWide = false,
): RunLogs {
  const api = useFleetApi();
  // A step-scoped instance stays idle while no step is selected, so the run-wide
  // instance (which drives the independent progress summary and live tail) is the
  // single run-wide reader - no duplicate run-wide fetch (E2E-LOG-23).
  const inert = inertWhenRunWide && selectedStepId === null;
  const [lines, setLines] = useState<readonly E2eLogLine[]>([]);
  const [hasMoreOlder, setHasMoreOlder] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [loadingInitial, setLoadingInitial] = useState(true);
  const [failed, setFailed] = useState(false);
  const [locallyLimited, setLocallyLimited] = useState(false);
  // An explicit reload counter so retry() can re-run the scope load effect.
  const [reloadNonce, setReloadNonce] = useState(0);

  // Refs the loaders read without re-creating themselves on every line change, and
  // to make scope-change guards synchronous (a late result for a previous scope is
  // ignored, E2E-LOG-23).
  const oldestCursor = useRef<string | null>(null);
  const linesRef = useRef<readonly E2eLogLine[]>([]);
  const inFlight = useRef(false);
  const failedRef = useRef(false);
  const hasMoreRef = useRef(false);
  const scopeRef = useRef<string | null>(selectedStepId);
  const inertRef = useRef(inert);
  const controllerRef = useRef<AbortController | null>(null);
  useEffect(() => {
    linesRef.current = lines;
  }, [lines]);
  useEffect(() => {
    inertRef.current = inert;
  }, [inert]);

  // Load the newest page for the current scope. Re-runs on a run or step change
  // (E2E-LOG-23) and on an explicit retry of a failed initial load (E2E-LOG-26),
  // resetting the scoped buffer, cursor, and error state each time.
  useEffect(() => {
    scopeRef.current = selectedStepId;
    inFlight.current = false;
    oldestCursor.current = null;
    linesRef.current = [];
    failedRef.current = false;
    hasMoreRef.current = false;
    setLines([]);
    setHasMoreOlder(false);
    setLoadingOlder(false);
    setLocallyLimited(false);
    setFailed(false);
    if (inert) {
      // Idle: no scope is selected for this instance, so it holds no buffer and does
      // not fetch. It settles out of the loading state immediately.
      setLoadingInitial(false);
      return;
    }
    setLoadingInitial(true);
    const controller = new AbortController();
    controllerRef.current = controller;
    api
      .getE2eRunLogs(id, null, selectedStepId, controller.signal)
      .then((plane) => {
        if (controller.signal.aborted) {
          return;
        }
        if (plane.stale) {
          failedRef.current = true;
          setFailed(true);
          setLoadingInitial(false);
          return;
        }
        const next = mergeLogLines([], plane.data.entries);
        linesRef.current = next;
        setLines(next);
        oldestCursor.current = plane.data.nextCursor;
        const atCeiling = next.length >= MAX_LOG_BUFFER;
        const more = plane.data.hasMore && !atCeiling;
        hasMoreRef.current = more;
        setHasMoreOlder(more);
        setLocallyLimited(plane.data.hasMore && atCeiling);
        setLoadingInitial(false);
      })
      .catch(() => {
        if (controller.signal.aborted) {
          return;
        }
        failedRef.current = true;
        setFailed(true);
        setLoadingInitial(false);
      });
    return () => {
      controller.abort();
    };
  }, [api, id, selectedStepId, inert, reloadNonce]);

  const loadOlder = useCallback(() => {
    // Stop automatic fetching while a failure is pending, so an empty pane that keeps
    // the sentinel visible cannot busy-loop the same request (E2E-LOG-26).
    if (
      inFlight.current ||
      failedRef.current ||
      !hasMoreRef.current ||
      oldestCursor.current === null
    ) {
      return;
    }
    inFlight.current = true;
    const controller = controllerRef.current;
    const scopeAtRequest = scopeRef.current;
    const requested = oldestCursor.current;
    setLoadingOlder(true);
    api
      .getE2eRunLogs(id, requested, scopeAtRequest, controller?.signal)
      .then((plane) => {
        // Ignore a late result once the scope has changed (E2E-LOG-23).
        if (controller?.signal.aborted || scopeRef.current !== scopeAtRequest) {
          return;
        }
        if (plane.stale) {
          // A stale/unavailable older page is actionable, not silently stopped
          // (E2E-LOG-26.04): surface the failed state so Retry can recover.
          failedRef.current = true;
          setFailed(true);
          return;
        }
        const merged = mergeLogLines(linesRef.current, plane.data.entries);
        linesRef.current = merged;
        setLines(merged);
        oldestCursor.current = plane.data.nextCursor;
        // Stop paging when the cursor did not advance (a defensive guard against a
        // nanosecond shared by more than one page). Distinguish the in-memory buffer
        // ceiling (locally limited) from the true end of history (E2E-LOG-26).
        const cursorAdvanced = plane.data.nextCursor !== requested;
        const atCeiling = merged.length >= MAX_LOG_BUFFER;
        const more = plane.data.hasMore && cursorAdvanced && !atCeiling;
        hasMoreRef.current = more;
        setHasMoreOlder(more);
        setLocallyLimited(plane.data.hasMore && cursorAdvanced && atCeiling);
      })
      .catch(() => {
        if (controller?.signal.aborted || scopeRef.current !== scopeAtRequest) {
          return;
        }
        // A timeout or rejection preserves the loaded lines and stops auto-fetch
        // behind an explicit, recoverable error (E2E-LOG-26.01).
        failedRef.current = true;
        setFailed(true);
      })
      .finally(() => {
        if (scopeRef.current === scopeAtRequest) {
          inFlight.current = false;
          setLoadingOlder(false);
        }
      });
  }, [api, id]);

  const retry = useCallback(() => {
    if (!failedRef.current) {
      return;
    }
    failedRef.current = false;
    setFailed(false);
    if (linesRef.current.length === 0 || oldestCursor.current === null) {
      // The initial load failed: reload the newest page for the scope.
      setReloadNonce((n) => n + 1);
    } else {
      // An older-page load failed: resume paging from the preserved cursor.
      inFlight.current = false;
      setLoadingOlder(false);
      loadOlder();
    }
  }, [loadOlder]);

  // Stable across scope changes (reads scope/inert via refs) so fanning it into the
  // live tail does not re-subscribe the stream when the log scope toggles (E2E-LOG-21).
  const appendLive = useCallback((line: E2eLogLine) => {
    // An idle step-scoped instance holds no buffer; the run-wide instance takes the
    // line instead.
    if (inertRef.current) {
      return;
    }
    // Merge a live line only when it belongs to the active scope, so a run-wide live
    // line cannot leak into a selected step's buffer (E2E-LOG-23).
    const scope = scopeRef.current;
    if (scope !== null && line.stepId !== scope) {
      return;
    }
    setLines((prev) =>
      boundLogBuffer(mergeLogLines(prev, [line]), MAX_LOG_BUFFER),
    );
  }, []);

  return {
    lines,
    hasMoreOlder,
    loadingOlder,
    loadingInitial,
    failed,
    locallyLimited,
    loadOlder,
    retry,
    appendLive,
  };
}

// Sort source values, never localized display strings. Null durations sort last.
// Controlled by route state: the current sort and its dispatcher come from the
// caller so sort selection round-trips through the URL (FLT-NAV-12, FLT-NAV-11).
function useTableSort<T>(
  rows: readonly T[],
  values: (row: T) => readonly (string | number | null)[],
  sort: SortState | null,
  onSort: (sort: SortState | null) => void,
  initialIndex?: number,
) {
  const index = sort?.col ?? initialIndex;
  const direction = sort?.dir ?? "desc";
  const sortBy = { index, direction };
  const sorted =
    index === undefined
      ? rows
      : [...rows].sort((a, b) => {
          const av = values(a)[index] ?? null;
          const bv = values(b)[index] ?? null;
          if (av === null) return bv === null ? 0 : 1;
          if (bv === null) return -1;
          const comparison =
            typeof av === "number" && typeof bv === "number"
              ? av - bv
              : String(av).localeCompare(String(bv));
          return direction === "asc" ? comparison : -comparison;
        });
  const column = (columnIndex: number): ThProps["sort"] => ({
    sortBy,
    columnIndex,
    onSort: (_, ci, dir) => {
      onSort({ col: ci, dir });
    },
  });
  return { sorted, column };
}

function RunBundleIdentity({
  bundle,
}: {
  bundle: ReleaseBundle | null;
}): React.ReactElement {
  return bundle?.digest ? (
    <span className={styles.bundleIdentity} title={bundle.digest}>
      <svg width={28} height={28} aria-hidden="true">
        <Identicon seed={bundle.digest} x={0} y={0} size={28} />
      </svg>
      <span>
        {identiName(bundle.digest)}
        <small className={styles.instance}>{bundle.version}</small>
      </span>
    </span>
  ) : (
    <span>
      <FormattedMessage {...messages.e2eBundleUnknown} />
    </span>
  );
}

function RunsTable({
  runs,
  promotion,
  selectedId,
  onActivate,
  sort,
  onSort,
  nowMs,
}: {
  promotion?: PromotionData;
  runs: readonly E2eRunSummary[];
  selectedId: string | null;
  /** Records the trigger element for focus restoration; navigation is the link's. */
  onActivate: (event: React.MouseEvent) => void;
  sort: SortState | null;
  onSort: (sort: SortState | null) => void;
  nowMs: number;
}): React.ReactElement {
  const intl = useIntl();
  const { sorted, column } = useTableSort(
    runs,
    (run) => [
      run.revision,
      run.runId,
      Date.parse(run.startedAt),
      runDurationMs(run, nowMs),
      run.verdict,
      runBundle(run, promotion)?.version ?? null,
      runTargetInstance(run, promotion) ?? run.cluster,
    ],
    sort,
    onSort,
    2,
  );
  return (
    <Table
      gridBreakPoint=""
      aria-label={intl.formatMessage(messages.e2eRunsAria)}
      variant="compact"
    >
      <Thead>
        <Tr>
          <Th sort={column(5)}>
            <FormattedMessage {...messages.e2eColumnBundle} />
          </Th>
          <Th sort={column(6)}>
            <FormattedMessage {...messages.e2eColumnTarget} />
          </Th>
          <Th sort={column(0)}>
            <FormattedMessage {...messages.e2eColumnRevision} />
          </Th>
          <Th sort={column(1)}>
            <FormattedMessage {...messages.e2eColumnAttempt} />
          </Th>
          <Th sort={column(2)}>
            <FormattedMessage {...messages.e2eColumnStarted} />
          </Th>
          <Th sort={column(3)}>
            <FormattedMessage {...messages.e2eColumnDuration} />
          </Th>
          <Th sort={column(4)}>
            <FormattedMessage {...messages.e2eColumnVerdict} />
          </Th>
        </Tr>
      </Thead>
      <Tbody>
        {sorted.map((run) => {
          const selected = run.id === selectedId;
          return (
            <Tr
              key={run.id}
              isRowSelected={selected}
              className={selected ? styles.selectedRow : undefined}
            >
              <Td dataLabel={intl.formatMessage(messages.e2eColumnBundle)}>
                <RunBundleIdentity bundle={runBundle(run, promotion)} />
              </Td>
              <Td dataLabel={intl.formatMessage(messages.e2eColumnTarget)}>
                <span>{runTargetInstance(run, promotion) ?? run.cluster}</span>
                <small className={styles.instance}>
                  {runTargetInstance(run, promotion)
                    ? intl.formatMessage(messages.e2eTargetLocation, {
                        cluster: run.cluster,
                        namespace: run.namespace,
                      })
                    : run.namespace}
                </small>
              </Td>
              <Td dataLabel={intl.formatMessage(messages.e2eColumnRevision)}>
                <NavLinkButton
                  action={{ type: "e2eSelectRun", run: run.id }}
                  onActivate={onActivate}
                  aria-label={intl.formatMessage(messages.e2eRunSelect, {
                    revision: run.revision,
                    attempt: run.runId,
                  })}
                >
                  {run.revision !== ""
                    ? run.revision.slice(0, 8)
                    : intl.formatMessage(messages.valueNone)}
                </NavLinkButton>
              </Td>
              <Td dataLabel={intl.formatMessage(messages.e2eColumnAttempt)}>
                <span title={run.runId}>{run.runId.slice(0, 8)}</span>
              </Td>
              <Td dataLabel={intl.formatMessage(messages.e2eColumnStarted)}>
                {formatAgo(intl, run.startedAt, nowMs)}
              </Td>
              <Td dataLabel={intl.formatMessage(messages.e2eColumnDuration)}>
                {formatDuration(intl, runDurationMs(run, nowMs))}
              </Td>
              <Td dataLabel={intl.formatMessage(messages.e2eColumnVerdict)}>
                <span className={styles.inline}>
                  <StatusChip visual={verdictVisual(run.verdict)} />
                  {!run.hasStructuredSteps ? (
                    <Label isCompact icon={<OutlinedCircleIcon />}>
                      <FormattedMessage {...messages.e2eNoStructuredSteps} />
                    </Label>
                  ) : null}
                </span>
              </Td>
            </Tr>
          );
        })}
      </Tbody>
    </Table>
  );
}

function StepsTable({
  detail,
  logLines,
  selectedStepId,
  onSelectStep,
  sort,
  onSort,
  nowMs,
}: {
  detail: E2eRunDetail;
  /** The run-wide raw-log buffer (paginated + live), used only for the failed-step
   *  output preview. It is the run-wide reader, independent of the selected-step log
   *  view, so selecting a step for the log pane does not strip other steps' previews
   *  (E2E-LOG-12/23); the detail itself no longer embeds logs (E2E-LOG-19/20). */
  logLines: readonly E2eLogLine[];
  selectedStepId: string | null;
  onSelectStep: (stepId: string) => void;
  sort: SortState | null;
  onSort: (sort: SortState | null) => void;
  nowMs: number;
}): React.ReactElement {
  const intl = useIntl();
  const { sorted, column } = useTableSort(
    detail.steps,
    (step) => [
      step.name || step.stepId,
      step.state,
      step.durationMs ??
        (step.state === "running" && step.startedAt
          ? nowMs - Date.parse(step.startedAt)
          : null),
    ],
    sort,
    onSort,
  );
  return (
    <Table
      gridBreakPoint=""
      className={styles.stepsTable}
      aria-label={intl.formatMessage(messages.e2eStepsAria)}
      variant="compact"
    >
      <Thead>
        <Tr>
          <Th sort={column(0)}>
            <FormattedMessage {...messages.e2eColumnStep} />
          </Th>
          <Th sort={column(1)}>
            <FormattedMessage {...messages.e2eColumnState} />
          </Th>
          <Th sort={column(2)}>
            <FormattedMessage {...messages.e2eColumnDuration} />
          </Th>
        </Tr>
      </Thead>
      <Tbody>
        {sorted.map((step) => {
          const dur =
            step.durationMs ??
            (step.state === "running" && step.startedAt !== null
              ? runDurationMs(
                  {
                    startedAt: step.startedAt,
                    endedAt: null,
                    durationMs: null,
                  },
                  nowMs,
                )
              : null);
          const selected = step.stepId === selectedStepId;
          // The failed-step snippet is best-effort from whatever log pages have
          // loaded (the newest page loads on open); empty until logs arrive.
          const lastOutput =
            step.state === "failed"
              ? logsForStep(logLines, step.stepId).at(-1)?.message
              : null;
          return (
            <Tr
              key={step.stepId}
              isRowSelected={selected}
              className={selected ? styles.selectedRow : undefined}
            >
              <Td dataLabel={intl.formatMessage(messages.e2eColumnStep)}>
                <Button
                  variant="link"
                  isInline
                  onClick={() => {
                    onSelectStep(step.stepId);
                  }}
                >
                  {step.name !== ""
                    ? step.name.replaceAll("-", " ")
                    : step.stepId}
                </Button>
                {lastOutput ? (
                  <small className={styles.failureOutput}>
                    {lastOutput.slice(0, 180)}
                  </small>
                ) : null}
              </Td>
              <Td dataLabel={intl.formatMessage(messages.e2eColumnState)}>
                <StatusChip visual={stepStateVisual(step.state)} />
              </Td>
              <Td dataLabel={intl.formatMessage(messages.e2eColumnDuration)}>
                {formatDuration(intl, dur)}
              </Td>
            </Tr>
          );
        })}
      </Tbody>
    </Table>
  );
}

function LogPane({
  detail,
  logs,
  selectedStepId,
  onSelectScope,
  search,
  onSearch,
  expanded,
  onExpanded,
}: {
  detail: E2eRunDetail;
  /** The paginated + live-streamed log buffer and its controls (E2E-LOG-19/21/22). */
  logs: RunLogs;
  selectedStepId: string | null;
  onSelectScope: (stepId: string | null) => void;
  /** Log search text (URL-coalesced, FLT-NAV-11). */
  search: string;
  onSearch: (value: string) => void;
  /** Expanded log viewer (URL-represented, FLT-NAV-11). */
  expanded: boolean;
  onExpanded: (value: boolean) => void;
}): React.ReactElement {
  const intl = useIntl();
  // follow/pause and the timestamp toggle are transient view toggles, not
  // URL-represented (FLT-NAV-11).
  const [following, setFollowing] = useState(true);
  const [timestamps, setTimestamps] = useState(false);
  const paneRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);
  // Scroll anchor captured before an older page is prepended, so the viewport does
  // not jump when older lines are inserted at the top (E2E-LOG-22).
  const anchor = useRef<{ height: number; top: number } | null>(null);

  const {
    lines: buffer,
    hasMoreOlder,
    loadingOlder,
    loadingInitial,
    failed,
    locallyLimited,
    loadOlder,
    retry,
  } = logs;
  // The buffer is already scoped to the selected step by the hook (E2E-LOG-23), so it
  // is rendered directly rather than filtered at render time.
  const matches = buffer.filter((line) =>
    line.message.toLowerCase().includes(search.toLowerCase()),
  );
  // The in-memory buffer is bounded (MAX_LOG_BUFFER), so rendering it bounds the DOM
  // too; infinite scroll loads older pages into the same bounded buffer (E2E-LOG-22).
  const rendered = matches;

  // The log pane distinguishes five states (E2E-LOG-26): loading, empty, end-of-
  // history, locally-limited history (the in-memory ceiling), and failed loading.
  const showSpinner = loadingOlder || loadingInitial;
  const showEmpty = rendered.length === 0 && !showSpinner && !failed;
  const showLimited = !showSpinner && !failed && locallyLimited;
  const showEndOfHistory =
    !showSpinner &&
    !failed &&
    !locallyLimited &&
    !hasMoreOlder &&
    rendered.length > 0;

  // Older-page load fires from the scroll observer, not a button (E2E-LOG-22): a
  // sentinel at the top of the scroll container triggers the next older page when it
  // enters view, guarded so only one page is in flight and stopped while a history
  // request is in an explicit failed state (E2E-LOG-26).
  const triggerLoadOlder = useCallback(() => {
    if (!hasMoreOlder || loadingOlder || failed) {
      return;
    }
    const pane = paneRef.current;
    if (pane) {
      anchor.current = { height: pane.scrollHeight, top: pane.scrollTop };
    }
    loadOlder();
  }, [hasMoreOlder, loadingOlder, failed, loadOlder]);

  // Retry a failed history request. When lines are already loaded the failure was an
  // older-page fetch, so anchor the scroll position first (as the observer does) so a
  // recovered older page does not jump the viewport on prepend (E2E-LOG-22/26).
  const onRetry = useCallback(() => {
    if (buffer.length > 0) {
      const pane = paneRef.current;
      if (pane) {
        anchor.current = { height: pane.scrollHeight, top: pane.scrollTop };
      }
    }
    retry();
  }, [buffer.length, retry]);

  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel || typeof IntersectionObserver === "undefined") {
      return;
    }
    const observer = new IntersectionObserver((entries) => {
      for (const entry of entries) {
        if (entry.isIntersecting) {
          triggerLoadOlder();
        }
      }
    });
    observer.observe(sentinel);
    return () => {
      observer.disconnect();
    };
  }, [triggerLoadOlder]);

  // On each buffer change: restore the anchored position when older lines were
  // prepended; otherwise keep the view pinned to the bottom while following
  // (auto-follow for a live run, E2E-LOG-22).
  const lastSequence = rendered.at(-1)?.sequence;
  useLayoutEffect(() => {
    const pane = paneRef.current;
    if (!pane) {
      return;
    }
    if (anchor.current) {
      pane.scrollTop =
        pane.scrollHeight - anchor.current.height + anchor.current.top;
      anchor.current = null;
    } else if (following) {
      pane.scrollTop = pane.scrollHeight;
    }
  }, [following, lastSequence, rendered.length, selectedStepId, search]);

  // triggerLoadOlder sets anchor.current before the fetch, and the layout effect
  // above clears it once older lines are prepended. When a page adds no lines (a
  // stale or failed older-page fetch), lastSequence and rendered.length do not
  // change, so that effect never runs and the anchor would stay set, freezing
  // auto-follow (onScroll returns early while an anchor is pending). Clear a
  // dangling anchor when the older-page load finishes so scrolling resumes follow.
  useEffect(() => {
    if (!loadingOlder) {
      anchor.current = null;
    }
  }, [loadingOlder]);

  // Dropping out of follow when the operator scrolls up reveals the jump-to-latest
  // affordance; returning to the bottom resumes auto-follow (E2E-LOG-22).
  const onScroll = useCallback(() => {
    const pane = paneRef.current;
    if (!pane || anchor.current) {
      return;
    }
    const atBottom = pane.scrollHeight - pane.scrollTop - pane.clientHeight < 8;
    setFollowing(atBottom);
  }, []);

  const content = (
    <Stack hasGutter>
      <StackItem>
        <div className={styles.logControls}>
          <FormSelect
            value={selectedStepId ?? ""}
            aria-label={intl.formatMessage(messages.e2eLogScope)}
            onChange={(_, value) => {
              onSelectScope(value || null);
            }}
          >
            <FormSelectOption
              value=""
              label={intl.formatMessage(messages.e2eLogScopeRun)}
            />
            {detail.steps.map((step) => (
              <FormSelectOption
                key={step.stepId}
                value={step.stepId}
                label={step.name || step.stepId}
              />
            ))}
          </FormSelect>
          <SearchInput
            value={search}
            aria-label={intl.formatMessage(messages.e2eSearchLogs)}
            placeholder={intl.formatMessage(messages.e2eSearchLogs)}
            onChange={(_, value) => {
              onSearch(value);
            }}
            onClear={() => {
              onSearch("");
            }}
          />
          <Button
            variant="secondary"
            size="sm"
            icon={following ? <PauseIcon /> : <PlayIcon />}
            onClick={() => {
              setFollowing((f) => !f);
            }}
          >
            {following ? (
              <FormattedMessage {...messages.e2ePause} />
            ) : (
              <FormattedMessage {...messages.e2eFollow} />
            )}
          </Button>
          <Checkbox
            id={`e2e-timestamps-${detail.runId}`}
            label={intl.formatMessage(messages.e2eTimestamps)}
            isChecked={timestamps}
            onChange={(_, checked) => {
              setTimestamps(checked);
            }}
          />
          <Button
            variant="link"
            isInline
            onClick={() => {
              onExpanded(!expanded);
            }}
          >
            <FormattedMessage
              {...(expanded
                ? messages.e2eCollapseLogs
                : messages.e2eExpandLogs)}
            />
          </Button>
        </div>
      </StackItem>
      {failed ? (
        <StackItem>
          {/* A failed/stale/timed-out history request is surfaced with an explicit,
              recoverable Retry rather than a stuck loading indicator, and it is a
              logs-only notice so the steps above stay visible (E2E-LOG-20/26). The
              already-loaded lines below are preserved. */}
          <Alert
            variant="warning"
            isInline
            isPlain
            title={<FormattedMessage {...messages.e2eLogsFailed} />}
            actionLinks={
              <AlertActionLink onClick={onRetry}>
                <FormattedMessage {...messages.e2eLogsRetry} />
              </AlertActionLink>
            }
          />
        </StackItem>
      ) : null}
      <StackItem>
        <div className={styles.logPaneWrap}>
          <div
            className={styles.logPane}
            ref={paneRef}
            onScroll={onScroll}
            role="log"
            aria-label={intl.formatMessage(messages.e2eLogPaneAria)}
          >
            {/* Top sentinel + loading indicator for infinite scroll (E2E-LOG-22). */}
            <div ref={sentinelRef} className={styles.logSentinel} aria-hidden />
            {showSpinner ? (
              <div
                className={styles.logLoadingRow}
                role="status"
                aria-label={intl.formatMessage(messages.e2eLogsLoadingOlder)}
              >
                <Spinner size="sm" aria-hidden />
                <Content component="small">
                  <FormattedMessage {...messages.e2eLogsLoadingOlder} />
                </Content>
              </div>
            ) : null}
            {/* The in-memory buffer ceiling is a distinct state from the true end of
                history: say so rather than implying all history was read (E2E-LOG-26). */}
            {showLimited ? (
              <Content component="small">
                <FormattedMessage {...messages.e2eLogsLimited} />
              </Content>
            ) : null}
            {showEndOfHistory ? (
              <Content component="small">
                <FormattedMessage {...messages.e2eLogsEndOfHistory} />
              </Content>
            ) : null}
            {showEmpty ? (
              <Content component="small">
                <FormattedMessage {...messages.e2eLogsEmpty} />
              </Content>
            ) : (
              rendered.map((line) => (
                <span
                  key={line.sequence}
                  className={styles.logLine}
                  data-testid="e2e-log-line"
                >
                  {timestamps ? (
                    <span className={styles.logTime}>{line.timestamp}</span>
                  ) : null}
                  {selectedStepId === null && line.stepId ? (
                    <span className={styles.logStep}>{line.stepId}</span>
                  ) : null}
                  {/* Rendered as text: React escapes it, so record content cannot
                      inject markup (E2E-LOG-14). */}
                  {line.message}
                </span>
              ))
            )}
          </div>
          {!following ? (
            <Button
              className={styles.jumpToLatest}
              variant="secondary"
              size="sm"
              icon={<PlayIcon />}
              onClick={() => {
                setFollowing(true);
              }}
            >
              <FormattedMessage {...messages.e2eJumpToLatest} />
            </Button>
          ) : null}
        </div>
      </StackItem>
    </Stack>
  );
  return expanded ? (
    <Modal
      isOpen
      variant={ModalVariant.large}
      onClose={() => {
        onExpanded(false);
      }}
      aria-label={intl.formatMessage(messages.e2eLogPaneTitle)}
    >
      <ModalHeader title={intl.formatMessage(messages.e2eLogPaneTitle)} />
      <ModalBody>{content}</ModalBody>
    </Modal>
  ) : (
    content
  );
}

/**
 * The selected run's detail pane. Run-keyed by the parent (key={runId}) so its
 * snapshot/live state resets naturally on selection change - no in-effect reset. It
 * loads the snapshot detail through the port (the stream's backfill baseline) and
 * folds live events onto it in the domain.
 */
export function SelectedRunPane({
  runId,
  nowMs: suppliedNowMs,
  promotion,
  controls,
}: {
  runId: string;
  nowMs?: number;
  promotion?: PromotionData;
  /** Shared execution-detail controls, wired to the owning route slice. */
  controls: ExecControls;
}): React.ReactElement {
  const intl = useIntl();
  const [clock, setClock] = useState(() => Date.now());
  useEffect(() => {
    const timer = setInterval(() => {
      setClock(Date.now());
    }, 1000);
    return () => {
      clearInterval(timer);
    };
  }, []);
  const nowMs = suppliedNowMs ?? clock;
  const api = useFleetApi();
  const observability = useObservability();
  const [snapshot, setSnapshot] = useState<E2eRunDetail | null>(null);
  const [backendUnavailable, setBackendUnavailable] = useState(false);
  // The log scope and sub-tab are URL-represented via the shared controls, so they
  // survive reload and Back/Forward without restarting the live stream (FLT-NAV-18).
  const selectedStepId = controls.exec.step;
  const activeTab: string | number = controls.exec.tab;

  useEffect(() => {
    const controller = new AbortController();
    api
      .getE2eRun(runId, controller.signal)
      .then((plane) => {
        setSnapshot(plane.data);
        setBackendUnavailable(plane.stale);
      })
      .catch(() => {
        if (!controller.signal.aborted) {
          setBackendUnavailable(true);
        }
      });
    return () => {
      controller.abort();
    };
  }, [api, runId]);

  // Logs are paged (and live-appended) independently of the steps snapshot, so a
  // slow or failed log read never blanks the steps (E2E-LOG-19/20). Two scoped
  // readers keep the run-wide progress summary independent of the selected-step log
  // view (E2E-LOG-12/23): runWideLogs is the single run-wide reader (it feeds the
  // step table's failed-output preview and the run-wide log view, and takes the live
  // tail); stepLogs requests the selected step's lines directly and stays idle while
  // no step is selected. The log pane renders whichever matches the current scope.
  const runWideLogs = useRunLogs(runId, null);
  const stepLogs = useRunLogs(runId, selectedStepId, true);
  const activeLogs = selectedStepId === null ? runWideLogs : stepLogs;
  // The live tail opens only while the run is in progress (E2E-LOG-21); the decision
  // comes from the loaded snapshot's terminal state.
  const live = snapshot !== null && isRunning(snapshot);
  const runWideAppendLive = runWideLogs.appendLive;
  const stepAppendLive = stepLogs.appendLive;
  const appendLive = useCallback(
    (line: E2eLogLine) => {
      // Fan the live line to both readers: the run-wide buffer always takes it; the
      // step buffer takes it only when its scope matches (E2E-LOG-21/23).
      runWideAppendLive(line);
      stepAppendLive(line);
    },
    [runWideAppendLive, stepAppendLive],
  );
  const { detail, status } = useLiveRunDetail(
    runId,
    snapshot,
    live,
    appendLive,
    observability,
  );

  if (detail === null) {
    return backendUnavailable ? (
      <Alert
        variant="warning"
        isInline
        title={<FormattedMessage {...messages.e2eBackendUnavailable} />}
      />
    ) : (
      <Content component="p">
        <FormattedMessage {...messages.loadingPlane} />
      </Content>
    );
  }

  const streamState = status?.state ?? null;
  const dropped = status?.droppedEntries ?? 0;
  const liveDisconnected = streamState !== null && streamState !== "live";

  return (
    <div className={styles.detail}>
      <div className={styles.runHeader}>
        <RunBundleIdentity bundle={runBundle(detail, promotion)} />
        <span>
          {runTargetInstance(detail, promotion) ?? detail.cluster}
          <small className={styles.instance}>{detail.namespace}</small>
        </span>
      </div>
      <div className={styles.runHeader}>
        <StatusChip
          visual={
            detail.verdict === "unknown" &&
            streamState === "live" &&
            detail.steps.some((step) => step.state === "running")
              ? stepStateVisual("running")
              : verdictVisual(detail.verdict)
          }
        />
        <code title={detail.revision}>{detail.revision.slice(0, 8)}</code>
        <span title={detail.runId}>
          {intl.formatMessage(messages.e2eAttemptShort, {
            id: detail.runId.slice(0, 8),
          })}
        </span>
      </div>

      {/* The four degraded states, kept distinct (E2E-LOG-10). */}
      {backendUnavailable ? (
        <Alert
          variant="warning"
          isInline
          isPlain
          title={<FormattedMessage {...messages.e2eBackendUnavailable} />}
        />
      ) : null}
      {detail.verdict === "failed" ? (
        <Alert
          variant="danger"
          isInline
          isPlain
          title={<FormattedMessage {...messages.e2eTestFailed} />}
        />
      ) : null}
      {detail.verdict === "unknown" &&
      !detail.reportingComplete &&
      streamState !== "live" ? (
        <Alert
          variant="warning"
          isInline
          isPlain
          title={<FormattedMessage {...messages.e2eReportingIncomplete} />}
        />
      ) : null}
      {liveDisconnected ? (
        <Alert
          variant="warning"
          isInline
          isPlain
          customIcon={<ExclamationTriangleIcon />}
          title={
            streamState === "gap" ? (
              <FormattedMessage
                {...messages.e2eLiveGap}
                values={{ count: dropped }}
              />
            ) : (
              <FormattedMessage {...messages.e2eLiveDisconnected} />
            )
          }
        />
      ) : null}
      <div className={styles.runHeader}>
        {streamState === "live" ? (
          <span className={styles.inline}>
            <InProgressIcon style={{ color: TONE_COLOR.info }} aria-hidden />
            <small>
              <FormattedMessage {...messages.e2eLiveLabel} />
            </small>
          </span>
        ) : null}

        <small>
          <FormattedMessage
            {...messages.e2eSourceRevision}
            values={{
              revision:
                detail.sourceRevision?.slice(0, 8) ??
                intl.formatMessage(messages.valueNone),
            }}
          />
        </small>

        <small>
          {intl.formatMessage(messages.e2eRunDuration, {
            duration: formatDuration(intl, runDurationMs(detail, nowMs)),
          })}
        </small>
      </div>

      <Tabs
        activeKey={activeTab}
        onSelect={(_, key) => {
          controls.onTab(key === "logs" ? "logs" : "progress");
        }}
        aria-label={intl.formatMessage(messages.e2eExecutionTabs)}
      >
        <Tab
          eventKey="progress"
          title={
            <TabTitleText>
              <FormattedMessage {...messages.e2eProgressTab} />
            </TabTitleText>
          }
        >
          <div className={styles.tabBody}>
            {!detail.hasStructuredSteps ? (
              <Content component="p">
                <span className={styles.inline}>
                  <OutlinedCircleIcon aria-hidden />
                  <FormattedMessage {...messages.e2eNoStructuredSteps} />
                </span>
              </Content>
            ) : detail.steps.length === 0 ? (
              <Content component="p">
                <FormattedMessage {...messages.e2eNoSteps} />
              </Content>
            ) : (
              <StepsTable
                detail={detail}
                logLines={runWideLogs.lines}
                selectedStepId={selectedStepId}
                onSelectStep={(id) => {
                  controls.onStep(id);
                }}
                sort={controls.exec.stepSort}
                onSort={controls.onStepSort}
                nowMs={nowMs}
              />
            )}
          </div>
        </Tab>
        <Tab
          eventKey="logs"
          title={
            <TabTitleText>
              <FormattedMessage {...messages.e2eLogPaneTitle} />
            </TabTitleText>
          }
        >
          <div className={styles.tabBody}>
            <LogPane
              detail={detail}
              logs={activeLogs}
              selectedStepId={selectedStepId}
              onSelectScope={controls.onStep}
              search={controls.exec.logQuery}
              onSearch={controls.onLogQuery}
              expanded={controls.exec.logExpanded}
              onExpanded={controls.onLogExpanded}
            />
          </div>
        </Tab>
      </Tabs>
    </div>
  );
}

/** Build the e2e run execution controls from the route (FLT-NAV-11). */
function useE2eExecControls(exec: ExecState): ExecControls {
  const { navigate } = useRouter();
  return useMemo(
    () => ({
      exec,
      onTab: (tab) => {
        navigate({ type: "e2eExecTab", tab });
      },
      onStep: (step) => {
        navigate({ type: "e2eStep", step });
      },
      onStepSort: (sort) => {
        navigate({ type: "e2eStepSort", sort });
      },
      onLogExpanded: (value) => {
        navigate({ type: "e2eLogExpanded", value });
      },
      onLogQuery: (value) => {
        navigate({ type: "e2eLogQuery", value });
      },
    }),
    [exec, navigate],
  );
}

export function E2eRunView(props: E2eRunViewProps): React.ReactElement {
  const { data } = props;
  const intl = useIntl();
  const nowMs = resolveNowMs(props.nowMs);
  // The URL is authoritative for selection, drawer, search, sort, and the shared
  // execution detail (FLT-NAV-05, FLT-NAV-12).
  const route = useRoute();
  const { navigate } = useRouter();
  const selectedId = route.e2e.run;
  const isExpanded = route.e2e.drawer;
  const search = route.e2e.query;
  const execControls = useE2eExecControls(route.e2e.exec);

  const [inline, setInline] = useState(
    () =>
      typeof window.matchMedia === "function" &&
      window.matchMedia("(min-width: 1600px)").matches,
  );
  const triggerRef = useRef<HTMLElement | null>(null);
  const headingRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const media = window.matchMedia("(min-width: 1600px)");
    const update = () => {
      setInline(media.matches);
    };
    media.addEventListener("change", update);
    return () => {
      media.removeEventListener("change", update);
    };
  }, []);
  const close = useCallback(() => {
    // Closing preserves the selected run (FLT-NAV-12); only the drawer closes.
    navigate({ type: "e2eDrawer", open: false });
    triggerRef.current?.focus();
  }, [navigate]);
  useEffect(() => {
    if (!isExpanded) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (
        event.key === "Escape" &&
        !event.defaultPrevented &&
        !(event.target as HTMLElement).closest('[role="dialog"]')
      )
        close();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [isExpanded, close]);

  const runs = useMemo(
    () =>
      runsNewestFirst(data.runs).filter((run) =>
        `${run.namespace} ${run.cluster} ${run.revision} ${run.runId} ${runTargetInstance(run, props.promotion) ?? ""} ${runBundle(run, props.promotion)?.version ?? ""}`
          .toLowerCase()
          .includes(search.toLowerCase()),
      ),
    [data.runs, search, props.promotion],
  );

  if (!data.enabled) {
    return (
      <Content component="p">
        <span className={styles.inline}>
          <OutlinedCircleIcon aria-hidden />
          <FormattedMessage {...messages.e2eDisabled} />
        </span>
      </Content>
    );
  }

  return (
    <div className={styles.view}>
      {data.truncated ? (
        <Alert
          variant="info"
          isInline
          isPlain
          title={<FormattedMessage {...messages.e2eHistoryTruncated} />}
        />
      ) : null}
      <SearchInput
        value={search}
        aria-label={intl.formatMessage(messages.e2eSearchRuns)}
        placeholder={intl.formatMessage(messages.e2eSearchRuns)}
        onChange={(_, value) => {
          navigate({ type: "e2eQuery", value });
        }}
        onClear={() => {
          navigate({ type: "e2eQuery", value: "" });
        }}
      />
      <div className={styles.historyStage}>
        <Drawer
          isExpanded={isExpanded}
          isInline={inline}
          onExpand={() => {
            headingRef.current?.focus();
          }}
        >
          <DrawerContent
            panelContent={
              <DrawerPanelContent
                isResizable={inline}
                defaultSize="560px"
                minSize="320px"
                maxSize="100%"
                resizeAriaLabel={intl.formatMessage(messages.e2eResizeDetails)}
                focusTrap={{ enabled: isExpanded && !inline }}
              >
                <DrawerHead>
                  <div ref={headingRef} tabIndex={-1}>
                    <Title headingLevel="h3" size="lg">
                      {intl.formatMessage(messages.e2eAttemptShort, {
                        id:
                          data.runs
                            .find((run) => run.id === selectedId)
                            ?.runId.slice(0, 8) ??
                          selectedId ??
                          "",
                      })}
                    </Title>
                  </div>
                  <DrawerActions>
                    <DrawerCloseButton
                      aria-label={intl.formatMessage(messages.drawerClose)}
                      onClick={close}
                    />
                  </DrawerActions>
                </DrawerHead>
                <DrawerPanelBody className={styles.historyDetails}>
                  {isExpanded && selectedId !== null ? (
                    <SelectedRunPane
                      key={selectedId}
                      runId={selectedId}
                      promotion={props.promotion}
                      nowMs={nowMs}
                      controls={execControls}
                    />
                  ) : null}
                </DrawerPanelBody>
              </DrawerPanelContent>
            }
          >
            <DrawerContentBody className={styles.historyTable}>
              {runs.length === 0 ? (
                <Content component="p">
                  <FormattedMessage {...messages.e2eNoRuns} />
                </Content>
              ) : (
                <RunsTable
                  promotion={props.promotion}
                  runs={runs}
                  selectedId={selectedId}
                  onActivate={(event) => {
                    // Record the trigger for focus restoration on close; the link
                    // itself performs the navigation (FLT-NAV-08, FLT-NAV-15).
                    triggerRef.current = event.currentTarget as HTMLElement;
                  }}
                  sort={route.e2e.runSort}
                  onSort={(sort) => {
                    navigate({ type: "e2eRunSort", sort });
                  }}
                  nowMs={nowMs}
                />
              )}
            </DrawerContentBody>
          </DrawerContent>
        </Drawer>
      </div>
    </div>
  );
}
