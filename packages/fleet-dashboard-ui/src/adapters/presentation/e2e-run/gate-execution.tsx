import {
  Alert,
  Button,
  Content,
  FormSelect,
  FormSelectOption,
} from "@patternfly/react-core";
import { useCallback, useEffect, useMemo } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import { runsForVerification, type E2eRunsData } from "../../../domain/e2e-run";
import type {
  PromotionData,
  VerificationTarget,
} from "../../../domain/promotion";
import { GATE_LIVE_ATTEMPT } from "../../../domain/navigation/route";
import { messages } from "../../../messages";
import { useNavLink, useRoute, useRouter } from "../../routing/router";
import { SelectedRunPane, type ExecControls } from "./e2e-run-view";
import styles from "./e2e-run-view.module.css";

export interface GateExecutionProps {
  readonly promotion?: PromotionData;
  readonly target: VerificationTarget | null | undefined;
  readonly data?: E2eRunsData;
  readonly unavailable?: boolean;
  // When set, render a real link into E2E run history for the current attempt.
  // The link navigates through its own useNavLink, so this is a plain feature flag.
  readonly showHistoryLink?: boolean;
}

/** Build the gate execution controls from the route (FLT-NAV-11). */
function useGateExecControls(): ExecControls {
  const route = useRoute();
  const { navigate } = useRouter();
  const exec = route.map.exec;
  return useMemo(
    () => ({
      exec,
      onTab: (tab) => {
        navigate({ type: "gateExecTab", tab });
      },
      onStep: (step) => {
        navigate({ type: "gateStep", step });
      },
      onStepSort: (sort) => {
        navigate({ type: "gateStepSort", sort });
      },
      onLogExpanded: (value) => {
        navigate({ type: "gateLogExpanded", value });
      },
      onLogQuery: (value) => {
        navigate({ type: "gateLogQuery", value });
      },
    }),
    [exec, navigate],
  );
}

// The gate's pinned attempt lives in the route (FLT-NAV-10): the explicit
// live/latest mode (attempt null) follows the current attempt, while a pinned
// attempt keeps addressing that attempt even after a retry arrives.
export function GateExecution({
  target,
  promotion,
  data,
  unavailable,
  showHistoryLink,
}: GateExecutionProps): React.ReactElement {
  const intl = useIntl();
  const runs = runsForVerification(data?.runs ?? [], target);
  const route = useRoute();
  const { navigate } = useRouter();
  const controls = useGateExecControls();
  // FLT-NAV-10: three attempt modes. `null` is a fresh open that auto-pins the
  // current attempt so a later retry does not drift it (E2E-LOG-12). The explicit
  // live sentinel follows the current attempt and never auto-pins. Any other value
  // is a pinned run address.
  const attempt = route.map.attempt;
  const live = attempt === GATE_LIVE_ATTEMPT;
  const pinned = attempt !== null && !live ? attempt : null;
  const runId = pinned ?? runs[0]?.id ?? null;
  // The dropdown value: the live sentinel in live mode, else the resolved run.
  const selectValue = live ? GATE_LIVE_ATTEMPT : runId;
  // Stable pin callback so PinnedRun's mount effect does not re-run every render.
  const pinAttempt = useCallback(
    (id: string) => {
      // Pinning the auto-selected attempt establishes canonical state; it replaces
      // rather than adding a Back step (FLT-NAV-10).
      navigate({ type: "gatePin", attempt: id }, { replace: true });
    },
    [navigate],
  );
  return (
    <div className={styles.detail}>
      {unavailable ? (
        <Alert
          variant="warning"
          isInline
          isPlain
          title={intl.formatMessage(messages.e2eBackendUnavailable)}
        />
      ) : null}
      {!target ? (
        <Content component="p">
          <FormattedMessage {...messages.e2eGateNoTarget} />
        </Content>
      ) : data && !data.enabled ? (
        <Content component="p">
          <FormattedMessage {...messages.e2eDisabled} />
        </Content>
      ) : runId === null ? (
        <Content component="p">
          <FormattedMessage {...messages.e2eGateNoRun} />
        </Content>
      ) : (
        <>
          <div className={styles.attemptControls}>
            <FormSelect
              aria-label={intl.formatMessage(messages.e2eAttempts)}
              value={selectValue ?? ""}
              onChange={(_, id) => {
                navigate({ type: "gatePin", attempt: id });
              }}
            >
              {/* An explicit live/latest mode, distinct from any pinned attempt. */}
              <FormSelectOption
                value={GATE_LIVE_ATTEMPT}
                label={intl.formatMessage(messages.e2eAttemptLatest)}
              />
              {runs.map((run) => (
                <FormSelectOption
                  key={run.id}
                  value={run.id}
                  label={intl.formatMessage(messages.e2eAttemptOption, {
                    id: run.runId.slice(0, 8),
                    started: intl.formatDate(run.startedAt, {
                      month: "short",
                      day: "numeric",
                      hour: "numeric",
                      minute: "2-digit",
                    }),
                  })}
                />
              ))}
              {pinned && !runs.some((run) => run.id === pinned) ? (
                <FormSelectOption value={pinned} label={pinned} />
              ) : null}
            </FormSelect>
            {showHistoryLink ? <GateHistoryLink runId={runId} /> : null}
          </div>
          {pinned && runs[0]?.id !== pinned ? (
            <Content component="small">
              <FormattedMessage {...messages.e2eGateNewAttempt} />
            </Content>
          ) : null}
          <PinnedRun
            key={runId}
            runId={runId}
            // Auto-pin only on a fresh open (no attempt in the URL); live mode and an
            // already-pinned attempt keep their explicit state (FLT-NAV-10).
            onPin={attempt === null ? pinAttempt : undefined}
            promotion={promotion}
            controls={controls}
          />
        </>
      )}
    </div>
  );
}

// A real link into E2E history for the current attempt, so copy-link, open-in-new-tab,
// and Ctrl/Cmd-click work while a plain click navigates in place (FLT-NAV-08).
function GateHistoryLink({ runId }: { runId: string }): React.ReactElement {
  const intl = useIntl();
  const link = useNavLink({ type: "openHistory", run: runId });
  return (
    <Button component="a" variant="link" isInline {...link}>
      {intl.formatMessage(messages.e2eGateHistory)}
    </Button>
  );
}

// Pin the auto-selected attempt on mount, outside render. The details component
// owns the single stream used by both tabs and closes it on a selection change.
function PinnedRun({
  runId,
  onPin,
  promotion,
  controls,
}: {
  promotion?: PromotionData;
  runId: string;
  onPin?: (id: string) => void;
  controls: ExecControls;
}): React.ReactElement {
  useEffect(() => {
    onPin?.(runId);
  }, [runId, onPin]);
  return (
    <SelectedRunPane runId={runId} promotion={promotion} controls={controls} />
  );
}
