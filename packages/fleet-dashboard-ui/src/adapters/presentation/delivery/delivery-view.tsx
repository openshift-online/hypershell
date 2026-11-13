// Glass Delivery tab (fleet-dashboard-delivery-pipeline.spec FLT-DLV-01..16).
// DeliveryView is presentational: it renders one plane state, the selected change,
// and a reference instant, and reports navigation as typed actions. DeliveryTab
// wires it to the query hook and the router.

import {
  ActionList,
  ActionListItem,
  Alert,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Content,
  DataList,
  DataListCell,
  DataListItem,
  DataListItemCells,
  DataListItemRow,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Drawer,
  DrawerActions,
  DrawerCloseButton,
  DrawerContent,
  DrawerContentBody,
  DrawerHead,
  DrawerPanelBody,
  DrawerPanelContent,
  EmptyState,
  EmptyStateBody,
  Flex,
  FlexItem,
  Icon,
  List,
  ListItem,
  PageSection,
  ProgressStep,
  ProgressStepper,
  Spinner,
  Stack,
  StackItem,
  Title,
  Truncate,
} from "@patternfly/react-core";
import {
  CheckCircleIcon,
  CubesIcon,
  ExclamationCircleIcon,
  ExclamationTriangleIcon,
  ExternalLinkAltIcon,
  InProgressIcon,
  PendingIcon,
  QuestionCircleIcon,
  RedoIcon,
  SyncAltIcon,
} from "@patternfly/react-icons";
import {
  InnerScrollContainer,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Tr,
} from "@patternfly/react-table";
import { useEffect, useState } from "react";
import type { MessageDescriptor } from "react-intl";
import { useIntl, type IntlShape } from "react-intl";

import {
  SOURCE_STAGES,
  changeListUnavailable,
  degradedSources,
  elapsedMs,
  eventfulHistory,
  findChange,
  groupLabel,
  isSlow,
  nowAndNext,
  typicalMs,
  type DeliveryChange,
  type DeliveryCheck,
  type DeliveryData,
  type DeliveryEnvState,
  type DeliveryPromotion,
  type DeliverySeverity,
  type DeliverySourceKey,
  type DeliveryStage,
  type DeliveryStageDef,
  type DeliveryStageState,
  type NowNextRow,
  type NowNextSummary,
  type PromotionOutcome,
} from "../../../domain/delivery";
import { identiName } from "../../../domain/map/identiname";
import type { NavAction } from "../../../domain/navigation/actions";
import type { Plane } from "../../../domain/plane";
import { useDelivery } from "../../query/hooks";
import { useRouter } from "../../routing/router";
import { deliveryMessages as m } from "./delivery-messages";
import styles from "./delivery-view.module.css";

/** One plane state the view renders. */
export type DeliveryViewState =
  | { readonly status: "loading" }
  | { readonly status: "error" }
  | { readonly status: "ready"; readonly plane: Plane<DeliveryData> };

export interface DeliveryViewProps {
  readonly state: DeliveryViewState;
  readonly selectedSha: string | null;
  readonly nowMs: number;
  readonly onNavigate: (action: NavAction) => void;
  readonly onRefresh: () => void;
}

// --- Formatting -------------------------------------------------------------

const STATE_LABEL: Readonly<Record<DeliveryStageState, MessageDescriptor>> = {
  "not-started": m.stateNotStarted,
  waiting: m.stateWaiting,
  running: m.stateRunning,
  passed: m.statePassed,
  failed: m.stateFailed,
  "not-needed": m.stateNotNeeded,
  unknown: m.stateUnknown,
};

const PIPELINE_LABEL: Readonly<Record<string, MessageDescriptor>> = {
  merged: m.stageMerged,
  build: m.stageBuild,
  policy: m.stagePolicy,
  release: m.stageRelease,
  gitops: m.stageGitops,
};

const SOURCE_LABEL: Readonly<Record<DeliverySourceKey, MessageDescriptor>> = {
  upstream: m.sourceUpstream,
  registry: m.sourceRegistry,
  gitops: m.sourceGitops,
  promotion: m.sourcePromotion,
  history: m.sourceHistory,
};

function formatDuration(intl: IntlShape, ms: number): string {
  const minutes = Math.floor(ms / 60_000);
  if (minutes < 1) {
    return intl.formatMessage(m.durationLessThanMinute);
  }
  if (minutes < 60) {
    return intl.formatMessage(m.durationMinutes, { minutes });
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 48) {
    return intl.formatMessage(m.durationHours, {
      hours,
      minutes: minutes % 60,
    });
  }
  return intl.formatMessage(m.durationDays, {
    days: Math.floor(hours / 24),
    hours: hours % 24,
  });
}

function formatClock(intl: IntlShape, iso: string | number | null): string {
  if (iso === null) {
    return "";
  }
  const ms = typeof iso === "number" ? iso : Date.parse(iso);
  if (Number.isNaN(ms)) {
    return "";
  }
  return intl.formatTime(ms, {
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  });
}

function stageLabel(
  intl: IntlShape,
  def: Pick<DeliveryStageDef, "key" | "group">,
): string {
  const descriptor = PIPELINE_LABEL[def.key];
  if (descriptor) {
    return intl.formatMessage(descriptor);
  }
  return groupLabel(def.group ?? def.key.replace(/^env:/, ""));
}

function gateName(intl: IntlShape, gate: string | null): string {
  if (gate === null) {
    return "";
  }
  if (gate.includes("analysis")) {
    return intl.formatMessage(m.gateAnalysis);
  }
  if (gate.includes("health")) {
    return intl.formatMessage(m.gateHealth);
  }
  return gate;
}

/** The condition text of a stage, without times. */
function stageDetail(intl: IntlShape, stage: DeliveryStage): string {
  switch (stage.reason) {
    case "no-component-changed":
      return intl.formatMessage(m.reasonNoComponentChanged);
    case "build-replaced":
      return intl.formatMessage(m.reasonBuildReplaced);
    case "waiting-policy":
      return intl.formatMessage(m.reasonWaitingPolicy);
    case "next-bundle":
      return intl.formatMessage(m.reasonNextBundle);
    case "waiting-bundle-pr":
      return intl.formatMessage(m.reasonWaitingBundlePR);
    case "waiting-hydration":
      return intl.formatMessage(m.reasonWaitingHydration);
    case "waiting-groups":
      return intl.formatMessage(m.reasonWaitingGroups, {
        groups: intl.formatList(stage.groups.map(groupLabel)),
      });
    case "gate-running":
      return intl.formatMessage(m.reasonGateRunning, {
        gate: gateName(intl, stage.gate),
      });
    case "gate-failed":
      return intl.formatMessage(m.reasonGateFailed, {
        gate: gateName(intl, stage.gate),
      });
    case "partial":
      return "";
    default:
      break;
  }
  if (stage.pr !== null) {
    const pr = intl.formatMessage(m.pullRequest, { number: stage.pr });
    const checks =
      stage.checks === "running"
        ? intl.formatMessage(m.checksRunning)
        : stage.checks === "passed"
          ? intl.formatMessage(m.checksPassed)
          : stage.checks === "failed"
            ? intl.formatMessage(m.checksFailed)
            : "";
    return checks ? `${pr}, ${checks}` : pr;
  }
  return stage.components.join(", ");
}

/** The leading time or count of a stage cell. */
function stagePrimary(
  intl: IntlShape,
  stage: DeliveryStage,
  nowMs: number,
): string {
  if (stage.reason === "partial") {
    return intl.formatMessage(m.partial, {
      done: stage.done,
      total: stage.total,
    });
  }
  const elapsed = elapsedMs(stage, nowMs);
  if (elapsed !== null) {
    return formatDuration(intl, elapsed);
  }
  if (stage.state === "passed" || stage.state === "failed") {
    return formatClock(intl, stage.finishedAt);
  }
  return "";
}

// --- Cells ------------------------------------------------------------------

function StageStateIcon({ state }: { readonly state: DeliveryStageState }) {
  switch (state) {
    case "running":
      return (
        <Icon isInline status="info">
          <InProgressIcon aria-hidden />
        </Icon>
      );
    case "passed":
      return (
        <Icon isInline status="success">
          <CheckCircleIcon aria-hidden />
        </Icon>
      );
    case "failed":
      return (
        <Icon isInline status="danger">
          <ExclamationCircleIcon aria-hidden />
        </Icon>
      );
    case "waiting":
      return (
        <Icon isInline>
          <PendingIcon aria-hidden />
        </Icon>
      );
    case "unknown":
      return (
        <Icon isInline>
          <QuestionCircleIcon aria-hidden />
        </Icon>
      );
    default:
      return null;
  }
}

function SlowerThanUsual() {
  const intl = useIntl();
  return (
    <Flex
      spaceItems={{ default: "spaceItemsXs" }}
      flexWrap={{ default: "nowrap" }}
    >
      <FlexItem>
        <Icon isInline status="warning">
          <ExclamationTriangleIcon aria-hidden />
        </Icon>
      </FlexItem>
      <FlexItem>{intl.formatMessage(m.slower)}</FlexItem>
    </Flex>
  );
}

// FLT-DLV-04: every state is an icon paired with a text label.
function StageStatus({
  stage,
  slow,
  nowMs,
}: {
  readonly stage: DeliveryStage;
  readonly slow: boolean;
  readonly nowMs: number;
}) {
  const intl = useIntl();
  const muted =
    stage.state === "not-started" ||
    stage.state === "not-needed" ||
    stage.state === "unknown";
  const label =
    stage.reason === "inferred-release"
      ? intl.formatMessage(m.stateReleasingInferred)
      : intl.formatMessage(STATE_LABEL[stage.state]);
  const secondary = [stagePrimary(intl, stage, nowMs), stageDetail(intl, stage)]
    .filter((s) => s !== "")
    .join(" - ");
  return (
    <Stack>
      <StackItem className="pf-v6-u-text-nowrap">
        <span className={muted ? "pf-v6-u-text-color-subtle" : undefined}>
          <StageStateIcon state={stage.state} /> {label}
        </span>
      </StackItem>
      {secondary ? (
        <StackItem className="pf-v6-u-font-size-sm pf-v6-u-text-color-subtle">
          {secondary}
        </StackItem>
      ) : null}
      {slow ? (
        <StackItem className="pf-v6-u-font-size-sm">
          <SlowerThanUsual />
        </StackItem>
      ) : null}
      {stage.failedAttempts > 0 ? (
        <StackItem className="pf-v6-u-font-size-sm">
          {/* FLT-DLV-18: a Passed group that needed more than one bundle. */}
          <Flex
            spaceItems={{ default: "spaceItemsXs" }}
            flexWrap={{ default: "nowrap" }}
          >
            <FlexItem>
              <Icon isInline status="warning">
                <ExclamationTriangleIcon aria-hidden />
              </Icon>
            </FlexItem>
            <FlexItem>
              {intl.formatMessage(m.failedAttempts, {
                count: stage.failedAttempts,
              })}
            </FlexItem>
          </Flex>
        </StackItem>
      ) : null}
    </Stack>
  );
}

const OUTCOME_LABEL: Readonly<Record<PromotionOutcome, MessageDescriptor>> = {
  passed: m.statePassed,
  failed: m.stateFailed,
  replaced: m.outcomeReplaced,
  running: m.stateRunning,
  unknown: m.stateUnknown,
};

function OutcomeIcon({ outcome }: { readonly outcome: PromotionOutcome }) {
  if (outcome === "replaced") {
    return (
      <Icon isInline>
        <RedoIcon aria-hidden />
      </Icon>
    );
  }
  return <StageStateIcon state={outcome} />;
}

function attemptDetail(intl: IntlShape, a: DeliveryPromotion): string {
  if (a.gate === null) {
    return "";
  }
  const gate = gateName(intl, a.gate);
  switch (a.outcome) {
    case "failed":
      return intl.formatMessage(m.attemptGateFailed, { gate });
    case "replaced":
      return intl.formatMessage(m.attemptGateRunning, { gate });
    case "running":
      return intl.formatMessage(m.attemptGateRunningNow, { gate });
    default:
      return "";
  }
}

// FLT-DLV-18: what happened in one environment, oldest promotion first.
function PromotionHistory({ env }: { readonly env: DeliveryEnvState }) {
  const intl = useIntl();
  return (
    <List
      aria-label={intl.formatMessage(m.promotionHistoryLabel, { env: env.env })}
      className="pf-v6-u-font-size-sm pf-v6-u-mt-xs"
      isPlain
    >
      {env.history.map((a) => {
        const values = {
          outcome: intl.formatMessage(OUTCOME_LABEL[a.outcome]),
          // The Fleet Map's bundle name, so two bundles of one day differ.
          bundle: a.digest ? identiName(a.digest) : a.version,
          time: formatClock(intl, a.promotedAt),
        };
        const gitops =
          a.gitopsPr !== null
            ? intl.formatMessage(m.attemptGitops, { number: a.gitopsPr })
            : null;
        const text =
          gitops !== null
            ? intl.formatMessage(m.attempt, { ...values, gitops })
            : intl.formatMessage(m.attemptNoGitops, values);
        const detail = attemptDetail(intl, a);
        const suffix = detail ? ` - ${detail}` : "";
        return (
          <ListItem key={`${a.digest}-${a.promotedAt}`}>
            <Flex
              spaceItems={{ default: "spaceItemsXs" }}
              flexWrap={{ default: "nowrap" }}
              alignItems={{ default: "alignItemsBaseline" }}
            >
              <FlexItem>
                <OutcomeIcon outcome={a.outcome} />
              </FlexItem>
              <FlexItem>
                {a.url !== null ? (
                  <ExternalLink href={a.url}>{text}</ExternalLink>
                ) : (
                  text
                )}
                {suffix ? (
                  <span className="pf-v6-u-text-color-subtle">{suffix}</span>
                ) : null}
              </FlexItem>
            </Flex>
          </ListItem>
        );
      })}
    </List>
  );
}

function SeverityIcon({ severity }: { readonly severity: DeliverySeverity }) {
  switch (severity) {
    case "danger":
      return (
        <Icon status="danger">
          <ExclamationCircleIcon aria-hidden />
        </Icon>
      );
    case "warning":
      return (
        <Icon status="warning">
          <ExclamationTriangleIcon aria-hidden />
        </Icon>
      );
    case "good":
      return (
        <Icon status="success">
          <CheckCircleIcon aria-hidden />
        </Icon>
      );
    default:
      return (
        <Icon status="info">
          <InProgressIcon aria-hidden />
        </Icon>
      );
  }
}

function changeName(change: Pick<DeliveryChange, "shortSha" | "pr">): string {
  return change.pr !== null
    ? `${change.shortSha} (#${String(change.pr)})`
    : change.shortSha;
}

// --- Now and next (FLT-DLV-10) ---------------------------------------------

function nowText(intl: IntlShape, data: DeliveryData, row: NowNextRow): string {
  const { stage } = row.now;
  const elapsed =
    row.now.elapsedMs !== null
      ? ` - ${formatDuration(intl, row.now.elapsedMs)}`
      : "";
  const typical =
    row.now.typicalMs !== null &&
    (stage.state === "running" || stage.state === "waiting")
      ? `, ${intl.formatMessage(m.usually, { duration: formatDuration(intl, row.now.typicalMs) })}`
      : "";
  if (
    stage.key === "build" &&
    stage.state === "running" &&
    stage.components.length > 0
  ) {
    return intl.formatMessage(m.nowBuilding, {
      components: stage.components.join(", "),
      elapsed,
      typical,
    });
  }
  if (stage.key === "build" && stage.state === "failed") {
    const at = formatClock(intl, stage.finishedAt);
    return intl.formatMessage(m.nowBuildFailed, {
      components: stage.components.join(", "),
      at: at ? ` at ${at}` : "",
    });
  }
  const def = data.stages.find((d) => d.key === stage.key);
  const state =
    stage.reason === "inferred-release"
      ? intl.formatMessage(m.stateReleasingInferred)
      : intl.formatMessage(STATE_LABEL[stage.state]).toLowerCase();
  const detail = stageDetail(intl, stage);
  const partial =
    stage.reason === "partial"
      ? intl.formatMessage(m.partial, { done: stage.done, total: stage.total })
      : "";
  const extra = [partial, detail].filter((s) => s !== "").join(", ");
  return intl.formatMessage(m.nowText, {
    stage: stageLabel(intl, def ?? { key: stage.key, group: null }),
    state,
    detail: extra ? ` (${extra})` : "",
    elapsed,
    typical,
  });
}

function nextText(
  intl: IntlShape,
  data: DeliveryData,
  row: NowNextRow,
): string {
  const next = row.next;
  switch (next.kind) {
    case "restart":
      return intl.formatMessage(
        next.restart === "new-change" ? m.nextRestartBuild : m.nextRestartGate,
      );
    case "none":
      return intl.formatMessage(m.nextNone);
    default: {
      const def = data.stages.find((d) => d.key === next.stage.key);
      const stage = stageLabel(
        intl,
        def ?? { key: next.stage.key, group: null },
      );
      return next.expectedAtMs !== null
        ? intl.formatMessage(m.nextStageAt, {
            stage,
            time: formatClock(intl, next.expectedAtMs),
          })
        : stage;
    }
  }
}

/** Joins non-empty text parts with the cell separator used across the tab. */
function joinParts(parts: readonly string[]): string {
  return parts.filter((p) => p !== "").join(" - ");
}

function deliveredText(
  intl: IntlShape,
  delivered: NonNullable<NowNextSummary["delivered"]>,
): string {
  const detail =
    delivered.deliveredAtMs !== null && delivered.totalMs !== null
      ? intl.formatMessage(m.allDeliveredDetail, {
          change: changeName(delivered.change),
          time: formatClock(intl, delivered.deliveredAtMs),
          total: formatDuration(intl, delivered.totalMs),
        })
      : intl.formatMessage(m.allDeliveredNoTime, {
          change: changeName(delivered.change),
        });
  return [intl.formatMessage(m.allDelivered), detail].join(" ");
}

function NowAndNext({
  data,
  nowMs,
}: {
  readonly data: DeliveryData;
  readonly nowMs: number;
}) {
  const intl = useIntl();
  const summary = nowAndNext(data, nowMs);
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <Flex
            alignItems={{ default: "alignItemsCenter" }}
            spaceItems={{ default: "spaceItemsSm" }}
          >
            <FlexItem>
              <SeverityIcon severity={summary.severity} />
            </FlexItem>
            <FlexItem>
              <Title headingLevel="h2" size="lg">
                {intl.formatMessage(m.nowNextTitle)}
              </Title>
            </FlexItem>
          </Flex>
        </CardTitle>
      </CardHeader>
      <CardBody>
        {summary.delivered !== null ? (
          <Content component="p">
            {deliveredText(intl, summary.delivered)}
          </Content>
        ) : (
          <Stack hasGutter>
            <StackItem>
              <DataList
                aria-label={intl.formatMessage(m.nowNextList)}
                isCompact
              >
                {summary.rows.map((row) => (
                  <DataListItem
                    aria-labelledby={`now-next-${row.change.sha}`}
                    key={row.change.sha}
                  >
                    <DataListItemRow>
                      <DataListItemCells
                        dataListCells={[
                          <DataListCell key="change" width={2}>
                            <Content
                              className="pf-v6-u-font-family-monospace"
                              component="p"
                              id={`now-next-${row.change.sha}`}
                            >
                              {changeName(row.change)}
                            </Content>
                            <Content
                              className="pf-v6-u-text-color-subtle"
                              component="small"
                            >
                              <Truncate content={row.change.title} />
                            </Content>
                          </DataListCell>,
                          <DataListCell key="now" width={3}>
                            <Content component="p">
                              <strong>{intl.formatMessage(m.now)}</strong>
                            </Content>
                            <Flex spaceItems={{ default: "spaceItemsSm" }}>
                              <FlexItem>
                                {row.severity === "info" ? (
                                  <StageStateIcon state={row.now.stage.state} />
                                ) : (
                                  <SeverityIcon severity={row.severity} />
                                )}
                              </FlexItem>
                              <FlexItem flex={{ default: "flex_1" }}>
                                {nowText(intl, data, row)}
                              </FlexItem>
                            </Flex>
                          </DataListCell>,
                          <DataListCell key="next" width={3}>
                            <Content component="p">
                              <strong>{intl.formatMessage(m.next)}</strong>
                            </Content>
                            {nextText(intl, data, row)}
                          </DataListCell>,
                        ]}
                      />
                    </DataListItemRow>
                  </DataListItem>
                ))}
              </DataList>
            </StackItem>
            {summary.more > 0 ? (
              <StackItem>
                <Button
                  component="a"
                  href="#delivery-changes"
                  isInline
                  variant="link"
                >
                  {intl.formatMessage(m.moreInTable, { count: summary.more })}
                </Button>
              </StackItem>
            ) : null}
          </Stack>
        )}
      </CardBody>
    </Card>
  );
}

// --- Change table (FLT-DLV-03) ----------------------------------------------

function ChangeTable({
  data,
  selectedSha,
  nowMs,
  onNavigate,
}: {
  readonly data: DeliveryData;
  readonly selectedSha: string | null;
  readonly nowMs: number;
  readonly onNavigate: (action: NavAction) => void;
}) {
  const intl = useIntl();
  const columnLabel = intl.formatMessage(m.columnChange);
  return (
    <Card id="delivery-changes">
      <CardHeader>
        <CardTitle>
          <Title headingLevel="h2" size="lg">
            {intl.formatMessage(m.tableTitle)}
          </Title>
        </CardTitle>
      </CardHeader>
      <CardBody>
        <Stack hasGutter>
          {data.omitted > 0 ? (
            <StackItem>
              <Content className="pf-v6-u-text-color-subtle" component="small">
                {intl.formatMessage(m.omitted, { count: data.omitted })}
              </Content>
            </StackItem>
          ) : null}
          <StackItem>
            <InnerScrollContainer>
              <Table aria-label={intl.formatMessage(m.tableLabel)}>
                <Thead>
                  <Tr>
                    <Th hasRightBorder isStickyColumn stickyMinWidth="18rem">
                      {columnLabel}
                    </Th>
                    {data.stages.map((def) => (
                      <Th key={def.key} modifier="nowrap">
                        {stageLabel(intl, def)}
                      </Th>
                    ))}
                  </Tr>
                </Thead>
                <Tbody>
                  {data.changes.map((change) => (
                    <Tr
                      isClickable
                      isRowSelected={change.sha === selectedSha}
                      key={change.sha}
                      onRowClick={() => {
                        onNavigate({
                          type: "deliverySelectChange",
                          sha: change.sha,
                        });
                      }}
                    >
                      <Td
                        dataLabel={columnLabel}
                        hasRightBorder
                        isStickyColumn
                        stickyMinWidth="18rem"
                      >
                        <Button
                          className="pf-v6-u-font-family-monospace"
                          isInline
                          variant="link"
                          onClick={(event) => {
                            event.stopPropagation();
                            onNavigate({
                              type: "deliverySelectChange",
                              sha: change.sha,
                            });
                          }}
                        >
                          {changeName(change)}
                        </Button>
                        <Content
                          className="pf-v6-u-text-color-subtle"
                          component="small"
                        >
                          <Truncate content={change.title} />
                        </Content>
                        {change.author !== null ? (
                          <Content
                            className="pf-v6-u-text-color-subtle"
                            component="small"
                          >
                            {intl.formatMessage(m.byAuthor, {
                              author: change.author,
                            })}
                          </Content>
                        ) : null}
                      </Td>
                      {data.stages.map((def) => {
                        const stage = change.stages.find(
                          (s) => s.key === def.key,
                        );
                        return (
                          <Td dataLabel={stageLabel(intl, def)} key={def.key}>
                            {stage ? (
                              <StageStatus
                                stage={stage}
                                slow={isSlow(data, stage, nowMs)}
                                nowMs={nowMs}
                              />
                            ) : null}
                          </Td>
                        );
                      })}
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            </InnerScrollContainer>
          </StackItem>
        </Stack>
      </CardBody>
    </Card>
  );
}

// --- Details drawer (FLT-DLV-11) --------------------------------------------

const STEP_VARIANT: Readonly<
  Record<
    DeliveryStageState,
    "default" | "success" | "info" | "pending" | "danger"
  >
> = {
  "not-started": "pending",
  waiting: "pending",
  running: "info",
  passed: "success",
  failed: "danger",
  "not-needed": "default",
  unknown: "default",
};

function ExternalLink({
  href,
  children,
}: {
  readonly href: string;
  readonly children: React.ReactNode;
}) {
  return (
    <Button
      component="a"
      href={href}
      icon={<ExternalLinkAltIcon />}
      iconPosition="end"
      isInline
      rel="noopener noreferrer"
      target="_blank"
      variant="link"
    >
      {children}
    </Button>
  );
}

function stepDescription(
  intl: IntlShape,
  data: DeliveryData,
  stage: DeliveryStage,
  nowMs: number,
): string {
  const detail = stageDetail(intl, stage);
  if (
    stage.state === "passed" &&
    stage.startedAt !== null &&
    stage.finishedAt !== null
  ) {
    const ms = Date.parse(stage.finishedAt) - Date.parse(stage.startedAt);
    const range = intl.formatMessage(m.stepRange, {
      start: formatClock(intl, stage.startedAt),
      end: formatClock(intl, stage.finishedAt),
      duration: formatDuration(intl, Math.max(0, ms)),
    });
    return detail ? `${range} - ${detail}` : range;
  }
  if (stage.state === "passed") {
    return [formatClock(intl, stage.finishedAt), detail]
      .filter((s) => s !== "")
      .join(" - ");
  }
  const parts = [stagePrimary(intl, stage, nowMs), detail].filter(
    (s) => s !== "",
  );
  const typical = typicalMs(data, stage.key);
  if (
    (stage.state === "running" || stage.state === "waiting") &&
    typical !== null
  ) {
    parts.push(
      intl.formatMessage(m.usually, {
        duration: formatDuration(intl, typical),
      }),
    );
  }
  return parts.join(" - ");
}

function checkStateText(
  intl: IntlShape,
  check: DeliveryCheck | undefined,
): string {
  return check
    ? intl.formatMessage(STATE_LABEL[check.state])
    : intl.formatMessage(STATE_LABEL["not-started"]);
}

function ChangeDetails({
  data,
  change,
  nowMs,
  onNavigate,
}: {
  readonly data: DeliveryData;
  readonly change: DeliveryChange;
  readonly nowMs: number;
  readonly onNavigate: (action: NavAction) => void;
}) {
  const intl = useIntl();
  const groups = Array.from(new Set(change.environments.map((e) => e.group)));
  const components = Array.from(new Set(change.builds.map((b) => b.component)));
  const bundle = change.bundle;
  return (
    <DrawerPanelContent widths={{ default: "width_50", xl: "width_33" }}>
      <DrawerHead>
        <Title headingLevel="h2" size="xl">
          <span className="pf-v6-u-font-family-monospace">
            {changeName(change)}
          </span>
        </Title>
        <Content component="p">{change.title}</Content>
        {change.author !== null ? (
          <Content className="pf-v6-u-text-color-subtle" component="small">
            {intl.formatMessage(m.byAuthor, { author: change.author })}
          </Content>
        ) : null}
        <DrawerActions>
          <DrawerCloseButton
            onClick={() => {
              onNavigate({ type: "deliverySelectChange", sha: null });
            }}
          />
        </DrawerActions>
      </DrawerHead>
      <DrawerPanelBody>
        <Stack hasGutter>
          <StackItem>
            <Flex spaceItems={{ default: "spaceItemsMd" }}>
              {change.prUrl !== null ? (
                <FlexItem>
                  <ExternalLink href={change.prUrl}>
                    {intl.formatMessage(m.viewPR)}
                  </ExternalLink>
                </FlexItem>
              ) : null}
              {change.commitUrl !== null ? (
                <FlexItem>
                  <ExternalLink href={change.commitUrl}>
                    {intl.formatMessage(m.viewCommit)}
                  </ExternalLink>
                </FlexItem>
              ) : null}
            </Flex>
          </StackItem>
          <StackItem>
            <Title headingLevel="h3" size="md">
              {intl.formatMessage(m.drawerStages)}
            </Title>
            <ProgressStepper
              aria-label={intl.formatMessage(m.drawerStagesLabel)}
              isVertical
            >
              {change.stages.map((stage) => {
                const def = data.stages.find((d) => d.key === stage.key);
                const label = stageLabel(
                  intl,
                  def ?? { key: stage.key, group: null },
                );
                const id = `delivery-step-${stage.key.replace(/[^a-z0-9-]/gi, "-")}`;
                return (
                  <ProgressStep
                    description={stepDescription(intl, data, stage, nowMs)}
                    id={id}
                    isCurrent={stage.state === "running"}
                    key={stage.key}
                    titleId={`${id}-title`}
                    variant={STEP_VARIANT[stage.state]}
                  >
                    {label}
                  </ProgressStep>
                );
              })}
            </ProgressStepper>
          </StackItem>
          <StackItem>
            <Title headingLevel="h3" size="md">
              {intl.formatMessage(m.drawerChecks)}
            </Title>
            {components.length === 0 ? (
              <Content component="p">{intl.formatMessage(m.noChecks)}</Content>
            ) : (
              <DescriptionList isCompact isHorizontal>
                {components.map((component) => {
                  const build = change.builds.find(
                    (b) => b.component === component,
                  );
                  const policy = change.policies.find(
                    (p) => p.component === component,
                  );
                  const url = build?.url ?? policy?.url ?? null;
                  return (
                    <DescriptionListGroup key={component}>
                      <DescriptionListTerm>{component}</DescriptionListTerm>
                      <DescriptionListDescription>
                        <Stack>
                          <StackItem>
                            {intl.formatMessage(m.checkLine, {
                              build: checkStateText(intl, build),
                              policy: checkStateText(intl, policy),
                            })}
                          </StackItem>
                          {url !== null ? (
                            <StackItem>
                              <ExternalLink href={url}>
                                {intl.formatMessage(m.viewCheckRuns)}
                              </ExternalLink>
                            </StackItem>
                          ) : null}
                        </Stack>
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                  );
                })}
              </DescriptionList>
            )}
          </StackItem>
          <StackItem>
            <Title headingLevel="h3" size="md">
              {intl.formatMessage(m.drawerBundle)}
            </Title>
            {bundle === null ? (
              <Content component="p">
                {intl.formatMessage(
                  change.stages.find((st) => st.key === "release")?.state ===
                    "unknown"
                    ? m.bundleUnknown
                    : m.bundleNone,
                )}
              </Content>
            ) : (
              <DescriptionList isCompact isHorizontal>
                <DescriptionListGroup>
                  <DescriptionListTerm>
                    {intl.formatMessage(m.bundleAlias)}
                  </DescriptionListTerm>
                  <DescriptionListDescription>
                    {joinParts([
                      bundle.version,
                      intl.formatMessage(
                        bundle.locked ? m.bundleLocked : m.bundlePending,
                      ),
                    ])}
                  </DescriptionListDescription>
                </DescriptionListGroup>
                <DescriptionListGroup>
                  <DescriptionListTerm>
                    {intl.formatMessage(m.bundleTag)}
                  </DescriptionListTerm>
                  <DescriptionListDescription>
                    <Truncate content={bundle.tag} position="middle" />
                  </DescriptionListDescription>
                </DescriptionListGroup>
                <DescriptionListGroup>
                  <DescriptionListTerm>
                    {intl.formatMessage(m.bundleDigest)}
                  </DescriptionListTerm>
                  <DescriptionListDescription>
                    <Truncate content={bundle.digest} position="middle" />
                  </DescriptionListDescription>
                </DescriptionListGroup>
                <DescriptionListGroup>
                  <DescriptionListTerm>
                    {intl.formatMessage(m.bundlePublished)}
                  </DescriptionListTerm>
                  <DescriptionListDescription>
                    <Stack>
                      {bundle.publishedAt !== null ? (
                        <StackItem>
                          {formatClock(intl, bundle.publishedAt)}
                        </StackItem>
                      ) : null}
                      <StackItem>
                        {bundle.releaseUrl !== null ? (
                          <ExternalLink href={bundle.releaseUrl}>
                            {intl.formatMessage(m.downloads)}
                          </ExternalLink>
                        ) : (
                          intl.formatMessage(m.downloadsPending)
                        )}
                      </StackItem>
                    </Stack>
                  </DescriptionListDescription>
                </DescriptionListGroup>
                {change.gitopsPr !== null ? (
                  <DescriptionListGroup>
                    <DescriptionListTerm>
                      {intl.formatMessage(m.gitopsUpdate)}
                    </DescriptionListTerm>
                    <DescriptionListDescription>
                      {change.gitopsPr.url !== null ? (
                        <ExternalLink href={change.gitopsPr.url}>
                          {intl.formatMessage(m.pullRequest, {
                            number: change.gitopsPr.number,
                          })}
                        </ExternalLink>
                      ) : (
                        intl.formatMessage(m.pullRequest, {
                          number: change.gitopsPr.number,
                        })
                      )}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                ) : null}
              </DescriptionList>
            )}
          </StackItem>
          {groups.length > 0 ? (
            <StackItem>
              <Title headingLevel="h3" size="md">
                {intl.formatMessage(m.drawerEnvironments)}
              </Title>
              <Stack hasGutter>
                {groups.map((group) => {
                  const groupStage = change.stages.find(
                    (s) => s.key === `env:${group}`,
                  );
                  return (
                    <StackItem key={group}>
                      <Title headingLevel="h4" size="md">
                        {groupLabel(group)}
                      </Title>
                      <DescriptionList isCompact isHorizontal>
                        {change.environments
                          .filter((env) => env.group === group)
                          .map((env) => (
                            <DescriptionListGroup key={env.env}>
                              <DescriptionListTerm>
                                <Button
                                  aria-label={intl.formatMessage(m.openNode, {
                                    env: env.env,
                                  })}
                                  isInline
                                  variant="link"
                                  onClick={() => {
                                    onNavigate({
                                      type: "openNodeOnMap",
                                      id: env.env,
                                    });
                                  }}
                                >
                                  {env.env}
                                </Button>
                              </DescriptionListTerm>
                              <DescriptionListDescription>
                                <StageStatus
                                  stage={{
                                    ...(groupStage ?? emptyStage(env.env)),
                                    key: `env:${group}`,
                                    state: env.state,
                                    reason:
                                      env.state === "passed"
                                        ? null
                                        : (groupStage?.reason ?? null),
                                    finishedAt: null,
                                    startedAt:
                                      env.state === "passed"
                                        ? null
                                        : (groupStage?.startedAt ?? null),
                                    done: 0,
                                    total: 0,
                                    failedAttempts: 0,
                                  }}
                                  slow={false}
                                  nowMs={nowMs}
                                />
                                {eventfulHistory(env) ? (
                                  <PromotionHistory env={env} />
                                ) : null}
                              </DescriptionListDescription>
                            </DescriptionListGroup>
                          ))}
                      </DescriptionList>
                    </StackItem>
                  );
                })}
              </Stack>
            </StackItem>
          ) : null}
          {bundle !== null ? (
            <StackItem>
              {/* FLT-DLV-11: the first bundle on the Fleet Map, and its Shipped row in
                  Agent SDLC only when the bundle is locked (a pending bundle has none). */}
              <ActionList isVertical>
                <ActionListItem>
                  <Button
                    variant="secondary"
                    onClick={() => {
                      onNavigate({
                        type: "openBundleOnMap",
                        seed: bundle.digest,
                      });
                    }}
                  >
                    {intl.formatMessage(m.showOnMap)}
                  </Button>
                </ActionListItem>
                {bundle.locked ? (
                  <ActionListItem>
                    <Button
                      variant="secondary"
                      onClick={() => {
                        onNavigate({
                          type: "openBundleInSdlc",
                          seed: bundle.digest,
                        });
                      }}
                    >
                      {intl.formatMessage(m.showInSdlc)}
                    </Button>
                  </ActionListItem>
                ) : null}
              </ActionList>
            </StackItem>
          ) : null}
        </Stack>
      </DrawerPanelBody>
    </DrawerPanelContent>
  );
}

function emptyStage(key: string): DeliveryStage {
  return {
    key,
    state: "unknown",
    startedAt: null,
    finishedAt: null,
    reason: null,
    components: [],
    groups: [],
    gate: null,
    done: 0,
    total: 0,
    pr: null,
    checks: null,
    failedAttempts: 0,
  };
}

// --- Page ------------------------------------------------------------------

function SourceAlerts({ data }: { readonly data: DeliveryData }) {
  const intl = useIntl();
  const degraded = degradedSources(data);
  if (degraded.length === 0) {
    return null;
  }
  return (
    <>
      {degraded.map(({ key, status }) => {
        const source = intl.formatMessage(SOURCE_LABEL[key]);
        const stageNames =
          key === "promotion"
            ? intl.formatMessage(m.environmentStages)
            : key === "history"
              ? intl.formatMessage(m.historyStages)
              : intl.formatList(
                  SOURCE_STAGES[key].map((k) =>
                    stageLabel(intl, { key: k, group: null }),
                  ),
                );
        const title =
          status.never || status.lastSuccess === null
            ? intl.formatMessage(m.sourceNeverTitle, { source })
            : intl.formatMessage(m.sourceStaleTitle, {
                source,
                time: formatClock(intl, status.lastSuccess),
              });
        const body =
          status.never || status.lastSuccess === null
            ? intl.formatMessage(m.sourceNeverBody, { stages: stageNames })
            : intl.formatMessage(m.sourceStaleBody, { stages: stageNames });
        return (
          <StackItem key={key}>
            <Alert isInline title={title} variant="warning">
              {body}
            </Alert>
          </StackItem>
        );
      })}
    </>
  );
}

function LoadedBody({
  data,
  selectedSha,
  nowMs,
  onNavigate,
}: {
  readonly data: DeliveryData;
  readonly selectedSha: string | null;
  readonly nowMs: number;
  readonly onNavigate: (action: NavAction) => void;
}) {
  const intl = useIntl();
  const outsideWindow =
    selectedSha !== null && findChange(data, selectedSha) === null;
  if (changeListUnavailable(data)) {
    return (
      <Stack hasGutter>
        <SourceAlerts data={data} />
        <StackItem>
          <EmptyState
            headingLevel="h2"
            icon={CubesIcon}
            titleText={intl.formatMessage(m.noChangesTitle)}
          >
            <EmptyStateBody>
              {intl.formatMessage(m.noChangesBody)}
            </EmptyStateBody>
          </EmptyState>
        </StackItem>
      </Stack>
    );
  }
  return (
    <Stack hasGutter>
      <SourceAlerts data={data} />
      {outsideWindow ? (
        <StackItem>
          <Alert
            isInline
            title={intl.formatMessage(m.changeOutsideWindow)}
            variant="info"
          />
        </StackItem>
      ) : null}
      <StackItem>
        <NowAndNext data={data} nowMs={nowMs} />
      </StackItem>
      <StackItem>
        <ChangeTable
          data={data}
          selectedSha={selectedSha}
          nowMs={nowMs}
          onNavigate={onNavigate}
        />
      </StackItem>
    </Stack>
  );
}

function Body({
  state,
  selectedSha,
  nowMs,
  onNavigate,
}: Omit<DeliveryViewProps, "onRefresh">) {
  const intl = useIntl();
  if (state.status === "loading") {
    return (
      <EmptyState
        headingLevel="h2"
        icon={Spinner}
        titleText={intl.formatMessage(m.loading)}
      />
    );
  }
  if (state.status === "error") {
    return (
      <Alert isInline title={intl.formatMessage(m.errorTitle)} variant="danger">
        {intl.formatMessage(m.errorBody)}
      </Alert>
    );
  }
  const data = state.plane.data;
  if (!data.enabled) {
    return (
      <EmptyState
        headingLevel="h2"
        icon={CubesIcon}
        titleText={intl.formatMessage(m.notConfiguredTitle)}
      >
        <EmptyStateBody>
          {intl.formatMessage(m.notConfiguredBody)}
        </EmptyStateBody>
      </EmptyState>
    );
  }
  const selected = findChange(data, selectedSha);
  const body = (
    <LoadedBody
      data={data}
      selectedSha={selectedSha}
      nowMs={nowMs}
      onNavigate={onNavigate}
    />
  );
  if (selected === null) {
    return body;
  }
  return (
    <Drawer isExpanded isInline>
      <DrawerContent
        panelContent={
          <ChangeDetails
            data={data}
            change={selected}
            nowMs={nowMs}
            onNavigate={onNavigate}
          />
        }
      >
        <DrawerContentBody>{body}</DrawerContentBody>
      </DrawerContent>
    </Drawer>
  );
}

export function DeliveryView(props: DeliveryViewProps): React.ReactElement {
  const intl = useIntl();
  const { state, onRefresh } = props;
  const asOf =
    state.status === "ready" && state.plane.data.enabled
      ? formatClock(intl, state.plane.generatedAt)
      : "";
  return (
    <div className={styles.root}>
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
        <Flex
          alignItems={{ default: "alignItemsFlexStart" }}
          justifyContent={{ default: "justifyContentSpaceBetween" }}
        >
          <FlexItem>
            <Content>
              <Title headingLevel="h1" size="2xl">
                {intl.formatMessage(m.pageTitle)}
              </Title>
              <Content component="p">
                {intl.formatMessage(m.pageDescription)}
              </Content>
              {asOf ? (
                <Content
                  className="pf-v6-u-text-color-subtle"
                  component="small"
                >
                  {intl.formatMessage(m.asOf, { time: asOf })}
                </Content>
              ) : null}
            </Content>
          </FlexItem>
          <FlexItem>
            <Button
              aria-label={intl.formatMessage(m.refresh)}
              icon={<SyncAltIcon />}
              variant="plain"
              onClick={onRefresh}
            />
          </FlexItem>
        </Flex>
      </PageSection>
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="secondary"
      >
        <Body {...props} />
      </PageSection>
    </div>
  );
}

/** Ticks a reference instant so elapsed times and the slow warning stay current. */
function useNow(intervalMs: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => {
      setNow(Date.now());
    }, intervalMs);
    return () => {
      clearInterval(id);
    };
  }, [intervalMs]);
  return now;
}

/** The Delivery tab wired to the delivery plane and the router. */
export function DeliveryTab(): React.ReactElement {
  const query = useDelivery();
  const { route, navigate } = useRouter();
  const nowMs = useNow(30_000);
  const state: DeliveryViewState = query.isPending
    ? { status: "loading" }
    : query.isError
      ? { status: "error" }
      : { status: "ready", plane: query.data };
  return (
    <DeliveryView
      state={state}
      selectedSha={route.delivery.change}
      nowMs={nowMs}
      onNavigate={navigate}
      onRefresh={() => {
        void query.refetch();
      }}
    />
  );
}
