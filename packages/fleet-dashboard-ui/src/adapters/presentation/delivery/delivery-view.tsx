// Glass Delivery tab (fleet-dashboard-delivery-pipeline.spec FLT-DLV-01..16).
// DeliveryView is presentational: it renders one plane state, the selected change,
// and a reference instant, and reports navigation as typed actions. DeliveryTab
// wires it to the query hook and the router.

import {
  Alert,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  EmptyState,
  EmptyStateBody,
  Flex,
  FlexItem,
  Icon,
  List,
  ListItem,
  PageSection,
  Popover,
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
  RedoIcon,
  SyncAltIcon,
} from "@patternfly/react-icons";
import { useEffect, useRef, useState } from "react";
import type { MessageDescriptor } from "react-intl";
import { useIntl, type IntlShape } from "react-intl";

import {
  SOURCE_STAGES,
  changeListUnavailable,
  degradedSources,
  loadingSources,
  elapsedMs,
  findChange,
  firstBundleOutcome,
  groupLabel,
  changeNowNext,
  changeSeverity,
  progress,
  whereToLook,
  type Arrival,
  type DeliveryChange,
  type DeliveryCheck,
  type DeliveryImageCheck,
  type DeliveryData,
  type DeliveryEnvState,
  type DeliveryPromotion,
  type DeliverySeverity,
  type DeliveryStage,
  type DeliveryStageDef,
  type DeliveryStageState,
  type FirstBundleOutcome,
  type MissingState,
  type ChangeNowNext,
  type NowNextRow,
  type PromotionOutcome,
  type WhereToLook,
} from "../../../domain/delivery";
import { identiName } from "../../../domain/map/identiname";
import type { NavAction } from "../../../domain/navigation/actions";
import type { Plane } from "../../../domain/plane";
import { useDelivery } from "../../query/hooks";
import { useRouter } from "../../routing/router";
import {
  BundleName,
  RichText,
  SOURCE_LABEL,
  STATE_LABEL,
  StageStateIcon,
  formatClock,
  formatDuration,
  gateName,
} from "./delivery-format";
import { deliveryMessages as m } from "./delivery-messages";
import styles from "./delivery-view.module.css";
import { PipelineNow } from "./pipeline-now";

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
  /** A read of the plane is in progress (FLT-DLV-01.03). */
  readonly refreshing?: boolean;
  /** The Shipped section (FLT-DLV-25), drawn between Pipeline now and the strip. */
  readonly shipped?: React.ReactNode;
}

// --- Formatting -------------------------------------------------------------

const PIPELINE_LABEL: Readonly<Record<string, MessageDescriptor>> = {
  merged: m.stageMerged,
  build: m.stageBuild,
  policy: m.stagePolicy,
  release: m.stageRelease,
  gitops: m.stageGitops,
};

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

const MISSING_LABEL: Readonly<Record<MissingState, MessageDescriptor>> = {
  building: m.missingBuilding,
  policy: m.missingPolicy,
  releasing: m.missingReleasing,
  failed: m.missingFailed,
};

/** "restarts when the <components> bundle lands" (FLT-DLV-10.07). */
function restartText(intl: IntlShape, stage: DeliveryStage): string {
  return stage.restart !== null
    ? intl.formatMessage(m.restartsWhen, {
        components: intl.formatList(stage.restart.components),
      })
    : "";
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
    case "outside-bundle":
      return intl.formatMessage(m.reasonOutsideBundle);
    case "waiting-bundle-pr":
      return intl.formatMessage(m.reasonWaitingBundlePR);
    case "waiting-hydration":
      return intl.formatMessage(m.reasonWaitingHydration);
    case "waiting-envs":
      return intl.formatMessage(m.reasonWaitingEnvs, {
        envs: intl.formatList(stage.waitingFor),
      });
    case "rebuilt-in":
      return stage.rebuiltIn !== null
        ? intl.formatMessage(m.reasonRebuiltIn, {
            change: changeName(stage.rebuiltIn),
          })
        : "";
    case "partly-released":
      return (stage.partly?.missing ?? [])
        .map((x) => intl.formatMessage(MISSING_LABEL[x.state], x))
        .join(", ");
    case "partly-shipped":
      return "";
    case "gate-running":
      return joinParts([
        intl.formatMessage(m.reasonGateRunning, {
          gate: gateName(intl, stage.gate),
        }),
        restartText(intl, stage),
      ]);
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
  if (stage.partly !== null) {
    return intl.formatMessage(
      stage.key === "release" ? m.partlyReleased : m.partlyShipped,
      { done: stage.partly.done, total: stage.partly.total },
    );
  }
  const elapsed = elapsedMs(stage, nowMs);
  if (elapsed !== null) {
    return formatDuration(intl, elapsed);
  }
  if (stage.state === "passed" || stage.state === "failed") {
    return formatClock(intl, stage.finishedAt, nowMs);
  }
  return "";
}

// --- Cells ------------------------------------------------------------------

function OverdueLabel() {
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
      <FlexItem>{intl.formatMessage(m.overdue)}</FlexItem>
    </Flex>
  );
}

function arrivalText(
  intl: IntlShape,
  arrival: Arrival | null,
  nowMs: number,
): string {
  switch (arrival?.kind) {
    case "range":
      return intl.formatMessage(m.arrivalRange, {
        early: formatClock(intl, arrival.earlyMs, nowMs),
        late: formatClock(intl, arrival.lateMs, nowMs),
      });
    case "time":
      return intl.formatMessage(m.arrivalTime, {
        time: formatClock(intl, arrival.atMs, nowMs),
      });
    default:
      return "";
  }
}

function LookTerm({
  term,
  children,
}: {
  readonly term: string;
  readonly children: React.ReactNode;
}) {
  return (
    <DescriptionListGroup>
      <DescriptionListTerm>{term}</DescriptionListTerm>
      <DescriptionListDescription>{children}</DescriptionListDescription>
    </DescriptionListGroup>
  );
}

/** The image check run in a Build popup (FLT-DLV-20.03). */
function ImageCheckTerm({
  check,
}: {
  readonly check: DeliveryImageCheck | null;
}) {
  const intl = useIntl();
  if (check === null) {
    return null;
  }
  return (
    <LookTerm term={intl.formatMessage(m.lookImageCheck)}>
      <Stack>
        <StackItem>
          {intl.formatMessage(STATE_LABEL[check.state])}
          {check.url !== null ? (
            <span className={styles.gap}>
              <ExternalLink href={check.url}>
                {intl.formatMessage(m.lookViewImageCheck)}
              </ExternalLink>
            </span>
          ) : null}
        </StackItem>
        {check.failures.map((f) => (
          <StackItem key={f}>{f}</StackItem>
        ))}
      </Stack>
    </LookTerm>
  );
}

function WarningText({ children }: { readonly children: React.ReactNode }) {
  return (
    <Flex spaceItems={{ default: "spaceItemsXs" }}>
      <FlexItem>
        <Icon isInline status="warning">
          <ExclamationTriangleIcon aria-hidden />
        </Icon>
      </FlexItem>
      <FlexItem>{children}</FlexItem>
    </Flex>
  );
}

// FLT-DLV-20: the content of the Where to look popup for one stage.
function WhereToLookBody({ look }: { readonly look: WhereToLook }) {
  const intl = useIntl();
  switch (look.kind) {
    case "build-failed":
      return (
        <Stack hasGutter>
          <StackItem>{intl.formatMessage(m.lookBuildFailed)}</StackItem>
          <StackItem className="pf-v6-u-text-color-subtle">
            {intl.formatMessage(m.lookBuildWhy)}
          </StackItem>
          <StackItem>
            <DescriptionList isCompact>
              {look.checks.map((c) => (
                <LookTerm key={c.component} term={c.component}>
                  {formatClock(intl, c.completedAt)}
                  {c.url !== null ? (
                    <span className={styles.gap}>
                      <ExternalLink href={c.url}>
                        {intl.formatMessage(m.lookViewCheckRun)}
                      </ExternalLink>
                    </span>
                  ) : null}
                </LookTerm>
              ))}
              <ImageCheckTerm check={look.imageCheck} />
            </DescriptionList>
          </StackItem>
        </Stack>
      );
    case "checks-overdue":
      return (
        <Stack hasGutter>
          <StackItem>{intl.formatMessage(m.lookChecksRunning)}</StackItem>
          <StackItem>
            <DescriptionList isCompact>
              {look.checks.map((c) => (
                <LookTerm key={c.component} term={c.component}>
                  {intl.formatMessage(STATE_LABEL[c.state])}
                  {c.url !== null ? (
                    <span className={styles.gap}>
                      <ExternalLink href={c.url}>
                        {intl.formatMessage(m.lookViewCheckRun)}
                      </ExternalLink>
                    </span>
                  ) : null}
                </LookTerm>
              ))}
              <ImageCheckTerm check={look.imageCheck} />
            </DescriptionList>
          </StackItem>
        </Stack>
      );
    case "release-overdue": {
      const d = look.diagnostics;
      if (d === null) {
        return (
          <Content component="p">{intl.formatMessage(m.lookNotLoaded)}</Content>
        );
      }
      const differ =
        d.registryTag !== null &&
        d.githubReleaseTag !== null &&
        d.registryTag !== d.githubReleaseTag;
      return (
        <Stack hasGutter>
          <StackItem>{intl.formatMessage(m.lookReleaseSummary)}</StackItem>
          <StackItem>
            <DescriptionList isCompact>
              {d.lastGood !== null ? (
                <LookTerm term={intl.formatMessage(m.lookLastGood)}>
                  {intl.formatMessage(m.lookLastGoodValue, {
                    bundle: identiName(d.lastGood.digest),
                    time: formatClock(intl, d.lastGood.publishedAt),
                  })}
                </LookTerm>
              ) : null}
              {d.logUrl !== null ? (
                <LookTerm term={intl.formatMessage(m.lookKonflux)}>
                  <ExternalLink href={d.logUrl}>
                    {intl.formatMessage(m.lookOpenLog)}
                  </ExternalLink>
                </LookTerm>
              ) : null}
              <LookTerm term={intl.formatMessage(m.lookPublisherChanges)}>
                {d.commitsError !== null ? (
                  <WarningText>
                    {intl.formatMessage(m.lookReadFailed)}
                  </WarningText>
                ) : d.commits.length === 0 ? (
                  intl.formatMessage(m.lookNoPublisherChanges)
                ) : (
                  <List isPlain>
                    {d.commits.map((c) => (
                      <ListItem key={c.sha}>
                        <ExternalLink href={c.url}>
                          {c.pr !== null
                            ? intl.formatMessage(m.lookCommitWithPr, {
                                sha: c.shortSha,
                                pr: String(c.pr),
                              })
                            : c.shortSha}
                        </ExternalLink>
                        <span className={styles.gap}>{c.title}</span>
                      </ListItem>
                    ))}
                  </List>
                )}
              </LookTerm>
              {d.appUrl !== null ? (
                <LookTerm term={intl.formatMessage(m.lookReleaseApp)}>
                  {intl.formatMessage(m.lookReleaseAppText)}
                  <span className={styles.gap}>
                    <ExternalLink href={d.appUrl}>
                      {intl.formatMessage(m.lookCheckApp)}
                    </ExternalLink>
                  </span>
                </LookTerm>
              ) : null}
              {d.registryTag !== null ? (
                <LookTerm term={intl.formatMessage(m.lookRegistryTag)}>
                  <Truncate content={d.registryTag} position="middle" />
                </LookTerm>
              ) : null}
              <LookTerm term={intl.formatMessage(m.lookGithubRelease)}>
                {d.releaseError !== null ? (
                  <WarningText>
                    {intl.formatMessage(m.lookReadFailed)}
                  </WarningText>
                ) : (
                  <>
                    {d.githubReleaseTag !== null ? (
                      <Truncate
                        content={d.githubReleaseTag}
                        position="middle"
                      />
                    ) : null}
                    {differ ? (
                      <WarningText>
                        {intl.formatMessage(m.lookTagsDiffer)}
                      </WarningText>
                    ) : null}
                  </>
                )}
              </LookTerm>
            </DescriptionList>
          </StackItem>
        </Stack>
      );
    }
    case "gitops":
      return look.pr !== null ? (
        <DescriptionList isCompact>
          <LookTerm term={intl.formatMessage(m.lookBundlePR)}>
            {look.pr.url !== null ? (
              <ExternalLink href={look.pr.url}>
                {intl.formatMessage(m.pullRequest, { number: look.pr.number })}
              </ExternalLink>
            ) : (
              intl.formatMessage(m.pullRequest, { number: look.pr.number })
            )}
          </LookTerm>
        </DescriptionList>
      ) : (
        <Content component="p">{intl.formatMessage(m.lookNoBundlePR)}</Content>
      );
    default:
      return (
        <DescriptionList isCompact>
          {look.environments.map((e) => (
            <LookTerm key={e.env} term={e.env}>
              <Stack>
                <StackItem>
                  {intl.formatMessage(STATE_LABEL[e.state])}
                </StackItem>
                {e.analysisUrl !== null ? (
                  <StackItem>
                    <ExternalLink href={e.analysisUrl}>
                      {intl.formatMessage(m.lookAnalysis)}
                    </ExternalLink>
                  </StackItem>
                ) : null}
                {e.argoUrl !== null ? (
                  <StackItem>
                    <ExternalLink href={e.argoUrl}>
                      {intl.formatMessage(m.lookArgo)}
                    </ExternalLink>
                  </StackItem>
                ) : null}
              </Stack>
            </LookTerm>
          ))}
        </DescriptionList>
      );
  }
}

// FLT-DLV-20: opens on hover, keyboard focus, and tap; Escape, a click
// outside, or the close button closes it. Hover and focus open it softly: the
// click that follows them keeps it open instead of closing it again.
function WhereToLookPopover({
  look,
  stageName,
}: {
  readonly look: WhereToLook;
  readonly stageName: string;
}) {
  const intl = useIntl();
  const [open, setOpen] = useState(false);
  const soft = useRef(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const title = intl.formatMessage(m.whereToLookTitle, { stage: stageName });
  const openSoftly = () => {
    if (!open) {
      soft.current = true;
      setOpen(true);
    }
  };
  return (
    <Popover
      aria-label={title}
      bodyContent={<WhereToLookBody look={look} />}
      headerContent={title}
      isVisible={open}
      position="bottom"
      shouldClose={(event) => {
        const target = event.target;
        if (
          soft.current &&
          target instanceof Node &&
          trigger.current?.contains(target) === true
        ) {
          soft.current = false;
          return;
        }
        soft.current = false;
        setOpen(false);
      }}
      shouldOpen={() => {
        soft.current = false;
        setOpen(true);
      }}
      triggerRef={trigger}
    >
      <Button
        className="pf-v6-u-font-size-sm"
        isInline
        onClick={(event) => {
          event.stopPropagation();
        }}
        onFocus={openSoftly}
        onPointerEnter={(event) => {
          if (event.pointerType === "mouse") {
            openSoftly();
          }
        }}
        ref={trigger}
        variant="link"
      >
        {intl.formatMessage(m.whereToLook)}
      </Button>
    </Popover>
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
function PromotionHistory({
  env,
  attempts,
  nowMs,
}: {
  readonly env: DeliveryEnvState;
  readonly attempts: readonly DeliveryPromotion[];
  readonly nowMs: number;
}) {
  const intl = useIntl();
  return (
    <List
      aria-label={intl.formatMessage(m.promotionHistoryLabel, { env: env.env })}
      className="pf-v6-u-font-size-sm pf-v6-u-mt-xs"
      isPlain
    >
      {attempts.map((a) => {
        const values = {
          outcome: intl.formatMessage(OUTCOME_LABEL[a.outcome]),
          // The Fleet Map's bundle name, so two bundles of one day differ.
          bundle: a.digest ? identiName(a.digest) : a.version,
          time: formatClock(intl, a.promotedAt, nowMs),
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
                  <RichText
                    bundles={[a.digest]}
                    nowMs={nowMs}
                    text={text}
                    times={[a.promotedAt]}
                  />
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

// --- Change panels (FLT-DLV-03, FLT-DLV-10, FLT-DLV-11, FLT-DLV-19) -------

/** Text with the times and bundles it names (FLT-DLV-24). */
interface Rich {
  readonly text: string;
  readonly times: readonly (string | number | null)[];
  readonly bundles: readonly (string | null)[];
}

function RichNode({
  value,
  nowMs,
}: {
  readonly value: Rich;
  readonly nowMs: number;
}) {
  return (
    <RichText
      bundles={value.bundles}
      nowMs={nowMs}
      text={value.text}
      times={value.times}
    />
  );
}

function nowText(
  intl: IntlShape,
  data: DeliveryData,
  row: NowNextRow,
  nowMs: number,
): string {
  const { stage } = row.now;
  const elapsed =
    row.now.elapsedMs !== null
      ? ` - ${formatDuration(intl, row.now.elapsedMs)}`
      : "";
  const typical =
    (row.now.estimateMs !== null &&
    (stage.state === "running" || stage.state === "waiting")
      ? `, ${intl.formatMessage(m.usually, { duration: formatDuration(intl, row.now.estimateMs) })}`
      : "") +
    (row.now.stageOverdue ? ` - ${intl.formatMessage(m.nowOverdue)}` : "");
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
    const at = formatClock(intl, stage.finishedAt, nowMs);
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
      : stage.partly !== null
        ? intl.formatMessage(
            stage.key === "release" ? m.partlyReleased : m.partlyShipped,
            { done: stage.partly.done, total: stage.partly.total },
          )
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
  nowMs: number,
): string {
  const next = row.next;
  switch (next.kind) {
    case "restart":
      return intl.formatMessage(
        next.restart === "new-change" ? m.nextRestartBuild : m.nextRestartGate,
      );
    case "none":
      return intl.formatMessage(m.nextNone);
    case "bundle-restart": {
      const text = intl.formatMessage(m.nextBundleRestart, {
        gate: [
          intl.formatList(next.restart.envs),
          gateName(intl, next.stage.gate).toLowerCase(),
        ]
          .filter((p) => p !== "")
          .join(" "),
        bundle: intl.formatMessage(m.bundleOf, {
          components: intl.formatList(next.restart.components),
        }),
      });
      return next.landsAtMs !== null
        ? intl.formatMessage(m.nextAbout, {
            text,
            time: formatClock(intl, next.landsAtMs, nowMs),
          })
        : text;
    }
    default: {
      const def = data.stages.find((d) => d.key === next.stage.key);
      const stage = stageLabel(
        intl,
        def ?? { key: next.stage.key, group: null },
      );
      if (row.now.overdue) {
        return intl.formatMessage(m.nextOverdue, { stage });
      }
      return next.expectedAtMs !== null
        ? intl.formatMessage(m.nextStageAt, {
            stage,
            time: formatClock(intl, next.expectedAtMs, nowMs),
          })
        : stage;
    }
  }
}

/** The times a Now and Next row names. */
function rowTimes(row: NowNextRow): (string | number | null)[] {
  const next = row.next;
  return [
    row.now.stage.finishedAt,
    next.kind === "stage"
      ? next.expectedAtMs
      : next.kind === "bundle-restart"
        ? next.landsAtMs
        : null,
  ];
}

/** Joins non-empty text parts with the cell separator used across the tab. */
function joinParts(parts: readonly string[]): string {
  return parts.filter((p) => p !== "").join(" - ");
}

// FLT-DLV-10: where the change is now and what comes next, or its delivery.
function NowNextLines({
  data,
  status,
  nowMs,
}: {
  readonly data: DeliveryData;
  readonly status: ChangeNowNext;
  readonly nowMs: number;
}) {
  const intl = useIntl();
  switch (status.kind) {
    case "delivered":
      return (
        <Content component="p">
          {status.deliveredAtMs !== null && status.totalMs !== null ? (
            <RichText
              nowMs={nowMs}
              text={intl.formatMessage(m.panelDelivered, {
                time: formatClock(intl, status.deliveredAtMs, nowMs),
                total: formatDuration(intl, status.totalMs),
              })}
              times={[status.deliveredAtMs]}
            />
          ) : (
            intl.formatMessage(m.panelDeliveredNoTime)
          )}
        </Content>
      );
    case "none":
      return (
        <Content component="p">{intl.formatMessage(m.panelAllStages)}</Content>
      );
    default: {
      const row = status.row;
      return (
        <DescriptionList isCompact>
          <DescriptionListGroup>
            <DescriptionListTerm>
              {intl.formatMessage(m.now)}
            </DescriptionListTerm>
            <DescriptionListDescription>
              <RichText
                nowMs={nowMs}
                text={nowText(intl, data, row, nowMs)}
                times={rowTimes(row)}
              />
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>
              {intl.formatMessage(m.next)}
            </DescriptionListTerm>
            <DescriptionListDescription>
              <RichText
                nowMs={nowMs}
                text={nextText(intl, data, row, nowMs)}
                times={rowTimes(row)}
              />
            </DescriptionListDescription>
          </DescriptionListGroup>
        </DescriptionList>
      );
    }
  }
}

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

/**
 * The description of one stepper step: its state's time (FLT-DLV-19), the
 * Release step's bundle, the Renovate pull request's opening time
 * (FLT-DLV-07), and a group's arrival when not reached yet.
 */
function stepDescription(
  intl: IntlShape,
  change: DeliveryChange,
  stage: DeliveryStage,
  arrival: Arrival | null,
  nowMs: number,
): Rich {
  const times: (string | number | null)[] = [stage.startedAt, stage.finishedAt];
  const bundles: (string | null)[] = [];
  const parts: string[] = [];
  if (
    stage.state === "passed" &&
    stage.startedAt !== null &&
    stage.finishedAt !== null
  ) {
    const ms = Date.parse(stage.finishedAt) - Date.parse(stage.startedAt);
    parts.push(
      intl.formatMessage(m.stepRange, {
        start: formatClock(intl, stage.startedAt, nowMs),
        end: formatClock(intl, stage.finishedAt, nowMs),
        duration: formatDuration(intl, Math.max(0, ms)),
      }),
    );
  } else if (stage.state === "passed") {
    parts.push(formatClock(intl, stage.finishedAt, nowMs));
  } else {
    parts.push(stagePrimary(intl, stage, nowMs));
  }
  parts.push(stageDetail(intl, stage));
  if (
    stage.key === "gitops" &&
    (stage.state === "running" || stage.state === "failed") &&
    stage.startedAt !== null
  ) {
    parts.push(
      intl.formatMessage(m.stepOpened, {
        time: formatClock(intl, stage.startedAt, nowMs),
      }),
    );
  }
  if (
    (stage.state === "running" || stage.state === "waiting") &&
    stage.estimateMs !== null
  ) {
    parts.push(
      intl.formatMessage(m.usually, {
        duration: formatDuration(intl, stage.estimateMs),
      }),
    );
  }
  if (arrival !== null && arrival.kind !== "reached") {
    parts.push(arrivalText(intl, arrival, nowMs));
    if (arrival.kind === "range") {
      times.push(arrival.earlyMs, arrival.lateMs);
    } else if (arrival.kind === "time") {
      times.push(arrival.atMs);
    }
  }
  if (stage.key === "release" && change.bundle !== null) {
    parts.push(identiName(change.bundle.digest));
    bundles.push(change.bundle.digest);
  }
  return { text: joinParts(parts), times, bundles };
}

const ENV_SEPARATOR = ": ";

/** A promotion's bundle as the Fleet Map names it (FLT-DLV-24). */
function attemptBundle(a: DeliveryPromotion): string {
  return a.digest ? identiName(a.digest) : a.version;
}

/**
 * One environment, reported by the change's first bundle only (FLT-DLV-18):
 * its state, the bundle and when it was deployed, and when its gates passed.
 * A later bundle appears only as the one that replaced the first.
 */
function envLine(
  intl: IntlShape,
  env: DeliveryEnvState,
  outcome: FirstBundleOutcome,
  nowMs: number,
  withName: boolean,
): Rich {
  const named = (state: string) =>
    withName ? intl.formatMessage(m.stepEnv, { env: env.env, state }) : state;
  if (outcome.kind === "none") {
    return {
      text: joinParts([
        named(intl.formatMessage(STATE_LABEL[env.state])),
        env.state !== "passed" && env.waitingFor.length > 0
          ? intl.formatMessage(m.reasonWaitingEnvs, {
              envs: intl.formatList(env.waitingFor),
            })
          : "",
      ]),
      times: [],
      bundles: [],
    };
  }
  if (outcome.kind === "not-deployed") {
    const later = outcome.replacedBy;
    return {
      text: joinParts([
        named(intl.formatMessage(m.envNotDeployed)),
        intl.formatMessage(m.envReplacedBy, {
          bundle: attemptBundle(later),
          time: formatClock(intl, later.promotedAt, nowMs),
        }),
      ]),
      times: [later.promotedAt],
      bundles: [later.digest || null],
    };
  }
  const a = outcome.attempt;
  const passed = outcome.kind === "passed" ? a.gatesPassedAt : null;
  return {
    text: joinParts([
      named(intl.formatMessage(STATE_LABEL[outcome.kind])),
      intl.formatMessage(m.envDeployed, {
        bundle: attemptBundle(a),
        time: formatClock(intl, a.promotedAt, nowMs),
      }),
      passed !== null
        ? intl.formatMessage(m.nowGatesPassed, {
            time: formatClock(intl, passed, nowMs),
          })
        : "",
      attemptDetail(intl, a),
    ]),
    times: [a.promotedAt, passed],
    bundles: [a.digest || null],
  };
}

function EnvIcon({
  env,
  outcome,
}: {
  readonly env: DeliveryEnvState;
  readonly outcome: FirstBundleOutcome;
}) {
  if (outcome.kind === "not-deployed") {
    return <OutcomeIcon outcome="replaced" />;
  }
  return (
    <StageStateIcon
      state={outcome.kind === "none" ? env.state : outcome.kind}
    />
  );
}

// FLT-DLV-19: one environment on its group's step - a link that opens the
// Fleet Map with the node selected, its first-bundle line (FLT-DLV-18), and the
// first bundle's promotions after a retry or failure.
function EnvItem({
  change,
  env,
  nowMs,
  onNavigate,
}: {
  readonly change: DeliveryChange;
  readonly env: DeliveryEnvState;
  readonly nowMs: number;
  readonly onNavigate: (action: NavAction) => void;
}) {
  const intl = useIntl();
  const outcome = firstBundleOutcome(change.bundle?.digest ?? null, env);
  return (
    <>
      <Flex
        spaceItems={{ default: "spaceItemsXs" }}
        flexWrap={{ default: "nowrap" }}
        alignItems={{ default: "alignItemsBaseline" }}
      >
        <FlexItem>
          <EnvIcon env={env} outcome={outcome} />
        </FlexItem>
        <FlexItem>
          <Button
            aria-label={intl.formatMessage(m.openNode, { env: env.env })}
            isInline
            variant="link"
            onClick={() => {
              onNavigate({ type: "openNodeOnMap", id: env.env });
            }}
          >
            {env.env}
          </Button>
          {ENV_SEPARATOR}
          <RichNode
            nowMs={nowMs}
            value={envLine(intl, env, outcome, nowMs, false)}
          />
        </FlexItem>
      </Flex>
      {eventfulAttempts(outcome) && outcome.kind !== "none" ? (
        <PromotionHistory attempts={outcome.attempts} env={env} nowMs={nowMs} />
      ) : null}
    </>
  );
}

/** The first bundle's history is worth showing: a retry or a failure. */
function eventfulAttempts(outcome: FirstBundleOutcome): boolean {
  return (
    outcome.kind !== "none" &&
    (outcome.attempts.length > 1 ||
      outcome.attempts.some((a) => a.outcome === "failed"))
  );
}

function StepBody({
  data,
  change,
  stage,
  arrival,
  overdue,
  nowMs,
  onNavigate,
}: {
  readonly onNavigate: (action: NavAction) => void;
  readonly data: DeliveryData;
  readonly change: DeliveryChange;
  readonly stage: DeliveryStage;
  readonly arrival: Arrival | null;
  readonly overdue: boolean;
  readonly nowMs: number;
}) {
  const intl = useIntl();
  const def = data.stages.find((d) => d.key === stage.key);
  const label = stageLabel(intl, def ?? { key: stage.key, group: null });
  const look = whereToLook(data, change, stage, nowMs);
  const envs =
    def?.kind === "group"
      ? change.environments.filter((e) => e.group === def.group)
      : [];
  return (
    <Stack>
      <StackItem>
        {stage.reason === "inferred-release"
          ? intl.formatMessage(m.stateReleasingInferred)
          : intl.formatMessage(STATE_LABEL[stage.state])}
      </StackItem>
      <StackItem>
        <RichNode
          nowMs={nowMs}
          value={stepDescription(intl, change, stage, arrival, nowMs)}
        />
      </StackItem>
      {envs.length > 0 ? (
        <StackItem>
          <List isPlain>
            {envs.map((env) => (
              <ListItem key={env.env}>
                <EnvItem
                  change={change}
                  env={env}
                  nowMs={nowMs}
                  onNavigate={onNavigate}
                />
              </ListItem>
            ))}
          </List>
        </StackItem>
      ) : null}
      {overdue ? (
        <StackItem>
          <OverdueLabel />
        </StackItem>
      ) : null}
      {look !== null ? (
        <StackItem>
          <WhereToLookPopover look={look} stageName={label} />
        </StackItem>
      ) : null}
      {stage.failedAttempts > 0 ? (
        <StackItem>
          {intl.formatMessage(m.failedAttempts, {
            count: stage.failedAttempts,
          })}
        </StackItem>
      ) : null}
    </Stack>
  );
}

function checkStateText(
  intl: IntlShape,
  check: DeliveryCheck | undefined,
): string {
  return check
    ? intl.formatMessage(STATE_LABEL[check.state])
    : intl.formatMessage(STATE_LABEL["not-started"]);
}

// FLT-DLV-11: the change's checks, bundle, Renovate pull request, and
// environments, shown in its panel without a click.
function PanelDetails({
  change,
  nowMs,
}: {
  readonly change: DeliveryChange;
  readonly nowMs: number;
}) {
  const intl = useIntl();
  const components = Array.from(new Set(change.builds.map((b) => b.component)));
  const bundle = change.bundle;
  const lockedAt =
    change.stages.find((st) => st.key === "gitops" && st.state === "passed")
      ?.finishedAt ?? null;
  return (
    <Stack hasGutter>
      <StackItem>
        <Title headingLevel="h4" size="md">
          {intl.formatMessage(m.drawerChecks)}
        </Title>
        {components.length === 0 ? (
          <Content component="p">{intl.formatMessage(m.noChecks)}</Content>
        ) : (
          <DescriptionList isCompact>
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
                      {build?.supersededBy ? (
                        <StackItem className="pf-v6-u-text-color-subtle">
                          {intl.formatMessage(m.reasonRebuiltIn, {
                            change: build.supersededBy,
                          })}
                        </StackItem>
                      ) : null}
                      {build?.outsideBundle === true ? (
                        <StackItem className="pf-v6-u-text-color-subtle">
                          {intl.formatMessage(m.outsideBundle)}
                        </StackItem>
                      ) : null}
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
        <Title headingLevel="h4" size="md">
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
          <DescriptionList isCompact>
            <DescriptionListGroup>
              <DescriptionListTerm>
                {intl.formatMessage(m.bundleAlias)}
              </DescriptionListTerm>
              <DescriptionListDescription>
                <Stack>
                  <StackItem>
                    <BundleName digest={bundle.digest} />
                  </StackItem>
                  <StackItem>
                    {/* FLT-DLV-11.04: every bundle fact carries a time. */}
                    {bundle.publishedAt !== null ? (
                      <RichText
                        nowMs={nowMs}
                        text={intl.formatMessage(m.bundleVersionPublished, {
                          version: bundle.version,
                          time: formatClock(intl, bundle.publishedAt, nowMs),
                        })}
                        times={[bundle.publishedAt]}
                      />
                    ) : (
                      bundle.version
                    )}
                  </StackItem>
                  <StackItem>
                    {lockedAt !== null ? (
                      <RichText
                        nowMs={nowMs}
                        text={intl.formatMessage(m.bundleLockedAt, {
                          time: formatClock(intl, lockedAt, nowMs),
                        })}
                        times={[lockedAt]}
                      />
                    ) : (
                      intl.formatMessage(
                        bundle.locked ? m.bundleLocked : m.bundlePending,
                      )
                    )}
                  </StackItem>
                </Stack>
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
                {bundle.releaseUrl !== null ? (
                  <ExternalLink href={bundle.releaseUrl}>
                    {intl.formatMessage(m.downloads)}
                  </ExternalLink>
                ) : (
                  intl.formatMessage(m.downloadsPending)
                )}
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
    </Stack>
  );
}

// FLT-DLV-11, FLT-DLV-19: one change: its Now and Next, a vertical stepper
// with one numbered step per stage, and its details.
function ChangePanel({
  data,
  change,
  selected,
  nowMs,
  onNavigate,
}: {
  readonly data: DeliveryData;
  readonly change: DeliveryChange;
  readonly selected: boolean;
  readonly nowMs: number;
  readonly onNavigate: (action: NavAction) => void;
}) {
  const intl = useIntl();
  const ref = useRef<HTMLLIElement>(null);
  const status = changeNowNext(data, change, nowMs);
  const model = progress(data, change, nowMs);
  const name = changeName(change);
  const titleId = `delivery-change-${change.sha}`;
  useEffect(() => {
    // FLT-DLV-14: the change parameter scrolls its panel into view.
    if (selected) {
      // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- jsdom has no scrollIntoView.
      ref.current?.scrollIntoView?.({ block: "nearest", inline: "start" });
    }
  }, [selected]);
  const cells = new Map(model.cells.map((c) => [c.stage.key, c]));
  return (
    <li
      aria-labelledby={titleId}
      className={styles.panel}
      data-selected={selected ? "true" : undefined}
      ref={ref}
    >
      <Card isCompact isFullHeight isPlain>
        <CardHeader>
          <CardTitle>
            <Flex
              alignItems={{ default: "alignItemsCenter" }}
              flexWrap={{ default: "wrap" }}
              spaceItems={{ default: "spaceItemsSm" }}
            >
              <FlexItem>
                <SeverityIcon severity={changeSeverity(status)} />
              </FlexItem>
              <FlexItem>
                <Title headingLevel="h3" id={titleId} size="md">
                  <Button
                    className="pf-v6-u-font-family-monospace"
                    isInline
                    variant="link"
                    onClick={() => {
                      onNavigate({
                        type: "deliverySelectChange",
                        sha: change.sha,
                      });
                    }}
                  >
                    {name}
                  </Button>
                </Title>
              </FlexItem>
              {model.percent !== null ? (
                <FlexItem align={{ default: "alignRight" }}>
                  <span
                    aria-label={intl.formatMessage(m.progressLabelPercent, {
                      change: name,
                      percent: model.percent,
                    })}
                    className="pf-v6-u-font-size-sm"
                    role="img"
                  >
                    {intl.formatMessage(m.progressPercent, {
                      percent: model.percent,
                    })}
                  </span>
                </FlexItem>
              ) : null}
            </Flex>
          </CardTitle>
          <Content component="p">
            <Truncate content={change.title} />
          </Content>
          <Content className="pf-v6-u-text-color-subtle" component="small">
            {change.author !== null
              ? intl.formatMessage(m.byAuthor, { author: change.author })
              : null}
            {change.prUrl !== null ? (
              <span className={styles.gap}>
                <ExternalLink href={change.prUrl}>
                  {intl.formatMessage(m.viewPR)}
                </ExternalLink>
              </span>
            ) : null}
            {change.commitUrl !== null ? (
              <span className={styles.gap}>
                <ExternalLink href={change.commitUrl}>
                  {intl.formatMessage(m.viewCommit)}
                </ExternalLink>
              </span>
            ) : null}
          </Content>
        </CardHeader>
        <CardBody>
          <Stack hasGutter>
            <StackItem>
              <NowNextLines data={data} nowMs={nowMs} status={status} />
            </StackItem>
            <StackItem>
              <ProgressStepper
                aria-label={intl.formatMessage(m.progressLabel, {
                  change: name,
                })}
                isVertical
              >
                {change.stages.map((stage, i) => {
                  const def = data.stages.find((d) => d.key === stage.key);
                  const label = stageLabel(
                    intl,
                    def ?? { key: stage.key, group: null },
                  );
                  const cell = cells.get(stage.key);
                  const id = `delivery-${change.shortSha}-${stage.key.replace(/[^a-z0-9-]/gi, "-")}`;
                  return (
                    <ProgressStep
                      description={
                        <StepBody
                          arrival={cell?.arrival ?? null}
                          change={change}
                          data={data}
                          nowMs={nowMs}
                          overdue={cell?.overdue ?? false}
                          stage={stage}
                          onNavigate={onNavigate}
                        />
                      }
                      icon={
                        <span className="pf-v6-u-font-size-xs">
                          {String(i + 1)}
                        </span>
                      }
                      id={id}
                      isCurrent={
                        stage.state === "running" || stage.state === "waiting"
                      }
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
              <PanelDetails change={change} nowMs={nowMs} />
            </StackItem>
          </Stack>
        </CardBody>
      </Card>
    </li>
  );
}

// FLT-DLV-03: the Changes on main strip, newest first. Only the strip scrolls
// sideways; it is reachable and scrollable by keyboard.
function ChangeStrip({
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
            <ol
              aria-label={intl.formatMessage(m.stripLabel)}
              className={styles.strip}
              // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- A scrollable region must be reachable by keyboard (FLT-DLV-03).
              tabIndex={0}
            >
              {data.changes.map((change) => (
                <ChangePanel
                  change={change}
                  data={data}
                  key={change.sha}
                  nowMs={nowMs}
                  onNavigate={onNavigate}
                  selected={change.sha === selectedSha}
                />
              ))}
            </ol>
          </StackItem>
        </Stack>
      </CardBody>
    </Card>
  );
}

// --- Page ------------------------------------------------------------------

function SourceAlerts({ data }: { readonly data: DeliveryData }) {
  const intl = useIntl();
  const degraded = degradedSources(data);
  const loading = loadingSources(data);
  if (degraded.length === 0 && loading.length === 0) {
    return null;
  }
  return (
    <>
      {/* Sources still loading after a restart share one informational alert
          instead of a warning each (FLT-DLV-13.04). */}
      {loading.length > 0 ? (
        <StackItem>
          <Alert
            isInline
            customIcon={<Spinner size="md" aria-hidden="true" />}
            title={intl.formatMessage(m.loadingTitle)}
            variant="info"
          >
            {intl.formatMessage(m.loadingBody, {
              sources: intl.formatList(
                loading.map((k) => intl.formatMessage(SOURCE_LABEL[k])),
              ),
              count: loading.length,
            })}
          </Alert>
        </StackItem>
      ) : null}
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
  shipped,
}: {
  readonly data: DeliveryData;
  readonly selectedSha: string | null;
  readonly nowMs: number;
  readonly onNavigate: (action: NavAction) => void;
  readonly shipped?: React.ReactNode;
}) {
  const intl = useIntl();
  const outsideWindow =
    selectedSha !== null && findChange(data, selectedSha) === null;
  if (changeListUnavailable(data)) {
    return (
      <Stack hasGutter>
        <SourceAlerts data={data} />
        <StackItem>
          <PipelineNow data={data} nowMs={nowMs} />
        </StackItem>
        {shipped !== undefined ? <StackItem>{shipped}</StackItem> : null}
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
        <PipelineNow data={data} nowMs={nowMs} />
      </StackItem>
      {shipped !== undefined ? <StackItem>{shipped}</StackItem> : null}
      <StackItem>
        <ChangeStrip
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
  shipped,
}: Omit<DeliveryViewProps, "onRefresh" | "refreshing">) {
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
  return (
    <LoadedBody
      data={data}
      selectedSha={selectedSha}
      nowMs={nowMs}
      onNavigate={onNavigate}
      shipped={shipped}
    />
  );
}

export function DeliveryView(props: DeliveryViewProps): React.ReactElement {
  const intl = useIntl();
  const { state, onRefresh, refreshing = false } = props;
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
          alignItems={{ default: "alignItemsCenter" }}
          justifyContent={{ default: "justifyContentSpaceBetween" }}
        >
          <FlexItem>
            <Content>
              {/* While the plane is read, the time gives way to
                  "Refreshing..." (FLT-DLV-01.03). */}
              {refreshing || asOf ? (
                <Content
                  className="pf-v6-u-text-color-subtle"
                  component="small"
                  aria-live="polite"
                >
                  {refreshing
                    ? intl.formatMessage(m.refreshing)
                    : intl.formatMessage(m.asOf, { time: asOf })}
                </Content>
              ) : null}
            </Content>
          </FlexItem>
          <FlexItem>
            <Button
              aria-label={intl.formatMessage(m.refresh)}
              aria-busy={refreshing}
              icon={
                <span
                  className={
                    refreshing ? styles.refreshSpin : styles.refreshIcon
                  }
                >
                  <SyncAltIcon />
                </span>
              }
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

/** Ticks a reference instant so elapsed times and the Overdue warning stay current. */
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

/** Shortest time a requested refresh shows its progress (FLT-DLV-01.03). */
const REFRESH_HOLD_MS = 800;

/** The Delivery tab wired to the delivery plane and the router. */
export function DeliveryTab({
  shipped,
}: {
  /** The Shipped section (FLT-DLV-25), owned by the Agent SDLC plane. */
  readonly shipped?: React.ReactNode;
}): React.ReactElement {
  const query = useDelivery();
  const { route, navigate } = useRouter();
  const nowMs = useNow(30_000);
  // Any read after the first, user-triggered or polled, shows as refreshing.
  // A refresh the operator asks for shows for at least REFRESH_HOLD_MS, so a
  // quick read is still seen (FLT-DLV-01.03).
  const [held, setHeld] = useState(false);
  const holdTimer = useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined,
  );
  useEffect(
    () => () => {
      clearTimeout(holdTimer.current);
    },
    [],
  );
  const refreshing = held || (query.isFetching && !query.isPending);
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
        setHeld(true);
        clearTimeout(holdTimer.current);
        holdTimer.current = setTimeout(() => {
          setHeld(false);
        }, REFRESH_HOLD_MS);
        void query.refetch();
      }}
      refreshing={refreshing}
      shipped={shipped}
    />
  );
}
