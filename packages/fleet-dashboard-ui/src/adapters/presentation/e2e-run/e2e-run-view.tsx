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

import {
  Alert,
  Button,
  Content,
  Label,
  Stack,
  StackItem,
  Title,
  ToggleGroup,
  ToggleGroupItem,
} from "@patternfly/react-core";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
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
import { useEffect, useMemo, useRef, useState } from "react";
import type { IntlShape, MessageDescriptor } from "react-intl";
import { FormattedMessage, useIntl } from "react-intl";

import {
  applyEvents,
  elapsedSeconds,
  logsForStep,
  reduceRun,
  runDurationMs,
  runsNewestFirst,
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
import { messages } from "../../../messages";
import { useFleetApi } from "../../query/api-context";
import { useObservability } from "../../observability/observability-context";
import { TONE_COLOR } from "../map/colors";
import styles from "./e2e-run-view.module.css";

/** How many log lines the pane renders, so an unbounded log cannot blow up the DOM. */
const MAX_RENDERED_LOG_LINES = 500;

/**
 * Resolve the reference instant. Kept in a plain helper (not the component body) so
 * the fallback clock read stays out of render purity analysis, matching the map's
 * releaseTimeParts pattern; tests inject a fixed nowMs.
 */
function resolveNowMs(nowMs: number | undefined): number {
  return nowMs ?? Date.now();
}

export interface E2eRunViewProps {
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
    <Label isCompact icon={<Icon style={{ color: visual.tone }} />}>
      <FormattedMessage {...visual.message} />
    </Label>
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
        setEvents((prev) => [...prev, event]);
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

function RunsTable({
  runs,
  selectedId,
  onSelect,
  nowMs,
}: {
  runs: readonly E2eRunSummary[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  nowMs: number;
}): React.ReactElement {
  const intl = useIntl();
  return (
    <Table
      aria-label={intl.formatMessage(messages.e2eRunsAria)}
      variant="compact"
    >
      <Thead>
        <Tr>
          <Th>
            <FormattedMessage {...messages.e2eColumnRevision} />
          </Th>
          <Th>
            <FormattedMessage {...messages.e2eColumnAttempt} />
          </Th>
          <Th>
            <FormattedMessage {...messages.e2eColumnStarted} />
          </Th>
          <Th>
            <FormattedMessage {...messages.e2eColumnDuration} />
          </Th>
          <Th>
            <FormattedMessage {...messages.e2eColumnVerdict} />
          </Th>
        </Tr>
      </Thead>
      <Tbody>
        {runs.map((run) => {
          const selected = run.id === selectedId;
          return (
            <Tr
              key={run.id}
              isRowSelected={selected}
              className={selected ? styles.selectedRow : undefined}
            >
              <Td dataLabel={intl.formatMessage(messages.e2eColumnRevision)}>
                <Button
                  variant="link"
                  isInline
                  onClick={() => {
                    onSelect(run.id);
                  }}
                  aria-label={intl.formatMessage(messages.e2eRunSelect, {
                    revision: run.revision,
                    attempt: run.runId,
                  })}
                >
                  {run.revision !== ""
                    ? run.revision
                    : intl.formatMessage(messages.valueNone)}
                </Button>
              </Td>
              <Td dataLabel={intl.formatMessage(messages.e2eColumnAttempt)}>
                {run.runId}
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
  nowMs,
}: {
  detail: E2eRunDetail;
  selectedStepId: string | null;
  onSelectStep: (stepId: string) => void;
  nowMs: number;
}): React.ReactElement {
  const intl = useIntl();
  return (
    <Table
      aria-label={intl.formatMessage(messages.e2eStepsAria)}
      variant="compact"
    >
      <Thead>
        <Tr>
          <Th>
            <FormattedMessage {...messages.e2eColumnStep} />
          </Th>
          <Th>
            <FormattedMessage {...messages.e2eColumnState} />
          </Th>
          <Th>
            <FormattedMessage {...messages.e2eColumnDuration} />
          </Th>
        </Tr>
      </Thead>
      <Tbody>
        {detail.steps.map((step) => {
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
                  {step.name !== "" ? step.name : step.stepId}
                </Button>
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
}: {
  detail: E2eRunDetail;
  selectedStepId: string | null;
  onSelectScope: (stepId: string | null) => void;
}): React.ReactElement {
  const intl = useIntl();
  const [following, setFollowing] = useState(true);
  const paneRef = useRef<HTMLDivElement>(null);

  const lines = useMemo(
    () =>
      selectedStepId === null
        ? detail.logs
        : logsForStep(detail.logs, selectedStepId),
    [detail.logs, selectedStepId],
  );
  // Bounded rendering: only the most recent lines are ever in the DOM (E2E-LOG-12).
  const rendered = lines.slice(-MAX_RENDERED_LOG_LINES);

  useEffect(() => {
    if (following && paneRef.current) {
      paneRef.current.scrollTop = paneRef.current.scrollHeight;
    }
  }, [following, rendered.length]);

  return (
    <Stack hasGutter>
      <StackItem>
        <div className={styles.logControls}>
          <ToggleGroup aria-label={intl.formatMessage(messages.e2eLogScope)}>
            <ToggleGroupItem
              text={intl.formatMessage(messages.e2eLogScopeRun)}
              buttonId="e2e-log-run"
              isSelected={selectedStepId === null}
              onChange={() => {
                onSelectScope(null);
              }}
            />
            {selectedStepId !== null ? (
              <ToggleGroupItem
                text={intl.formatMessage(messages.e2eLogScopeStep)}
                buttonId="e2e-log-step"
                isSelected
                onChange={() => {
                  onSelectScope(selectedStepId);
                }}
              />
            ) : null}
          </ToggleGroup>
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
        </div>
      </StackItem>
      {detail.logsTruncated ? (
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
                <span className={styles.logTime}>{line.timestamp}</span>
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
}

/**
 * The selected run's detail pane. Run-keyed by the parent (key={runId}) so its
 * snapshot/live state resets naturally on selection change - no in-effect reset. It
 * loads the snapshot detail through the port (the stream's backfill baseline) and
 * folds live events onto it in the domain.
 */
function SelectedRunPane({
  runId,
  nowMs,
}: {
  runId: string;
  nowMs: number;
}): React.ReactElement {
  const intl = useIntl();
  const api = useFleetApi();
  const observability = useObservability();
  const [snapshot, setSnapshot] = useState<E2eRunDetail | null>(null);
  const [backendUnavailable, setBackendUnavailable] = useState(false);
  const [selectedStepId, setSelectedStepId] = useState<string | null>(null);

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
      <Title headingLevel="h3" size="md">
        {detail.revision !== ""
          ? detail.revision
          : intl.formatMessage(messages.valueNone)}
      </Title>
      <Content component="small">
        <FormattedMessage
          {...messages.e2eSourceRevision}
          values={{
            revision:
              detail.sourceRevision ?? intl.formatMessage(messages.valueNone),
          }}
        />
      </Content>

      {/* The four degraded states, kept distinct (E2E-LOG-10). */}
      {backendUnavailable ? (
        <Alert
          variant="warning"
          isInline
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
      {streamState === "live" ? (
        <span className={styles.inline}>
          <InProgressIcon style={{ color: TONE_COLOR.info }} aria-hidden />
          <Content component="small">
            <FormattedMessage {...messages.e2eLiveLabel} />
          </Content>
        </span>
      ) : null}

      <Content component="small">
        {intl.formatMessage(messages.e2eRunDuration, {
          duration: formatDuration(intl, runDurationMs(detail, nowMs)),
        })}
      </Content>

      <Title headingLevel="h4" size="md">
        <FormattedMessage {...messages.e2eStepsTitle} />
      </Title>
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
          onSelectStep={setSelectedStepId}
          nowMs={nowMs}
        />
      )}

      <Title headingLevel="h4" size="md">
        <FormattedMessage {...messages.e2eLogPaneTitle} />
      </Title>
      <LogPane
        detail={detail}
        selectedStepId={selectedStepId}
        onSelectScope={setSelectedStepId}
      />
    </div>
  );
}

export function E2eRunView(props: E2eRunViewProps): React.ReactElement {
  const { data } = props;
  const nowMs = resolveNowMs(props.nowMs);
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const runs = useMemo(() => runsNewestFirst(data.runs), [data.runs]);

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
      <div className={styles.split}>
        <div>
          {runs.length === 0 ? (
            <Content component="p">
              <FormattedMessage {...messages.e2eNoRuns} />
            </Content>
          ) : (
            <RunsTable
              runs={runs}
              selectedId={selectedId}
              onSelect={setSelectedId}
              nowMs={nowMs}
            />
          )}
        </div>
        <div>
          {selectedId !== null ? (
            <SelectedRunPane
              key={selectedId}
              runId={selectedId}
              nowMs={nowMs}
            />
          ) : (
            <Content component="p">
              <FormattedMessage {...messages.e2eSelectRun} />
            </Content>
          )}
        </div>
      </div>
    </div>
  );
}
