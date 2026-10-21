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
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { IntlShape, MessageDescriptor } from "react-intl";
import { FormattedMessage, useIntl } from "react-intl";

import {
  applyEvents,
  elapsedSeconds,
  logsForStep,
  reduceRun,
  runDurationMs,
  runBundle,
  runTargetInstance,
  runsNewestFirst,
  MAX_DETAIL_LOG_LINES,
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

/** How many log lines the pane renders, so an unbounded log cannot blow up the DOM. */
const MAX_RENDERED_LOG_LINES = 500;

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
  observability: ObservabilityPort,
): { detail: E2eRunDetail | null; status: E2eStreamStatus | null } {
  const api = useFleetApi();
  const [events, setEvents] = useState<readonly E2eRunEvent[]>([]);
  const [status, setStatus] = useState<E2eStreamStatus | null>(null);

  useEffect(() => {
    // This hook lives inside a run-keyed pane, so `id` is stable for the pane's life
    // and the subscription is set up once on mount; no in-effect reset is needed.
    let prevState: E2eStreamStatus["state"] | null = null;
    observability.publishE2eStream(e2eStreamFact("connect"));
    const unsubscribe = api.streamE2eRun(id, {
      onEvent: (event) => {
        setEvents((prev) => appendBoundedEvents(prev, event));
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
  }, [id, api, observability]);

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
  selectedStepId,
  onSelectStep,
  sort,
  onSort,
  nowMs,
}: {
  detail: E2eRunDetail;
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
          const lastOutput =
            step.state === "failed"
              ? logsForStep(detail.logs, step.stepId).at(-1)?.message
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
  selectedStepId,
  onSelectScope,
  search,
  onSearch,
  expanded,
  onExpanded,
}: {
  detail: E2eRunDetail;
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

  const lines = useMemo(
    () =>
      selectedStepId === null
        ? detail.logs
        : logsForStep(detail.logs, selectedStepId),
    [detail.logs, selectedStepId],
  );
  // Bounded rendering: only the most recent lines are ever in the DOM (E2E-LOG-12).
  const matches = lines.filter((line) =>
    line.message.toLowerCase().includes(search.toLowerCase()),
  );
  const rendered = matches.slice(-MAX_RENDERED_LOG_LINES);

  const lastSequence = rendered.at(-1)?.sequence;
  useEffect(() => {
    if (following && paneRef.current) {
      paneRef.current.scrollTop = paneRef.current.scrollHeight;
    }
  }, [following, lastSequence, selectedStepId, search]);

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
      {detail.logsTruncated || matches.length > MAX_RENDERED_LOG_LINES ? (
        <StackItem>
          <Alert
            variant="info"
            isInline
            isPlain
            title={<FormattedMessage {...messages.e2eLogsTruncated} />}
          />
        </StackItem>
      ) : null}
      <StackItem>
        <div
          className={styles.logPane}
          ref={paneRef}
          role="log"
          aria-label={intl.formatMessage(messages.e2eLogPaneAria)}
        >
          {rendered.length === 0 ? (
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

  const { detail, status } = useLiveRunDetail(runId, snapshot, observability);

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
