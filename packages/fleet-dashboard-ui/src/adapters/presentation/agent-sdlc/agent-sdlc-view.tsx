// The Agent SDLC view: an aging work-in-progress board (time-in-phase on the
// vertical axis), a rework meter per in-review card, an escalation lane, and a
// release-by-hub-cluster matrix (agent-sdlc-visibility-dashboard.spec). It holds no
// fleet knowledge: phase keys are the funnel's own vocabulary and everything else
// (titles, versions, hub-cluster names, digests) is runtime data from /api/sdlc.
//
// Every colour signal is paired with text or an icon so colour is never the sole
// channel (SDLC-VIS-16 / accessibility.spec).

import {
  Button,
  Content,
  Label,
  Popover,
  Stack,
  StackItem,
  Title,
} from "@patternfly/react-core";
import {
  ExpandableRowContent,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Tr,
} from "@patternfly/react-table";
import ArrowRightIcon from "@patternfly/react-icons/dist/esm/icons/arrow-right-icon";
import CheckCircleIcon from "@patternfly/react-icons/dist/esm/icons/check-circle-icon";
import ClockIcon from "@patternfly/react-icons/dist/esm/icons/clock-icon";
import CopyIcon from "@patternfly/react-icons/dist/esm/icons/copy-icon";
import ExclamationCircleIcon from "@patternfly/react-icons/dist/esm/icons/exclamation-circle-icon";
import ExclamationTriangleIcon from "@patternfly/react-icons/dist/esm/icons/exclamation-triangle-icon";
import QuestionCircleIcon from "@patternfly/react-icons/dist/esm/icons/question-circle-icon";
import { useState } from "react";
import type { IntlShape, MessageDescriptor } from "react-intl";
import { FormattedMessage, useIntl } from "react-intl";

import {
  agingRatio,
  agingScaleSeconds,
  boardPhases,
  bundleRowSeed,
  cardsInPhase,
  ESCALATION_BLOCKED,
  ESCALATION_DUPLICATE,
  ESCALATION_NEEDS_INPUT,
  ESCALATION_PAST_SLE,
  reworkRatio,
  reworkSeverity,
  type AgentSdlcBundleRow,
  type AgentSdlcCard,
  type AgentSdlcData,
  type AgentSdlcPhase,
  type AgentSdlcRework,
  type AgentSdlcShippedIssue,
} from "../../../domain/agent-sdlc";
import { messages } from "../../../messages";
import { TONE_COLOR } from "../map/colors";
import { ReleaseTime } from "../map/release-time";
import styles from "./agent-sdlc-view.module.css";

export interface AgentSdlcViewProps {
  readonly data: AgentSdlcData;
  readonly selectedBundleSeed: string | null;
  readonly onSelectBundle: (seed: string) => void;
  /** Switch to the Fleet Map tab (the shipped/bundle cross-link, SDLC-VIS-12). */
  readonly onJumpToMap: () => void;
  /** Reference instant for relative timestamps; injectable for deterministic tests. */
  readonly nowMs?: number;
}

const PHASE_MESSAGE: Record<string, MessageDescriptor> = {
  intake: messages.sdlcPhaseIntake,
  workable: messages.sdlcPhaseWorkable,
  spec: messages.sdlcPhaseSpec,
  "spec-approved": messages.sdlcPhaseSpecApproved,
  code: messages.sdlcPhaseCode,
  "code-approved": messages.sdlcPhaseCodeApproved,
  shipped: messages.sdlcPhaseShipped,
};

const ESCALATION_MESSAGE: Record<string, MessageDescriptor> = {
  [ESCALATION_BLOCKED]: messages.sdlcEscalationBlocked,
  [ESCALATION_NEEDS_INPUT]: messages.sdlcEscalationNeedsInput,
  [ESCALATION_DUPLICATE]: messages.sdlcEscalationDuplicate,
  [ESCALATION_PAST_SLE]: messages.sdlcEscalationPastSle,
};

function escalationIcon(escalation: string): React.ReactElement {
  switch (escalation) {
    case ESCALATION_BLOCKED:
      return <ExclamationCircleIcon />;
    case ESCALATION_NEEDS_INPUT:
      return <QuestionCircleIcon />;
    case ESCALATION_DUPLICATE:
      return <CopyIcon />;
    default:
      return <ClockIcon />;
  }
}

/** A colour token for an escalation bucket; always shown alongside icon + text. */
function escalationTone(escalation: string): string {
  if (escalation === ESCALATION_BLOCKED) {
    return TONE_COLOR.danger;
  }
  if (escalation === ESCALATION_PAST_SLE) {
    return TONE_COLOR.warning;
  }
  return TONE_COLOR.info;
}

/** Inline icon+text (or label) group with token spacing, so no literal space node
 *  is needed between an icon and its text. */
function Inline({
  children,
}: {
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: "var(--pf-t--global--spacer--xs, 4px)",
      }}
    >
      {children}
    </span>
  );
}

// Formatters are plain functions (not a hook) that take the reference instant as a
// parameter defaulting to Date.now(), mirroring release-time.tsx: the clock default
// stays out of component render (so react-compiler stays happy) and tests can inject
// a fixed instant for deterministic relative-time output.
function formatAge(intl: IntlShape, seconds: number): string {
  const totalHours = Math.floor(Math.max(seconds, 0) / 3600);
  const days = Math.floor(totalHours / 24);
  const hours = totalHours % 24;
  return intl.formatMessage(messages.sdlcAge, { days, hours });
}

function formatMergedAgo(
  intl: IntlShape,
  iso: string | null,
  nowMs: number = Date.now(),
): string | null {
  if (iso === null) {
    return null;
  }
  const parsed = Date.parse(iso);
  if (Number.isNaN(parsed)) {
    return null;
  }
  const days = Math.floor((nowMs - parsed) / 86_400_000);
  return days <= 0
    ? intl.formatMessage(messages.sdlcMergedToday)
    : intl.formatMessage(messages.sdlcMergedAgo, { days });
}

function formatLabelEventTime(intl: IntlShape, iso: string): string {
  const parsed = Date.parse(iso);
  if (Number.isNaN(parsed)) {
    return "";
  }
  return intl.formatDate(parsed, { dateStyle: "medium", timeStyle: "short" });
}

function ReworkMeter({
  rework,
}: {
  rework: AgentSdlcRework;
}): React.ReactElement {
  const intl = useIntl();
  const severity = reworkSeverity(rework);
  const tone =
    severity === "critical"
      ? TONE_COLOR.danger
      : severity === "warning"
        ? TONE_COLOR.warning
        : TONE_COLOR.info;
  const label = intl.formatMessage(messages.sdlcReworkLabel, {
    count: rework.count,
    threshold: rework.threshold,
  });
  return (
    <div>
      <div
        className={styles.reworkTrack}
        role="meter"
        aria-valuenow={rework.count}
        aria-valuemin={0}
        aria-valuemax={rework.threshold}
        aria-label={label}
      >
        <div
          className={styles.reworkFill}
          style={{
            width: `${String(reworkRatio(rework) * 100)}%`,
            background: tone,
          }}
        />
      </div>
      <Content component="small">
        {label}
        {severity === "critical" ? (
          <>
            {" "}
            <ExclamationTriangleIcon aria-hidden />{" "}
            <FormattedMessage {...messages.sdlcReworkCritical} />
          </>
        ) : severity === "warning" ? (
          <>
            {" "}
            <FormattedMessage {...messages.sdlcReworkWarning} />
          </>
        ) : null}
      </Content>
    </div>
  );
}

function LabelHistory({ card }: { card: AgentSdlcCard }): React.ReactElement {
  const intl = useIntl();
  if (card.history.length === 0) {
    return (
      <Content component="small">
        <FormattedMessage {...messages.sdlcHistoryEmpty} />
      </Content>
    );
  }
  return (
    <Stack>
      {card.history.map((event, i) => {
        const marker = event.action === "labeled" ? "+" : "-";
        // Timestamp is rendered through intl date formatting, not the raw ISO
        // string (SDLC-VIS-16 localization).
        const when = event.at
          ? ` (${formatLabelEventTime(intl, event.at)})`
          : "";
        const line = `${marker} ${event.label}${when}`;
        return (
          <StackItem key={`${event.at}:${event.label}:${String(i)}`}>
            <Content component="small">{line}</Content>
          </StackItem>
        );
      })}
    </Stack>
  );
}

/** A card body shared by the board and the lane. */
function CardBodyContent({
  card,
  showEscalation,
}: {
  card: AgentSdlcCard;
  showEscalation: boolean;
}): React.ReactElement {
  const intl = useIntl();
  const ageText = formatAge(intl, card.timeInPhaseSeconds);
  const issueRef = `#${String(card.number)}`;
  const issueLabel = `#${String(card.number)} ${card.title}`;
  return (
    <Stack hasGutter>
      <StackItem>
        <Popover
          headerContent={<FormattedMessage {...messages.sdlcHistoryTitle} />}
          bodyContent={<LabelHistory card={card} />}
          footerContent={
            <Button
              component="a"
              variant="link"
              isInline
              href={card.url}
              target="_blank"
              rel="noreferrer"
            >
              {issueRef}
            </Button>
          }
        >
          <Button
            variant="link"
            isInline
            aria-label={intl.formatMessage(messages.sdlcCardOpen, {
              number: card.number,
              title: card.title,
            })}
          >
            {issueLabel}
          </Button>
        </Popover>
      </StackItem>
      <StackItem>
        <Content component="small">
          <Inline>
            <ClockIcon aria-hidden />
            {intl.formatMessage(messages.sdlcTimeInPhase, {
              duration: ageText,
            })}
          </Inline>
        </Content>
      </StackItem>
      {showEscalation && card.escalation !== null ? (
        <StackItem>
          <Label
            isCompact
            icon={escalationIcon(card.escalation)}
            style={{ borderColor: escalationTone(card.escalation) }}
          >
            {(() => {
              const escMessage = ESCALATION_MESSAGE[card.escalation];
              return escMessage ? (
                <FormattedMessage {...escMessage} />
              ) : (
                card.escalation
              );
            })()}
          </Label>
        </StackItem>
      ) : null}
      {card.rework !== null ? (
        <StackItem>
          <ReworkMeter rework={card.rework} />
        </StackItem>
      ) : null}
    </Stack>
  );
}

function AgingColumn({
  phase,
  cards,
}: {
  phase: AgentSdlcPhase;
  cards: readonly AgentSdlcCard[];
}): React.ReactElement {
  const intl = useIntl();
  const scale = agingScaleSeconds(phase, cards);
  const sleRatio =
    phase.sleSeconds > 0 ? agingRatio(phase.sleSeconds, scale) : null;
  const phaseMessage = PHASE_MESSAGE[phase.key];
  const phaseLabel = phaseMessage
    ? intl.formatMessage(phaseMessage)
    : phase.key;
  return (
    <div
      className={styles.column}
      role="group"
      aria-label={intl.formatMessage(messages.sdlcColumnAria, {
        phase: phaseLabel,
        count: cards.length,
      })}
    >
      <div className={styles.columnHeader}>
        <span>{phaseLabel}</span>
        <Label isCompact>{cards.length}</Label>
      </div>
      <div className={styles.track}>
        {sleRatio !== null ? (
          <>
            <div
              className={styles.sleLine}
              style={{ top: `${String(sleRatio * 90)}%` }}
              aria-hidden
            />
            <span
              className={styles.sleTag}
              style={{ top: `${String(sleRatio * 90)}%` }}
            >
              <FormattedMessage {...messages.sdlcSleLine} />
            </span>
          </>
        ) : null}
        {cards.length === 0 ? (
          <Content component="small">
            <FormattedMessage {...messages.sdlcBoardEmpty} />
          </Content>
        ) : (
          cards.map((card) => (
            <div
              key={card.number}
              className={styles.card}
              style={{
                top: `${String(agingRatio(card.timeInPhaseSeconds, scale) * 90)}%`,
              }}
            >
              <CardBodyContent card={card} showEscalation={false} />
            </div>
          ))
        )}
      </div>
    </div>
  );
}

function EscalationLane({
  lane,
}: {
  lane: readonly AgentSdlcCard[];
}): React.ReactElement {
  return (
    <Stack hasGutter>
      <StackItem>
        <Title headingLevel="h3" size="md">
          <Inline>
            <FormattedMessage {...messages.sdlcLaneTitle} />
            <Label isCompact>{lane.length}</Label>
          </Inline>
        </Title>
      </StackItem>
      <StackItem>
        {lane.length === 0 ? (
          <Content component="p">
            <Inline>
              <CheckCircleIcon aria-hidden />
              <FormattedMessage {...messages.sdlcLaneCleared} />
            </Inline>
          </Content>
        ) : (
          <div className={styles.lane}>
            {lane.map((card) => (
              <div key={card.number} className={styles.laneCard}>
                <CardBodyContent card={card} showEscalation />
              </div>
            ))}
          </div>
        )}
      </StackItem>
    </Stack>
  );
}

function ShippedMatrix({
  data,
  selectedBundleSeed,
  onSelectBundle,
  onJumpToMap,
  nowMs,
}: AgentSdlcViewProps): React.ReactElement {
  const intl = useIntl();
  const mergedAgo = (iso: string | null): string | null =>
    formatMergedAgo(intl, iso, nowMs);
  const [expanded, setExpanded] = useState<number | null>(null);
  const { hubClusters, rows } = data.matrix;
  const columnCount = 2 + hubClusters.length + 1;

  const openBundle = (row: AgentSdlcBundleRow) => {
    const seed = bundleRowSeed(row);
    if (seed === null) {
      return;
    }
    onSelectBundle(seed);
    onJumpToMap();
  };

  return (
    <Stack hasGutter>
      <StackItem>
        <Title headingLevel="h3" size="md">
          <FormattedMessage {...messages.sdlcMatrixTitle} />
        </Title>
      </StackItem>
      <StackItem>
        {rows.length === 0 ? (
          <Content component="p">
            <FormattedMessage {...messages.sdlcMatrixEmpty} />
          </Content>
        ) : (
          <Table
            aria-label={intl.formatMessage(messages.sdlcMatrixTitle)}
            variant="compact"
          >
            <Thead>
              <Tr>
                <Th screenReaderText="expand" />
                <Th>
                  <FormattedMessage {...messages.sdlcMatrixTitle} />
                </Th>
                {hubClusters.map((hc) => (
                  <Th key={hc.key}>{hc.cluster ?? hc.key}</Th>
                ))}
                <Th>
                  <FormattedMessage {...messages.sectionInBundle} />
                </Th>
              </Tr>
            </Thead>
            {rows.map((row, rowIndex) => {
              const seed = bundleRowSeed(row);
              const selected = seed !== null && seed === selectedBundleSeed;
              const isExpanded = expanded === rowIndex;
              return (
                <Tbody
                  key={
                    row.digest ??
                    row.version ??
                    `unreleased-${String(rowIndex)}`
                  }
                  isExpanded={isExpanded}
                >
                  <Tr isRowSelected={selected}>
                    <Td
                      expand={{
                        rowIndex,
                        isExpanded,
                        onToggle: () => {
                          setExpanded(isExpanded ? null : rowIndex);
                        },
                      }}
                    />
                    <Td>
                      {row.unreleased ? (
                        <FormattedMessage {...messages.sdlcUnreleased} />
                      ) : seed !== null ? (
                        <Button
                          variant="link"
                          isInline
                          icon={<ArrowRightIcon />}
                          iconPosition="end"
                          onClick={() => {
                            openBundle(row);
                          }}
                        >
                          {row.version ?? seed}
                        </Button>
                      ) : (
                        (row.version ?? "")
                      )}
                      {row.date ? (
                        <>
                          {" "}
                          <ReleaseTime iso={row.date} mode="relative" />
                        </>
                      ) : null}
                    </Td>
                    {hubClusters.map((hc) => {
                      const current = row.hubClusters.includes(hc.key);
                      return (
                        <Td key={hc.key} dataLabel={hc.cluster ?? hc.key}>
                          {current ? (
                            <span
                              aria-label={intl.formatMessage(
                                messages.sdlcMatrixCurrent,
                                { cluster: hc.cluster ?? hc.key },
                              )}
                            >
                              <Inline>
                                <CheckCircleIcon
                                  aria-hidden
                                  style={{ color: TONE_COLOR.success }}
                                />
                                <FormattedMessage
                                  {...messages.sdlcCurrentMarker}
                                />
                              </Inline>
                            </span>
                          ) : null}
                        </Td>
                      );
                    })}
                    <Td>
                      <Label isCompact>{row.issues.length}</Label>
                    </Td>
                  </Tr>
                  <Tr isExpanded={isExpanded}>
                    <Td />
                    <Td colSpan={columnCount - 1}>
                      <ExpandableRowContent>
                        <MatrixIssues
                          issues={row.issues}
                          mergedAgo={mergedAgo}
                        />
                      </ExpandableRowContent>
                    </Td>
                  </Tr>
                </Tbody>
              );
            })}
          </Table>
        )}
      </StackItem>
    </Stack>
  );
}

function MatrixIssues({
  issues,
  mergedAgo,
}: {
  issues: readonly AgentSdlcShippedIssue[];
  mergedAgo: (iso: string | null) => string | null;
}): React.ReactElement {
  if (issues.length === 0) {
    return (
      <Content component="small">
        <FormattedMessage {...messages.sdlcBoardEmpty} />
      </Content>
    );
  }
  return (
    <Stack>
      {issues.map((issue) => {
        const ago = mergedAgo(issue.mergedAt);
        const issueLabel = `#${String(issue.number)} ${issue.title}`;
        return (
          // key={issue.number} means React mounts a fresh node only for a NEWLY
          // shipped issue, so the ship-in animation (SDLC-VIS-12) plays just for
          // it; existing rows keep their DOM. The animation is disabled under
          // prefers-reduced-motion in the stylesheet.
          <StackItem key={issue.number} className={styles.shipEnter}>
            <Button
              component="a"
              variant="link"
              isInline
              href={issue.url}
              target="_blank"
              rel="noreferrer"
            >
              {issueLabel}
            </Button>
            {ago ? <Content component="small">{ago}</Content> : null}
          </StackItem>
        );
      })}
    </Stack>
  );
}

export function AgentSdlcView(props: AgentSdlcViewProps): React.ReactElement {
  const { data } = props;
  return (
    <Stack hasGutter>
      <StackItem>
        <div className={styles.board}>
          {boardPhases(data.phases).map((phase) => (
            <AgingColumn
              key={phase.key}
              phase={phase}
              cards={cardsInPhase(data.cards, phase.key)}
            />
          ))}
        </div>
      </StackItem>
      <StackItem>
        <EscalationLane lane={data.lane} />
      </StackItem>
      <StackItem>
        <ShippedMatrix {...props} />
      </StackItem>
    </Stack>
  );
}
