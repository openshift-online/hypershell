import {
  Alert,
  Button,
  Content,
  FormSelect,
  FormSelectOption,
} from "@patternfly/react-core";
import { useEffect, useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import { runsForVerification, type E2eRunsData } from "../../../domain/e2e-run";
import type {
  PromotionData,
  VerificationTarget,
} from "../../../domain/promotion";
import { messages } from "../../../messages";
import { SelectedRunPane } from "./e2e-run-view";
import styles from "./e2e-run-view.module.css";

export interface GateExecutionProps {
  readonly promotion?: PromotionData;
  readonly target: VerificationTarget | null | undefined;
  readonly data?: E2eRunsData;
  readonly unavailable?: boolean;
  readonly onOpenHistory?: (id: string) => void;
}

// Parent keys by gate + exact revision. Keep an inspected attempt pinned even
// when a retry arrives; operators explicitly choose when to follow the retry.
export function GateExecution({
  target,
  promotion,
  data,
  unavailable,
  onOpenHistory,
}: GateExecutionProps): React.ReactElement {
  const intl = useIntl();
  const runs = runsForVerification(data?.runs ?? [], target);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const runId = selectedId ?? runs[0]?.id ?? null;
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
              value={runId}
              onChange={(_, id) => {
                setSelectedId(id);
              }}
            >
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
              {selectedId && !runs.some((run) => run.id === selectedId) ? (
                <FormSelectOption value={selectedId} label={selectedId} />
              ) : null}
            </FormSelect>
            {onOpenHistory ? (
              <Button
                variant="link"
                isInline
                onClick={() => {
                  onOpenHistory(runId);
                }}
              >
                <FormattedMessage {...messages.e2eGateHistory} />
              </Button>
            ) : null}
          </div>
          {selectedId && runs[0]?.id !== selectedId ? (
            <Content component="small">
              <FormattedMessage {...messages.e2eGateNewAttempt} />
            </Content>
          ) : null}
          <PinnedRun
            key={runId}
            runId={runId}
            onPin={setSelectedId}
            promotion={promotion}
          />
        </>
      )}
    </div>
  );
}

// Pin the auto-selected attempt on mount, outside render. The details component
// owns the single stream used by both tabs and closes it on a selection change.
function PinnedRun({
  runId,
  onPin,
  promotion,
}: {
  promotion?: PromotionData;
  runId: string;
  onPin: (id: string) => void;
}): React.ReactElement {
  useEffect(() => {
    onPin(runId);
  }, [runId, onPin]);
  return <SelectedRunPane runId={runId} promotion={promotion} />;
}
