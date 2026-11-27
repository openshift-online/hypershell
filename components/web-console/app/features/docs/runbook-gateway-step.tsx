import {
  Alert,
  Button,
  Form,
  FormGroup,
  FormHelperText,
  FormSelect,
  FormSelectOption,
  HelperText,
  HelperTextItem,
  Skeleton,
  TextInput,
} from "@patternfly/react-core";
import {
  defaultGatewayListRequest,
  gatewayListQueryKey,
  gatewayListQueryRoot,
  gatewayNeedsStatusPolling,
  gatewayPlacementBatchQueryKey,
  gatewayQueryKey,
  gatewayStatusPollMilliseconds,
  GatewayOperationError,
  shellArgument,
  useConsoleWaitTracker,
  type GatewayOperations,
  type GatewayRecord,
} from "@openshift-online/hypershell-gateway-management-ui";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";
import { FormattedMessage, useIntl, type MessageDescriptor } from "react-intl";
import { useLocation } from "react-router";

import { isValidGatewayName } from "../../application/homepage/quick-create";
import type {
  DocsRunbookStepAction,
  DocsRunFailureKind,
} from "../../application/docs/docs-probes";
import {
  chooseRunbookPlacement,
  isGatewayEligibleForRunbook,
  runbookAvailabilitySelectors,
  runbookNeedsClusterCapabilities,
  runbookGatewayStage,
  runbookPlacementCandidates,
  type RunbookGatewayStage,
  type RunbookId,
  type RunbookPlacementCandidate,
  type RunbookPlacementDecision,
} from "../../application/docs/runbook-placement";
import {
  createDocsGatewayNameGenerator,
  docsFacts,
  docsGatewayOperations,
} from "../../composition/docs-composition";
import { currentPathAndQuery, signInHref } from "../../lib/sign-in";
import { sessionQueryKey } from "../shell/use-session";
import {
  useTerminal,
  type RunbookEntryHandle,
} from "../terminal/terminal-context";
import { DocsCommand } from "./docs-command";
import { docsMessages } from "./docs-messages";
import { useDocsSignInPromptShown } from "./docs-facts";
import { RunbookCell, type RunbookCellOutput } from "./runbook-cell";
import type { DocsSessionState } from "./use-docs-session";

const newGatewayChoice = "__new__";

const stageMessages: Record<RunbookGatewayStage, MessageDescriptor> = {
  degraded: docsMessages.stageDegraded,
  failed: docsMessages.stageFailed,
  pending: docsMessages.stagePending,
  provisioning: docsMessages.stageProvisioning,
  running: docsMessages.stageRunning,
  updating: docsMessages.stageUpdating,
  "waiting-for-placement": docsMessages.stageWaitingForPlacement,
};

const failureMessages: Record<DocsRunFailureKind, MessageDescriptor> = {
  cancelled: docsMessages.failureUnknown,
  conflict: docsMessages.failureConflict,
  denied: docsMessages.failureDenied,
  "not-found": docsMessages.failureNotFound,
  unavailable: docsMessages.failureUnavailable,
  unknown: docsMessages.failureUnknown,
};

const placementNames: Record<
  RunbookPlacementCandidate["id"],
  MessageDescriptor
> = {
  "aws-public": docsMessages.placementAwsPublic,
  "aws-vpn": docsMessages.placementAwsVpn,
  "ibm-public": docsMessages.placementIbmPublic,
  kind: docsMessages.placementKind,
};

/** hsctl equivalent of a create run, built with the CLI placement flags. */
export function buildCreateGatewayHsctlCommand(
  name: string,
  candidate: RunbookPlacementCandidate,
): string {
  return [
    "hsctl create gateway",
    `--name ${shellArgument(name)}`,
    ...candidate.placement.selector.map(
      (label) => `--placement-selector ${shellArgument(label)}`,
    ),
    ...(candidate.placement.excludedSelector ?? []).map(
      (label) => `--placement-exclude-selector ${shellArgument(label)}`,
    ),
  ].join(" \\\n  ");
}

/** hsctl equivalent of a check run on an existing gateway. */
export function buildGetGatewayHsctlCommand(gatewayId: string): string {
  return `hsctl get gateway ${shellArgument(gatewayId)}`;
}

function failureKindOf(error: unknown): DocsRunFailureKind {
  return error instanceof GatewayOperationError ? error.kind : "unknown";
}

export type RunbookStepPhase = "done" | "failed" | "idle" | "running";

export interface RunbookGatewayStepViewProps {
  readonly availability:
    | { readonly state: "error" }
    | { readonly state: "loading" }
    | { readonly state: "ready"; readonly decision: RunbookPlacementDecision };
  readonly choice: string;
  readonly command: string;
  readonly existingGateways:
    | { readonly state: "error" }
    | { readonly state: "loading" }
    | { readonly state: "ready"; readonly gateways: readonly GatewayRecord[] };
  readonly gatewayName?: string;
  readonly hasTerminalEntry: boolean;
  readonly name: string;
  readonly onChoiceChange: (choice: string) => void;
  readonly onNameChange: (name: string) => void;
  readonly onOpenInTerminal: () => void;
  readonly onRecheck: () => void;
  readonly onRun: () => void;
  readonly output: RunbookCellOutput;
  readonly phase: RunbookStepPhase;
  readonly runbook: RunbookId;
  readonly session: DocsSessionState;
  readonly sessionExpired: boolean;
  readonly signInHref: string;
  readonly stage?: RunbookGatewayStage;
}

/**
 * Presentational Step 1 of a runbook (WEB-UDOC-06/07). The container supplies
 * every value, so stories and tests can render each state directly.
 */
export function RunbookGatewayStepView({
  availability,
  choice,
  command,
  existingGateways,
  gatewayName,
  hasTerminalEntry,
  name,
  onChoiceChange,
  onNameChange,
  onOpenInTerminal,
  onRecheck,
  onRun,
  output,
  phase,
  runbook,
  session,
  sessionExpired,
  signInHref: signInLink,
  stage,
}: RunbookGatewayStepViewProps) {
  const intl = useIntl();
  const choiceId = useId();
  const nameId = useId();
  const isVpn = runbook === "gitlab-jira";
  const isNew = choice === newGatewayChoice;
  const nameValid = isValidGatewayName(name);
  const isRunning = phase === "running";
  const isReady = stage === "running";
  const placementUnavailable =
    isNew &&
    availability.state === "ready" &&
    availability.decision.kind === "unavailable";
  const runDisabled =
    isRunning ||
    (isNew &&
      (!nameValid ||
        availability.state !== "ready" ||
        availability.decision.kind !== "available"));
  const signedIn = session === "signed-in";
  const eligibleGateways =
    existingGateways.state === "ready" ? existingGateways.gateways : [];

  let action: React.ReactNode;
  if (session === "signed-out") {
    action = (
      <Button component="a" href={signInLink} isInline variant="link">
        <FormattedMessage {...docsMessages.signInToRun} />
      </Button>
    );
  } else if (signedIn && isReady && phase !== "running") {
    action = (
      <Button onClick={onRecheck} variant="secondary">
        <FormattedMessage {...docsMessages.recheck} />
      </Button>
    );
  } else if (signedIn) {
    action = (
      <Button
        isAriaDisabled={runDisabled}
        isLoading={isRunning}
        onClick={onRun}
        variant="primary"
      >
        <FormattedMessage
          {...(isRunning ? docsMessages.running : docsMessages.run)}
        />
      </Button>
    );
  }

  return (
    <RunbookCell
      actions={
        <>
          {action}
          {hasTerminalEntry ? (
            <Button isInline onClick={onOpenInTerminal} variant="link">
              <FormattedMessage {...docsMessages.openInTerminal} />
            </Button>
          ) : null}
        </>
      }
      description={
        <FormattedMessage
          {...(isVpn
            ? docsMessages.stepVpnGatewayBody
            : docsMessages.stepGatewayBody)}
        />
      }
      mode="browser"
      output={output}
      stepNumber={1}
      title={
        <FormattedMessage
          {...(isVpn
            ? docsMessages.stepVpnGatewayTitle
            : docsMessages.stepGatewayTitle)}
        />
      }
    >
      {signedIn ? (
        <Form
          onSubmit={(event) => {
            event.preventDefault();
          }}
        >
          <FormGroup
            fieldId={choiceId}
            label={intl.formatMessage(docsMessages.gatewayChoiceLabel)}
          >
            {existingGateways.state === "loading" ? (
              <Skeleton
                screenreaderText={intl.formatMessage(
                  docsMessages.gatewayChoiceLoading,
                )}
              />
            ) : (
              <FormSelect
                id={choiceId}
                isDisabled={isRunning}
                onChange={(_event, value) => {
                  onChoiceChange(value);
                }}
                value={choice}
              >
                <FormSelectOption
                  label={intl.formatMessage(docsMessages.gatewayChoiceNew)}
                  value={newGatewayChoice}
                />
                {eligibleGateways.map((gateway) => (
                  <FormSelectOption
                    key={gateway.id}
                    label={gateway.name}
                    value={gateway.id}
                  />
                ))}
              </FormSelect>
            )}
            {existingGateways.state === "error" ? (
              <FormHelperText>
                <HelperText>
                  <HelperTextItem variant="warning">
                    <FormattedMessage {...docsMessages.gatewayChoiceError} />
                  </HelperTextItem>
                </HelperText>
              </FormHelperText>
            ) : null}
            {isVpn &&
            existingGateways.state === "ready" &&
            eligibleGateways.length === 0 ? (
              <FormHelperText>
                <HelperText>
                  <HelperTextItem>
                    <FormattedMessage {...docsMessages.noVpnGateways} />
                  </HelperTextItem>
                </HelperText>
              </FormHelperText>
            ) : null}
          </FormGroup>
          {isNew ? (
            <FormGroup
              fieldId={nameId}
              isRequired
              label={intl.formatMessage(docsMessages.gatewayNameLabel)}
            >
              <TextInput
                id={nameId}
                isDisabled={isRunning}
                isRequired
                onChange={(_event, value) => {
                  onNameChange(value);
                }}
                validated={nameValid ? "default" : "error"}
                value={name}
              />
              <FormHelperText>
                <HelperText>
                  <HelperTextItem variant={nameValid ? "default" : "error"}>
                    <FormattedMessage
                      {...(nameValid
                        ? docsMessages.gatewayNameHelp
                        : docsMessages.gatewayNameInvalid)}
                    />
                  </HelperTextItem>
                </HelperText>
              </FormHelperText>
            </FormGroup>
          ) : null}
        </Form>
      ) : null}
      {signedIn && isNew ? (
        <PlacementStatus availability={availability} runbook={runbook} />
      ) : null}
      {placementUnavailable ? null : (
        <DocsCommand
          command={command}
          label={intl.formatMessage(docsMessages.gatewayChoiceLabel)}
        />
      )}
      {sessionExpired && session === "signed-out" ? (
        <Alert
          isInline
          isPlain
          title={intl.formatMessage(docsMessages.sessionExpired)}
          variant="warning"
        />
      ) : null}
      <div aria-live="polite" className="pf-v6-screen-reader">
        {gatewayName && stage
          ? intl.formatMessage(docsMessages.stepStatusAnnouncement, {
              name: gatewayName,
              status: intl.formatMessage(stageMessages[stage]),
            })
          : ""}
      </div>
    </RunbookCell>
  );
}

function PlacementStatus({
  availability,
  runbook,
}: {
  availability: RunbookGatewayStepViewProps["availability"];
  runbook: RunbookId;
}) {
  const intl = useIntl();
  if (availability.state === "loading") {
    return (
      <HelperText>
        <HelperTextItem>
          <FormattedMessage {...docsMessages.placementChecking} />
        </HelperTextItem>
      </HelperText>
    );
  }
  if (availability.state === "error") {
    return (
      <Alert
        isInline
        isPlain
        title={intl.formatMessage(docsMessages.placementError)}
        variant="warning"
      />
    );
  }
  if (availability.decision.kind === "unavailable") {
    return (
      <Alert
        isInline
        isPlain
        title={intl.formatMessage(
          runbook === "gitlab-jira"
            ? docsMessages.vpnPlacementUnavailable
            : docsMessages.publicPlacementUnavailable,
        )}
        variant="info"
      />
    );
  }
  return (
    <HelperText>
      <HelperTextItem>
        <FormattedMessage
          {...docsMessages.placementTarget}
          values={{
            placement: intl.formatMessage(
              placementNames[availability.decision.candidate.id],
            ),
          }}
        />
      </HelperTextItem>
    </HelperText>
  );
}

const availabilityQueryRoot = ["gateway", "placement-availability"] as const;

/**
 * Step 1 container. It reads gateways and placement availability through the
 * gateway management application ports (WEB-PKG-04, UIS-HEX), never on page
 * load for a signed-out reader (WEB-UDOC-03), and sends a create request only
 * after the reader activates Run (WEB-UDOC-05).
 */
export function RunbookGatewayStep({
  onGatewayChange,
  operations = docsGatewayOperations,
  prefillName,
  runbook,
  session,
}: {
  onGatewayChange: (gateway: GatewayRecord | undefined) => void;
  operations?: GatewayOperations;
  prefillName?: string;
  runbook: RunbookId;
  session: DocsSessionState;
}) {
  const intl = useIntl();
  const location = useLocation();
  const queryClient = useQueryClient();
  const terminal = useTerminal();
  const trackConsoleWait = useConsoleWaitTracker();
  const signedIn = session === "signed-in";
  const route =
    runbook === "github-jira"
      ? ("/docs/runbooks/github-jira" as const)
      : ("/docs/runbooks/gitlab-jira" as const);
  useDocsSignInPromptShown(route, session === "signed-out");

  const generateName = useMemo(() => createDocsGatewayNameGenerator(), []);
  const [name, setName] = useState(() =>
    prefillName !== undefined && isValidGatewayName(prefillName)
      ? prefillName
      : generateName(),
  );
  const [choice, setChoice] = useState(newGatewayChoice);
  const [activeGatewayId, setActiveGatewayId] = useState<string>();
  const [phase, setPhase] = useState<RunbookStepPhase>("idle");
  const [lines, setLines] = useState<readonly string[]>([]);
  const [failed, setFailed] = useState(false);
  const [sessionExpired, setSessionExpired] = useState(false);
  const [terminalEntryId, setTerminalEntryId] = useState<string>();
  const entryRef = useRef<RunbookEntryHandle | undefined>(undefined);
  const lastStageRef = useRef<RunbookGatewayStage | undefined>(undefined);
  const actionRef = useRef<DocsRunbookStepAction>("create");
  // Each run and each choice change gets a new token, so a response that
  // arrives after the reader moved on never updates the cell.
  const runTokenRef = useRef(0);

  const gatewaysQuery = useQuery({
    enabled: signedIn,
    queryFn: ({ signal }) =>
      operations.listGateways(defaultGatewayListRequest, signal),
    queryKey: gatewayListQueryKey(defaultGatewayListRequest),
  });
  const needsCapabilities = runbookNeedsClusterCapabilities(runbook);
  const clusterIds = useMemo(
    () =>
      [
        ...new Set(
          (gatewaysQuery.data?.items ?? [])
            .map((gateway) => gateway.clusterId)
            .filter((clusterId) => clusterId !== ""),
        ),
      ].sort(),
    [gatewaysQuery.data],
  );
  const clustersQuery = useQuery({
    enabled: signedIn && needsCapabilities && clusterIds.length > 0,
    queryFn: ({ signal }) =>
      operations.getGatewayPlacements(clusterIds, signal),
    queryKey: gatewayPlacementBatchQueryKey(clusterIds),
  });
  const eligibleGateways = useMemo(() => {
    const capabilities = new Map<string, readonly string[]>(
      (clustersQuery.data ?? []).map((cluster) => [
        cluster.id,
        cluster.capabilities ?? [],
      ]),
    );
    return (gatewaysQuery.data?.items ?? []).filter((gateway) =>
      isGatewayEligibleForRunbook(runbook, gateway, capabilities),
    );
  }, [clustersQuery.data, gatewaysQuery.data, runbook]);

  const getAvailability =
    operations.getGatewayPlacementAvailability?.bind(operations);
  const availabilityQuery = useQuery({
    enabled: signedIn && choice === newGatewayChoice,
    queryFn: ({ signal }) => {
      if (getAvailability === undefined) {
        throw new GatewayOperationError("unknown");
      }
      return getAvailability(runbookAvailabilitySelectors(runbook), signal);
    },
    queryKey: [...availabilityQueryRoot, "docs", runbook],
    staleTime: 30_000,
  });
  const decision = availabilityQuery.data
    ? chooseRunbookPlacement(runbook, availabilityQuery.data)
    : undefined;

  const gatewayQuery = useQuery({
    enabled: signedIn && activeGatewayId !== undefined,
    queryFn: ({ signal }) =>
      operations.getGateway(activeGatewayId ?? "", signal),
    queryKey: gatewayQueryKey(activeGatewayId ?? ""),
    refetchInterval: ({ state }) => {
      if (!state.data) {
        return false;
      }
      return gatewayNeedsStatusPolling(
        state.data,
        trackConsoleWait(state.data.id, state.data),
      )
        ? gatewayStatusPollMilliseconds
        : false;
    },
    refetchIntervalInBackground: false,
  });
  const gateway = activeGatewayId === undefined ? undefined : gatewayQuery.data;
  const stage = gateway ? runbookGatewayStage(gateway) : undefined;

  useEffect(() => {
    onGatewayChange(gateway);
  }, [gateway, onGatewayChange]);

  const runActiveRef = useRef(false);

  const finishRun = useCallback(
    (succeeded: boolean, failureKind?: DocsRunFailureKind) => {
      setPhase(succeeded ? "done" : "failed");
      setFailed(!succeeded);
      entryRef.current?.finish({ exitCode: succeeded ? 0 : 1 });
      entryRef.current = undefined;
      if (!runActiveRef.current) {
        return;
      }
      runActiveRef.current = false;
      docsFacts.runbookStepRun(succeeded ? "succeeded" : "failed", {
        action: actionRef.current,
        runbook,
        ...(succeeded ? {} : { failureKind: failureKind ?? "unknown" }),
      });
    },
    [runbook],
  );

  // Lifecycle changes append to the output pane and are announced politely
  // without moving focus (WEB-UDOC-11). A create run ends when its gateway
  // settles in Running, Degraded, or Failed.
  const reportStage = useCallback(
    (record: GatewayRecord) => {
      const next = runbookGatewayStage(record);
      if (lastStageRef.current === next) {
        return;
      }
      lastStageRef.current = next;
      const line = intl.formatMessage(docsMessages.outputStatus, {
        status: intl.formatMessage(stageMessages[next]),
      });
      setLines((current) => [...current, line]);
      entryRef.current?.appendOutput([line]);
      if (next === "running") {
        finishRun(true);
      } else if (next === "failed" || next === "degraded") {
        finishRun(false, "unavailable");
      }
    },
    [finishRun, intl],
  );

  // Leaving the page mid-run finishes its terminal entry.
  useEffect(
    () => () => {
      const entry = entryRef.current;
      entryRef.current = undefined;
      entry?.finish({ exitCode: 130 });
    },
    [],
  );

  // Polled detail responses report lifecycle changes through a query cache
  // subscription, so the effect itself never sets state synchronously.
  useEffect(() => {
    if (activeGatewayId === undefined) {
      return;
    }
    const activeKey = JSON.stringify(gatewayQueryKey(activeGatewayId));
    return queryClient.getQueryCache().subscribe((event) => {
      if (
        event.type !== "updated" ||
        event.action.type !== "success" ||
        JSON.stringify(event.query.queryKey) !== activeKey
      ) {
        return;
      }
      const data = event.query.state.data as GatewayRecord | undefined;
      if (data) {
        reportStage(data);
      }
    });
  }, [activeGatewayId, queryClient, reportStage]);

  // Until availability answers (or for a signed-out reader), show the first
  // production placement rather than the local development one.
  const candidate =
    decision?.kind === "available"
      ? decision.candidate
      : (runbookPlacementCandidates[runbook].find(
          (item) => item.id !== "kind",
        ) ?? runbookPlacementCandidates[runbook][0]);
  const selectedExisting =
    choice === newGatewayChoice
      ? undefined
      : [...eligibleGateways, ...(gateway ? [gateway] : [])].find(
          (item) => item.id === choice,
        );
  const command =
    choice === newGatewayChoice || selectedExisting === undefined
      ? candidate === undefined
        ? ""
        : buildCreateGatewayHsctlCommand(name, candidate)
      : buildGetGatewayHsctlCommand(selectedExisting.id);

  const startRun = (
    action: DocsRunbookStepAction,
    firstLine: string,
  ): number => {
    runTokenRef.current += 1;
    actionRef.current = action;
    runActiveRef.current = true;
    lastStageRef.current = undefined;
    setPhase("running");
    setFailed(false);
    setSessionExpired(false);
    const entry = terminal.startRunbookEntry(command);
    entryRef.current = entry;
    setTerminalEntryId(entry.entryId);
    setLines([`$ ${command}`, firstLine]);
    entry.appendOutput([firstLine]);
    docsFacts.runbookStepRun("started", { action, runbook });
    return runTokenRef.current;
  };

  const failRun = (token: number, error: unknown) => {
    if (token !== runTokenRef.current) {
      return;
    }
    const kind = failureKindOf(error);
    const line = intl.formatMessage(docsMessages.outputFailed, {
      reason: intl.formatMessage(failureMessages[kind]),
    });
    setLines((current) => [...current, line]);
    setPhase("failed");
    setFailed(true);
    entryRef.current?.appendOutput([line], "stderr");
    finishRun(false, kind);
    if (kind === "denied") {
      // A denied request can mean the session expired; re-read the session so
      // the cell can offer sign-in instead of navigating away (WEB-UDOC-03).
      setSessionExpired(true);
      void queryClient.invalidateQueries({ queryKey: sessionQueryKey });
    }
  };

  const onRun = () => {
    if (choice === newGatewayChoice) {
      if (decision?.kind !== "available" || !isValidGatewayName(name)) {
        return;
      }
      const token = startRun(
        "create",
        intl.formatMessage(docsMessages.outputCreating, { name }),
      );
      void operations
        .provisionGateway({ name, placement: decision.candidate.placement })
        .then((created) => {
          if (token !== runTokenRef.current) {
            return;
          }
          const line = intl.formatMessage(docsMessages.outputCreated, {
            id: created.id,
            name: created.name,
          });
          setLines((current) => [...current, line]);
          entryRef.current?.appendOutput([line]);
          queryClient.setQueryData(gatewayQueryKey(created.id), created);
          void queryClient.invalidateQueries({
            queryKey: gatewayListQueryRoot,
          });
          setChoice(created.id);
          setActiveGatewayId(created.id);
          reportStage(created);
        })
        .catch((error: unknown) => {
          failRun(token, error);
        });
      return;
    }
    if (selectedExisting === undefined) {
      return;
    }
    const token = startRun(
      "check",
      intl.formatMessage(docsMessages.outputChecking, {
        name: selectedExisting.name,
      }),
    );
    const gatewayId = selectedExisting.id;
    void queryClient
      .fetchQuery({
        queryFn: ({ signal }) => operations.getGateway(gatewayId, signal),
        queryKey: gatewayQueryKey(gatewayId),
        staleTime: 0,
      })
      .then((record) => {
        if (token !== runTokenRef.current) {
          return;
        }
        setActiveGatewayId(gatewayId);
        reportStage(record);
        // A check reports the current status and ends, whatever the stage.
        if (runActiveRef.current) {
          finishRun(true);
        }
      })
      .catch((error: unknown) => {
        failRun(token, error);
      });
  };

  const onRecheck = () => {
    if (activeGatewayId === undefined || gateway === undefined) {
      return;
    }
    const token = startRun(
      "recheck",
      intl.formatMessage(docsMessages.outputChecking, { name: gateway.name }),
    );
    void gatewayQuery.refetch().then((result) => {
      if (token !== runTokenRef.current) {
        return;
      }
      if (result.isError) {
        failRun(token, result.error);
        return;
      }
      if (result.data) {
        reportStage(result.data);
      }
      if (runActiveRef.current) {
        finishRun(true);
      }
    });
  };

  // A gateway this step created or checked stays selectable even before it
  // has a cluster, so the choice always shows the gateway the step follows.
  const choiceGateways =
    gateway !== undefined &&
    !eligibleGateways.some((item) => item.id === gateway.id)
      ? [...eligibleGateways, gateway]
      : eligibleGateways;
  const existingGatewaysState: RunbookGatewayStepViewProps["existingGateways"] =
    gatewaysQuery.isError || (needsCapabilities && clustersQuery.isError)
      ? { state: "error" }
      : gatewaysQuery.isPending ||
          (needsCapabilities &&
            clusterIds.length > 0 &&
            clustersQuery.isPending)
        ? { state: "loading" }
        : { gateways: choiceGateways, state: "ready" };

  return (
    <RunbookGatewayStepView
      availability={
        availabilityQuery.isError
          ? { state: "error" }
          : decision === undefined
            ? { state: "loading" }
            : { decision, state: "ready" }
      }
      choice={choice}
      command={command}
      existingGateways={existingGatewaysState}
      gatewayName={gateway?.name}
      hasTerminalEntry={terminalEntryId !== undefined}
      name={name}
      onChoiceChange={(next) => {
        // A new choice starts over: no response of an earlier run applies.
        runTokenRef.current += 1;
        runActiveRef.current = false;
        lastStageRef.current = undefined;
        entryRef.current?.finish({ exitCode: 130 });
        entryRef.current = undefined;
        setChoice(next);
        setActiveGatewayId(undefined);
        setPhase("idle");
        setFailed(false);
        setLines([]);
        setSessionExpired(false);
        setTerminalEntryId(undefined);
      }}
      onNameChange={setName}
      onOpenInTerminal={() => {
        terminal.open(
          terminalEntryId === undefined ? {} : { entryId: terminalEntryId },
        );
      }}
      onRecheck={onRecheck}
      onRun={onRun}
      output={{ failed, lines }}
      phase={phase}
      runbook={runbook}
      session={session}
      sessionExpired={sessionExpired}
      signInHref={signInHref(currentPathAndQuery(location))}
      {...(stage === undefined ? {} : { stage })}
    />
  );
}
