// The Agent SDLC view: an aging work-in-progress board (time-in-phase on the
// vertical axis), a rework meter per in-review card, an escalation lane, and a
// release-by-hub-cluster matrix (agent-sdlc-visibility-dashboard.spec). Clicking a
// card opens a details drawer with its full label history. It holds no fleet
// knowledge: phase keys are the funnel's own vocabulary and everything else
// (titles, versions, hub-cluster names, digests) is runtime data from /api/sdlc.
//
// Motion (SDLC-VIS-08/12) is driven imperatively by an rAF hook so cards ease into
// place (collision-packed, no overlap) and drift as they age - anchored to the last
// server value so drift is independent of the poll and corrects (not double-counts)
// on refresh - plus a shipped issue flies into the matrix. All suppressed under
// prefers-reduced-motion.
//
// Every colour signal is paired with text or an icon so colour is never the sole
// channel (SDLC-VIS-16 / accessibility.spec).

import {
  Alert,
  Button,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Divider,
  Drawer,
  DrawerActions,
  DrawerCloseButton,
  DrawerContent,
  DrawerContentBody,
  DrawerHead,
  DrawerPanelBody,
  DrawerPanelContent,
  Label,
  MenuToggle,
  NotificationBadge,
  NotificationDrawer,
  NotificationDrawerBody,
  NotificationDrawerGroup,
  NotificationDrawerGroupList,
  NotificationDrawerHeader,
  NotificationDrawerList,
  NotificationDrawerListItem,
  NotificationDrawerListItemBody,
  NotificationDrawerListItemHeader,
  Popover,
  SearchInput,
  Select,
  SelectList,
  SelectOption,
  Stack,
  StackItem,
  TextInputGroup,
  TextInputGroupMain,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarFilter,
  ToolbarGroup,
  ToolbarItem,
  ToolbarToggleGroup,
  Tooltip,
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
import AngleLeftIcon from "@patternfly/react-icons/dist/esm/icons/angle-left-icon";
import ArrowRightIcon from "@patternfly/react-icons/dist/esm/icons/arrow-right-icon";
import CheckCircleIcon from "@patternfly/react-icons/dist/esm/icons/check-circle-icon";
import CheckIcon from "@patternfly/react-icons/dist/esm/icons/check-icon";
import ClockIcon from "@patternfly/react-icons/dist/esm/icons/clock-icon";
import CodeIcon from "@patternfly/react-icons/dist/esm/icons/code-icon";
import CopyIcon from "@patternfly/react-icons/dist/esm/icons/copy-icon";
import ExclamationCircleIcon from "@patternfly/react-icons/dist/esm/icons/exclamation-circle-icon";
import ExclamationTriangleIcon from "@patternfly/react-icons/dist/esm/icons/exclamation-triangle-icon";
import ExternalLinkAltIcon from "@patternfly/react-icons/dist/esm/icons/external-link-alt-icon";
import FilterIcon from "@patternfly/react-icons/dist/esm/icons/filter-icon";
import InboxIcon from "@patternfly/react-icons/dist/esm/icons/inbox-icon";
import LayerGroupIcon from "@patternfly/react-icons/dist/esm/icons/layer-group-icon";
import ListIcon from "@patternfly/react-icons/dist/esm/icons/list-icon";
import PencilAltIcon from "@patternfly/react-icons/dist/esm/icons/pencil-alt-icon";
import QuestionCircleIcon from "@patternfly/react-icons/dist/esm/icons/question-circle-icon";
import RobotIcon from "@patternfly/react-icons/dist/esm/icons/robot-icon";
import ShippingFastIcon from "@patternfly/react-icons/dist/esm/icons/shipping-fast-icon";
import type { SVGIconProps } from "@patternfly/react-icons/dist/esm/createIcon";
import type { ComponentType } from "react";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import type { IntlShape, MessageDescriptor } from "react-intl";
import { FormattedMessage, useIntl } from "react-intl";

import {
  agingRatio,
  boardPhases,
  bundlePRCount,
  bundleRowExpandKey,
  bundleRowSeed,
  cardsInPhase,
  computeCardFlights,
  ESCALATION_BLOCKED,
  ESCALATION_DUPLICATE,
  ESCALATION_NEEDS_INPUT,
  ESCALATION_PAST_SLE,
  globalAgingScaleSeconds,
  reworkRatio,
  reworkSeverity,
  type AgentSdlcBundleRow,
  type AgentSdlcCard,
  type AgentSdlcData,
  type AgentSdlcLabelEvent,
  type CardFlight,
  type AgentSdlcPhase,
  type AgentSdlcPullRequest,
  type AgentSdlcRework,
  type AgentSdlcShippedIssue,
} from "../../../domain/agent-sdlc";
import { messages } from "../../../messages";
import { useRoute, useRouter } from "../../routing/router";
import { NavLinkButton } from "../nav-link-button";
import type { SdlcAgentMetrics } from "../../../domain/sdlc-agents";
import { TEXT_COLOR, TONE_COLOR, USER_COLOR } from "../map/colors";
import { Identicon } from "../map/identicon";
import { MetricTile } from "../map/metric-tiles";
import tileStyles from "../map/map-details.module.css";
import { ReleaseTime } from "../map/release-time";
import { identiName } from "../../../domain/map/identiname";
import {
  arcControl,
  arcTravelVars,
  DUST,
  DUST_MS,
  FLIGHT_MS,
  prefersReducedMotion,
} from "../shared/flight-motion";
import motion from "../shared/flight-motion.module.css";
import styles from "./agent-sdlc-view.module.css";

export interface AgentSdlcViewProps {
  readonly data: AgentSdlcData;
  /** The shared freight-bar bundle seed, used for the matrix current-row highlight. */
  readonly selectedBundleSeed: string | null;
  /** Runtime telemetry per agent, keyed by name. Optional: columns degrade to the
   *  phase name alone when absent (SDLC-AGT-08). */
  readonly agents?: ReadonlyMap<string, SdlcAgentMetrics>;
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

type IconComp = ComponentType<SVGIconProps>;

/** A step icon per funnel phase (decorative; the phase name is always shown too). */
const PHASE_ICON: Record<string, IconComp> = {
  intake: InboxIcon,
  workable: ListIcon,
  spec: PencilAltIcon,
  "spec-approved": CheckIcon,
  code: CodeIcon,
  "code-approved": CheckCircleIcon,
  shipped: ShippingFastIcon,
};

/** The funnel agent that acts on items in each phase (moves them to the next
 *  state). The phase name is always shown too, so this is an annotation, not the
 *  sole signal. Data vocabulary, not copy: the names are the funnel's own. */
const PHASE_AGENT: Record<string, string> = {
  intake: "triage",
  workable: "spec-implementation",
  spec: "spec-review",
  "spec-approved": "code-implementation",
  code: "code-review",
  "code-approved": "release-verification",
};

/** Approved steps read in success green; the rest stay subtle so colour is accent,
 *  not noise (and never the sole signal - the label sits beside the icon). */
function phaseIconTone(key: string): string {
  return key === "spec-approved" || key === "code-approved"
    ? TONE_COLOR.success
    : "var(--pf-t--global--icon--color--subtle, #9a9da0)";
}

const ESCALATION_MESSAGE: Record<string, MessageDescriptor> = {
  [ESCALATION_BLOCKED]: messages.sdlcEscalationBlocked,
  [ESCALATION_NEEDS_INPUT]: messages.sdlcEscalationNeedsInput,
  [ESCALATION_DUPLICATE]: messages.sdlcEscalationDuplicate,
  [ESCALATION_PAST_SLE]: messages.sdlcEscalationPastSle,
};

// The icon is coloured on the element itself (not a wrapper span) so PatternFly's
// Label icon slot keeps it vertically centred (SDLC-VIS-16 colour+icon pairing).
function escalationIcon(
  escalation: string,
  color?: string,
): React.ReactElement {
  const style = color ? { color } : undefined;
  switch (escalation) {
    case ESCALATION_BLOCKED:
      return <ExclamationCircleIcon style={style} />;
    case ESCALATION_NEEDS_INPUT:
      return <QuestionCircleIcon style={style} />;
    case ESCALATION_DUPLICATE:
      return <CopyIcon style={style} />;
    default:
      return <ClockIcon style={style} />;
  }
}

/** The phase step icon as an element, coloured on the icon so a Label keeps it
 *  centred. Returns undefined for an unknown phase. */
function phaseIconEl(key: string): React.ReactElement | undefined {
  const Icon = PHASE_ICON[key];
  if (!Icon) {
    return undefined;
  }
  return <Icon style={{ color: phaseIconTone(key) }} />;
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

/** Inline icon+text group with token spacing (no literal space node needed). */
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

/** A bundle's deterministic identicon (reuses the map's seed + glyph so a bundle
 *  looks identical in the freight bar, the map, and this matrix). */
function BundleIdenticon({
  seed,
  size = 20,
}: {
  seed: string;
  size?: number;
}): React.ReactElement {
  return (
    <svg
      width={size}
      height={size}
      className={styles.identicon}
      aria-hidden
      style={{ verticalAlign: "middle" }}
    >
      <Identicon seed={seed} x={0} y={0} size={size} />
    </svg>
  );
}

function formatAge(intl: IntlShape, seconds: number): string {
  const total = Math.max(seconds, 0);
  const days = Math.floor(total / 86_400);
  const hours = Math.floor((total % 86_400) / 3_600);
  const minutes = Math.floor((total % 3_600) / 60);
  // Show at most two adjacent units, down to minutes, so a young card reads "11m"
  // or "3h 20m" rather than a misleading "0d 0h".
  if (days > 0) {
    return intl.formatMessage(messages.sdlcAge, { days, hours });
  }
  if (hours > 0) {
    return intl.formatMessage(messages.sdlcAgeHm, { hours, minutes });
  }
  return intl.formatMessage(messages.sdlcAgeM, { minutes });
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

/** "merged N days ago" plus " by {user}" when the merging user is known. */
function formatMergedLine(
  intl: IntlShape,
  issue: AgentSdlcShippedIssue,
  nowMs?: number,
): string | null {
  const ago = formatMergedAgo(intl, issue.mergedAt, nowMs);
  if (ago === null) {
    return null;
  }
  return issue.mergedBy
    ? intl.formatMessage(messages.sdlcMergedBy, { ago, user: issue.mergedBy })
    : ago;
}

function reworkTone(rework: AgentSdlcRework): string {
  const severity = reworkSeverity(rework);
  return severity === "critical"
    ? TONE_COLOR.danger
    : severity === "warning"
      ? TONE_COLOR.warning
      : TONE_COLOR.info;
}

function ReworkMeter({
  rework,
}: {
  rework: AgentSdlcRework;
}): React.ReactElement {
  const intl = useIntl();
  const severity = reworkSeverity(rework);
  const tone = reworkTone(rework);
  const label = intl.formatMessage(messages.sdlcReworkLabel, {
    count: rework.count,
    threshold: rework.threshold,
  });
  return (
    <Tooltip
      content={intl.formatMessage(messages.sdlcReworkTooltip, {
        count: rework.count,
        threshold: rework.threshold,
      })}
    >
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
        <Content component="small" style={{ display: "block" }}>
          {label}
        </Content>
        {severity === "critical" || severity === "warning" ? (
          <Content component="small" style={{ display: "block" }}>
            <Inline>
              <ExclamationTriangleIcon aria-hidden style={{ color: tone }} />
              {rework.count >= rework.threshold ? (
                <FormattedMessage {...messages.sdlcReworkBlocked} />
              ) : (
                <FormattedMessage
                  {...messages.sdlcReworkRemaining}
                  values={{ remaining: rework.threshold - rework.count }}
                />
              )}
            </Inline>
          </Content>
        ) : null}
      </div>
    </Tooltip>
  );
}

/** One node in the collapsed history timeline. */
interface TimelineNode {
  readonly label: string;
  readonly count: number; // >1 for a collapsed reject -> retry run
  readonly at: string;
}

const REJECT_LABELS = new Set([
  "agent/review-spec-rejected",
  "agent/review-code-rejected",
]);

/**
 * Collapse the raw label-event history into a scannable timeline (Tufte: erase
 * redundant data-ink): drop the unlabel half of each transition (keep state
 * entries), and roll a reject -> retry loop (alternating review-*-rejected /
 * reviewable-*) up into one "xN" node.
 */
function timelineNodes(
  history: readonly AgentSdlcLabelEvent[],
): TimelineNode[] {
  const ev = history.filter((e) => e.action === "labeled");
  const nodes: TimelineNode[] = [];
  let i = 0;
  while (i < ev.length) {
    const cur = ev[i];
    if (!cur) {
      break;
    }
    if (REJECT_LABELS.has(cur.label)) {
      const rejectLabel = cur.label;
      const retryLabel = rejectLabel.includes("code")
        ? "agent/reviewable-code"
        : "agent/reviewable-spec";
      let count = 0;
      let last = cur;
      while (i < ev.length) {
        const e = ev[i];
        if (!e || (e.label !== rejectLabel && e.label !== retryLabel)) {
          break;
        }
        if (e.label === rejectLabel) {
          count++;
          last = e;
        }
        i++;
      }
      nodes.push({ label: rejectLabel, count, at: last.at });
      continue;
    }
    nodes.push({ label: cur.label, count: 1, at: cur.at });
    i++;
  }
  return nodes;
}

/** Node colour by event kind; always paired with the label text, never color-only. */
function eventTone(label: string): string {
  if (label.includes("rejected") || label === "agent/blocked") {
    return TONE_COLOR.danger;
  }
  if (label.includes("approved") || label === "agent/release-verified") {
    return TONE_COLOR.success;
  }
  if (label === "agent/needs-input" || label === "agent/duplicate") {
    return TONE_COLOR.warning;
  }
  return TONE_COLOR.info;
}

function LabelHistory({ card }: { card: AgentSdlcCard }): React.ReactElement {
  const nodes = timelineNodes(card.history);
  if (nodes.length === 0) {
    return (
      <Content component="small">
        <FormattedMessage {...messages.sdlcHistoryEmpty} />
      </Content>
    );
  }
  return (
    <div className={styles.timeline}>
      {nodes
        .slice()
        .reverse()
        .map((n, i) => (
          <div
            className={styles.tlNode}
            key={`${n.at}:${n.label}:${String(i)}`}
          >
            <span
              className={styles.tlDot}
              style={{ background: eventTone(n.label) }}
              aria-hidden
            />
            <div className={styles.tlMain}>
              <span className={styles.tlLabel}>
                {n.label.replace(/^agent\//, "")}
                {n.count > 1 ? (
                  <span className={styles.tlCount}>
                    <FormattedMessage
                      {...messages.sdlcReworkCount}
                      values={{ count: n.count }}
                    />
                  </span>
                ) : null}
              </span>
              {n.at ? (
                <ReleaseTime
                  iso={n.at}
                  mode="relative"
                  className={styles.tlTime}
                />
              ) : null}
            </div>
          </div>
        ))}
    </div>
  );
}

/** A compact board card: a clickable title (opens the details drawer), the age,
 *  and (in review) the single rework bar. Kept short so collision stays legible. */
function AgingCard({
  card,
  top,
  height,
  onSelect,
}: {
  card: AgentSdlcCard;
  top: number;
  height: number;
  onSelect: (card: AgentSdlcCard) => void;
}): React.ReactElement {
  const intl = useIntl();
  const toneClass =
    card.rework === null
      ? ""
      : reworkSeverity(card.rework) === "critical"
        ? styles.toneDanger
        : reworkSeverity(card.rework) === "warning"
          ? styles.toneWarning
          : "";
  const issueLabel = `#${String(card.number)} ${card.title}`;
  const tabColor =
    card.rework === null
      ? TONE_COLOR.info
      : reworkSeverity(card.rework) === "critical"
        ? TONE_COLOR.danger
        : reworkSeverity(card.rework) === "warning"
          ? TONE_COLOR.warning
          : TONE_COLOR.info;
  return (
    <div
      className={[styles.card, toneClass].filter(Boolean).join(" ")}
      style={{ top: `${String(top)}px`, height: `${String(height)}px` }}
      // Queryable anchor so the flight overlay can measure this card's source/dest rect
      // and clone it at 1:1 when its phase changes (SDLC-VIS-21 position anchors).
      data-card-number={card.number}
    >
      {/* Time-anchor tab (Google-calendar style): marks that the card's TOP edge is
          its point in time on the aging axis. */}
      <span
        className={styles.timeTab}
        style={{ background: tabColor }}
        aria-hidden
      />
      <Button
        variant="link"
        isInline
        className={styles.cardTitle}
        onClick={() => {
          onSelect(card);
        }}
        aria-label={intl.formatMessage(messages.sdlcCardOpen, {
          number: card.number,
          title: card.title,
        })}
      >
        {issueLabel}
      </Button>
      {card.rework !== null ? <ReworkMeter rework={card.rework} /> : null}
      <div className={styles.cardMeta}>
        <Content component="small">
          <Inline>
            <ClockIcon aria-hidden />
            {formatAge(intl, card.timeInPhaseSeconds)}
          </Inline>
        </Content>
      </div>
    </div>
  );
}

/** Measures an element's live height so the column can be packed in React. */
function useElementHeight(ref: React.RefObject<HTMLElement | null>): number {
  const [height, setHeight] = useState(0);
  useEffect(() => {
    const el = ref.current;
    if (!el) {
      return;
    }
    if (typeof ResizeObserver === "undefined") {
      setHeight(el.clientHeight);
      return;
    }
    const ro = new ResizeObserver((entries) => {
      for (const e of entries) {
        setHeight(e.contentRect.height);
      }
    });
    ro.observe(el);
    setHeight(el.clientHeight);
    return () => {
      ro.disconnect();
    };
  }, [ref]);
  return height;
}

const PACK_PAD = 6;
const PACK_GAP = 8;
const CLUSTER_H = 34;

/** Deterministic card heights so a column packs without measuring every card. */
function cardHeight(card: AgentSdlcCard): number {
  if (card.rework === null) {
    return 84;
  }
  return reworkSeverity(card.rework) === "ok" ? 112 : 132;
}

interface PlacedCard {
  readonly card: AgentSdlcCard;
  readonly top: number;
  readonly height: number;
}
interface CardCluster {
  readonly top: number;
  readonly cards: readonly AgentSdlcCard[];
}

/**
 * Packs a column by TRUE time-in-phase (SDLC-VIS-08): a card that fits at its age is
 * placed there; a run that would otherwise overlap collapses into one cluster chip
 * at that band rather than being pushed down into older/past-SLE territory. Fresh at
 * the top, stale lower, and the time axis stays honest.
 */
function packColumn(
  cards: readonly AgentSdlcCard[],
  scale: number,
  usable: number,
): { placed: PlacedCard[]; clusters: CardCluster[] } {
  const sorted = [...cards].sort(
    (a, b) => a.timeInPhaseSeconds - b.timeInPhaseSeconds,
  );
  const placed: PlacedCard[] = [];
  const clusters: CardCluster[] = [];
  const trueTopOf = (c: AgentSdlcCard): number => {
    const span = Math.max(usable - cardHeight(c) - PACK_PAD * 2, 0);
    return PACK_PAD + agingRatio(c.timeInPhaseSeconds, scale) * span;
  };
  let cursor = PACK_PAD;
  let i = 0;
  while (i < sorted.length) {
    const c = sorted[i];
    if (!c) {
      break;
    }
    if (trueTopOf(c) >= cursor) {
      const h = cardHeight(c);
      placed.push({ card: c, top: trueTopOf(c), height: h });
      cursor = trueTopOf(c) + h + PACK_GAP;
      i += 1;
      continue;
    }
    // overlap run: cards that cannot sit at their true age without displacement
    const run: AgentSdlcCard[] = [];
    while (i < sorted.length) {
      const cc = sorted[i];
      if (!cc || trueTopOf(cc) >= cursor) {
        break;
      }
      run.push(cc);
      i += 1;
    }
    const first = run[0];
    if (run.length === 1 && first) {
      const h = cardHeight(first);
      placed.push({ card: first, top: cursor, height: h });
      cursor += h + PACK_GAP;
    } else {
      clusters.push({ top: cursor, cards: run });
      cursor += CLUSTER_H + PACK_GAP;
    }
  }
  return { placed, clusters };
}

/** A collapsed band of near-same-age cards; click to list them (no vertical
 *  displacement, so the axis never implies they are past SLE). */
function ClusterChip({
  cluster,
  onSelect,
}: {
  cluster: CardCluster;
  onSelect: (card: AgentSdlcCard) => void;
}): React.ReactElement {
  const intl = useIntl();
  const ages = cluster.cards.map((c) => c.timeInPhaseSeconds);
  return (
    <Popover
      headerContent={
        <FormattedMessage
          {...messages.sdlcClusterTitle}
          values={{ count: cluster.cards.length }}
        />
      }
      bodyContent={
        <Stack hasGutter>
          {cluster.cards.map((c) => (
            <StackItem key={c.number}>
              <Button
                variant="link"
                isInline
                onClick={() => {
                  onSelect(c);
                }}
              >
                {intl.formatMessage(messages.sdlcIssueLabel, {
                  number: c.number,
                  title: c.title,
                })}
              </Button>
            </StackItem>
          ))}
        </Stack>
      }
    >
      <button
        type="button"
        className={styles.clusterChip}
        style={{ top: `${String(cluster.top)}px` }}
      >
        <Inline>
          <LayerGroupIcon aria-hidden />
          {intl.formatMessage(messages.sdlcClusterChip, {
            count: cluster.cards.length,
            from: formatAge(intl, Math.min(...ages)),
            to: formatAge(intl, Math.max(...ages)),
          })}
        </Inline>
      </button>
    </Popover>
  );
}

/** Translate a vertical mouse wheel into horizontal scroll so a plain mouse (no
 *  horizontal wheel/trackpad) can page through the board columns. Non-passive so it
 *  can preventDefault the page scroll it redirects (mirrors the map's freight bar). */
function useHorizontalWheel(ref: React.RefObject<HTMLDivElement | null>): void {
  useEffect(() => {
    const el = ref.current;
    if (!el) {
      return;
    }
    const onWheel = (e: WheelEvent) => {
      // Never hijack a zoom (ctrl/cmd + wheel) or an explicit horizontal gesture
      // (shift + wheel / trackpad) - let the browser handle those natively.
      if (e.ctrlKey || e.metaKey || e.shiftKey) {
        return;
      }
      if (Math.abs(e.deltaY) <= Math.abs(e.deltaX)) {
        return;
      }
      if (el.scrollWidth <= el.clientWidth) {
        return;
      }
      el.scrollLeft += e.deltaY;
      e.preventDefault();
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => {
      el.removeEventListener("wheel", onWheel);
    };
  }, [ref]);
}

const QUEUE_WARN = 8;
const METRIC_SUBTLE = "var(--pf-t--global--text--color--subtle, #9a9da0)";

function ColumnAgentMetrics({
  agent,
}: {
  readonly agent: SdlcAgentMetrics;
}): React.ReactElement {
  const failing = agent.itemsFailed;
  const queueTone =
    agent.queueDepth >= QUEUE_WARN ? TONE_COLOR.warning : USER_COLOR;
  return (
    <div className={styles.columnMetrics}>
      <span
        className={styles.columnMetric}
        style={{
          color: agent.replicasOn > 0 ? TONE_COLOR.success : METRIC_SUBTLE,
        }}
      >
        <span className={styles.columnMetricLabel}>
          <FormattedMessage {...messages.columnMetricOnLabel} />
        </span>
        <FormattedMessage
          {...messages.columnMetricRatio}
          values={{ on: agent.replicasOn, total: agent.replicasTotal }}
        />
      </span>
      <span
        className={styles.columnMetric}
        style={{
          color: agent.queueDepth > 0 ? queueTone : METRIC_SUBTLE,
        }}
      >
        <span className={styles.columnMetricLabel}>
          <FormattedMessage {...messages.columnMetricQueueLabel} />
        </span>
        {agent.queueDepth}
      </span>
      <span
        className={styles.columnMetric}
        style={{
          color: failing > 0 ? TONE_COLOR.danger : METRIC_SUBTLE,
        }}
      >
        <span className={styles.columnMetricLabel}>
          <FormattedMessage {...messages.columnMetricFailLabel} />
        </span>
        {failing}
      </span>
    </div>
  );
}

function AgingColumn({
  phase,
  cards,
  scale,
  onSelect,
  agentSummary,
  onSelectAgent,
}: {
  phase: AgentSdlcPhase;
  cards: readonly AgentSdlcCard[];
  scale: number;
  onSelect: (card: AgentSdlcCard) => void;
  agentSummary?: SdlcAgentMetrics;
  onSelectAgent?: (name: string) => void;
}): React.ReactElement {
  const intl = useIntl();
  const sleRatio =
    phase.sleSeconds > 0 ? agingRatio(phase.sleSeconds, scale) : null;
  const phaseMessage = PHASE_MESSAGE[phase.key];
  const phaseLabel = phaseMessage
    ? intl.formatMessage(phaseMessage)
    : phase.key;
  const PhaseIcon = PHASE_ICON[phase.key];
  const agent = PHASE_AGENT[phase.key];
  const trackRef = useRef<HTMLDivElement>(null);
  const trackHeight = useElementHeight(trackRef);
  const { placed, clusters } = packColumn(cards, scale, trackHeight || 420);
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
        <div className={styles.columnHeaderMain}>
          {agent !== undefined && agentSummary !== undefined ? (
            <button
              type="button"
              className={styles.agentButton}
              onClick={() => {
                onSelectAgent?.(agent);
              }}
              aria-label={intl.formatMessage(messages.sdlcAgentDetailAria, {
                agent,
              })}
            >
              <RobotIcon aria-hidden />
              <span className={styles.agentName}>{agent}</span>
            </button>
          ) : agent ? (
            <span className={styles.agentNamePlain}>
              <RobotIcon aria-hidden />
              <span className={styles.agentName}>{agent}</span>
            </span>
          ) : (
            <Inline>
              {PhaseIcon ? (
                <span
                  style={{
                    color: phaseIconTone(phase.key),
                    display: "inline-flex",
                  }}
                >
                  <PhaseIcon aria-hidden />
                </span>
              ) : null}
              <span>{phaseLabel}</span>
            </Inline>
          )}
          <Label isCompact>{cards.length}</Label>
        </div>
        <div className={styles.columnPhase}>
          <Inline>
            {PhaseIcon ? <PhaseIcon aria-hidden /> : null}
            <span>{phaseLabel}</span>
          </Inline>
        </div>
        {agentSummary ? <ColumnAgentMetrics agent={agentSummary} /> : null}
      </div>
      <div className={styles.track} ref={trackRef} data-phase-col={phase.key}>
        {sleRatio !== null ? (
          <>
            <div
              className={styles.sleLine}
              style={{ top: `${String(sleRatio * 90)}%` }}
              aria-hidden
            />
            <Tooltip
              content={intl.formatMessage(messages.sdlcSleTooltip, {
                duration: formatAge(intl, phase.sleSeconds),
              })}
            >
              <span
                className={styles.sleTag}
                style={{ top: `${String(sleRatio * 90)}%` }}
                aria-label={intl.formatMessage(messages.sdlcSleLine)}
              >
                <ExclamationTriangleIcon
                  aria-hidden
                  style={{ color: TONE_COLOR.warning }}
                />
                <FormattedMessage {...messages.sdlcSleShort} />
              </span>
            </Tooltip>
          </>
        ) : null}
        {cards.length === 0 ? (
          <div className={styles.trackEmpty}>
            <Content component="small">
              <FormattedMessage {...messages.sdlcBoardEmpty} />
            </Content>
          </div>
        ) : (
          <>
            {placed.map((p) => (
              <AgingCard
                key={p.card.number}
                card={p.card}
                top={p.top}
                height={p.height}
                onSelect={onSelect}
              />
            ))}
            {clusters.map((cl) => (
              // Key by the stacked issues, not the position: live aging moves the
              // stack every second, and a position key would remount the chip and
              // close its open popover.
              <ClusterChip
                key={`cluster-${cl.cards.map((c) => String(c.number)).join("-")}`}
                cluster={cl}
                onSelect={onSelect}
              />
            ))}
          </>
        )}
      </div>
    </div>
  );
}

// Escalation buckets ordered by urgency for the notification drawer: the
// human-action buckets (blocked, needs-input) on top, then duplicate, then the
// lower-priority past-SLE (SDLC-VIS-13).
const ESC_ORDER = [
  ESCALATION_BLOCKED,
  ESCALATION_NEEDS_INPUT,
  ESCALATION_DUPLICATE,
  ESCALATION_PAST_SLE,
];

type NotifVariant = "danger" | "warning" | "info" | "custom";
function escVariant(escalation: string): NotifVariant {
  switch (escalation) {
    case ESCALATION_BLOCKED:
      return "danger";
    case ESCALATION_NEEDS_INPUT:
      return "info";
    case ESCALATION_PAST_SLE:
      return "warning";
    default:
      return "custom";
  }
}

/** Bell variant: attention when a human is required (blocked / needs-input),
 *  unread when only lower-priority (duplicate / past-SLE) items remain, else read. */
function escalationBadgeVariant(
  lane: readonly AgentSdlcCard[],
): "read" | "unread" | "attention" {
  if (lane.length === 0) {
    return "read";
  }
  const urgent = lane.some(
    (c) =>
      c.escalation === ESCALATION_BLOCKED ||
      c.escalation === ESCALATION_NEEDS_INPUT,
  );
  return urgent ? "attention" : "unread";
}

/** The escalations notification drawer: every blocked/paused/past-SLE item grouped
 *  by type, human-action buckets first (SDLC-VIS-13). Selecting one opens its
 *  details. */
function NotificationsPanel({
  lane,
  onClose,
  onSelect,
}: {
  lane: readonly AgentSdlcCard[];
  onClose: () => void;
  onSelect: (card: AgentSdlcCard) => void;
}): React.ReactElement {
  const intl = useIntl();
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set());
  const toggleGroup = (key: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };
  const groups = ESC_ORDER.map((key) => ({
    key,
    items: lane.filter((c) => c.escalation === key),
  })).filter((g) => g.items.length > 0);
  return (
    <DrawerPanelContent isResizable defaultSize="33%" minSize="360px">
      <NotificationDrawer>
        <NotificationDrawerHeader
          count={lane.length}
          title={intl.formatMessage(messages.sdlcNotificationsTitle)}
        >
          <DrawerCloseButton onClick={onClose} />
        </NotificationDrawerHeader>
        <NotificationDrawerBody>
          {lane.length === 0 ? (
            <Content
              component="p"
              style={{ padding: "var(--pf-t--global--spacer--md, 16px)" }}
            >
              <Inline>
                <CheckCircleIcon
                  aria-hidden
                  style={{ color: TONE_COLOR.success }}
                />
                <FormattedMessage {...messages.sdlcLaneCleared} />
              </Inline>
            </Content>
          ) : (
            <NotificationDrawerGroupList>
              {groups.map((g) => {
                const groupMsg = ESCALATION_MESSAGE[g.key];
                return (
                  <NotificationDrawerGroup
                    key={g.key}
                    isExpanded={!collapsed.has(g.key)}
                    onExpand={() => {
                      toggleGroup(g.key);
                    }}
                    count={g.items.length}
                    title={groupMsg ? intl.formatMessage(groupMsg) : g.key}
                  >
                    <NotificationDrawerList>
                      {g.items.map((card) => (
                        <NotificationDrawerListItem
                          key={card.number}
                          variant={escVariant(g.key)}
                          onClick={() => {
                            onSelect(card);
                          }}
                        >
                          <NotificationDrawerListItemHeader
                            variant={escVariant(g.key)}
                            title={intl.formatMessage(messages.sdlcIssueLabel, {
                              number: card.number,
                              title: card.title,
                            })}
                            icon={escalationIcon(g.key, escalationTone(g.key))}
                          />
                          <NotificationDrawerListItemBody>
                            <Inline>
                              <ClockIcon aria-hidden />
                              {intl.formatMessage(messages.sdlcTimeInPhase, {
                                duration: formatAge(
                                  intl,
                                  card.timeInPhaseSeconds,
                                ),
                              })}
                            </Inline>
                          </NotificationDrawerListItemBody>
                        </NotificationDrawerListItem>
                      ))}
                    </NotificationDrawerList>
                  </NotificationDrawerGroup>
                );
              })}
            </NotificationDrawerGroupList>
          )}
        </NotificationDrawerBody>
      </NotificationDrawer>
    </DrawerPanelContent>
  );
}

/** The tracked-issue list of a bundle (or the Unreleased row): each execution record
 *  links to its shipped-issue side panel (SDLC-VIS-10/11). In a bundle, the pull
 *  requests that delivered the issue are grouped (indented) beneath it, so the row
 *  summary's PR count reconciles with what is on screen (SDLC-VIS-03). In the
 *  Unreleased row an issue carries no bundle PRs, so nothing is nested. */
function MatrixIssueList({
  issues,
  version,
  mergedLine,
  nowMs,
}: {
  issues: readonly AgentSdlcShippedIssue[];
  /** The bundle version context for the shipped selection the link addresses. */
  version: string | null;
  mergedLine: (issue: AgentSdlcShippedIssue) => string | null;
  nowMs?: number;
}): React.ReactElement {
  return (
    <Stack>
      {issues.map((issue) => {
        const ago = mergedLine(issue);
        const issueLabel = `#${String(issue.number)} ${issue.title}`;
        return (
          <StackItem key={issue.number} className={styles.shipEnter}>
            <NavLinkButton
              action={{
                type: "sdlcShipped",
                shipped: { issue: issue.number, version },
              }}
            >
              {issueLabel}
            </NavLinkButton>
            {ago ? <Content component="small"> {ago}</Content> : null}
            {issue.prs.length > 0 ? (
              <div
                style={{
                  paddingInlineStart: "var(--pf-t--global--spacer--md, 16px)",
                }}
              >
                <MatrixPRList prs={issue.prs} nowMs={nowMs} />
              </div>
            ) : null}
          </StackItem>
        );
      })}
    </Stack>
  );
}

/** The one-off pull-request list of a bundle: PRs with no tracked execution record,
 *  each a direct link to GitHub (SDLC-VIS-11). */
function MatrixPRList({
  prs,
  nowMs,
}: {
  prs: readonly AgentSdlcPullRequest[];
  nowMs?: number;
}): React.ReactElement {
  const intl = useIntl();
  return (
    <Stack>
      {prs.map((pr) => {
        const ago = formatMergedAgo(intl, pr.mergedAt, nowMs);
        const prLabel = `#${String(pr.number)} ${pr.title}`;
        const line =
          ago && pr.author
            ? intl.formatMessage(messages.sdlcMergedBy, {
                ago,
                user: pr.author,
              })
            : ago;
        return (
          <StackItem key={pr.number}>
            <Button
              component="a"
              variant="link"
              isInline
              href={pr.url}
              target="_blank"
              rel="noreferrer"
              icon={<ExternalLinkAltIcon />}
              iconPosition="end"
            >
              {prLabel}
            </Button>
            {line ? <Content component="small"> {line}</Content> : null}
          </StackItem>
        );
      })}
    </Stack>
  );
}

/** A bundle row's expanded contents (SDLC-VIS-11): the Unreleased row lists only its
 *  in-flight tracked issues; a released bundle splits its PR-driven contents into a
 *  "Tracked issues" group and an "Other pull requests" group. */
function MatrixBundleContents({
  row,
  mergedLine,
  nowMs,
}: {
  row: AgentSdlcBundleRow;
  mergedLine: (issue: AgentSdlcShippedIssue) => string | null;
  nowMs?: number;
}): React.ReactElement {
  if (row.issues.length === 0 && row.otherPRs.length === 0) {
    return (
      <Content component="small">
        <FormattedMessage
          {...(row.unreleased
            ? messages.sdlcBundleNoIssues
            : messages.sdlcBundleNoContents)}
        />
      </Content>
    );
  }
  // The Unreleased row holds only in-flight tracked work, so it needs no group headers.
  if (row.unreleased) {
    return (
      <MatrixIssueList
        issues={row.issues}
        version={null}
        mergedLine={mergedLine}
      />
    );
  }
  return (
    <Stack hasGutter>
      {row.issues.length > 0 ? (
        <StackItem>
          <Title headingLevel="h4" size="md">
            <FormattedMessage {...messages.sdlcTrackedIssues} />
          </Title>
          <MatrixIssueList
            issues={row.issues}
            version={row.version}
            mergedLine={mergedLine}
            nowMs={nowMs}
          />
        </StackItem>
      ) : null}
      {row.otherPRs.length > 0 ? (
        <StackItem>
          <Title headingLevel="h4" size="md">
            <FormattedMessage {...messages.sdlcOtherPRs} />
          </Title>
          <MatrixPRList prs={row.otherPRs} nowMs={nowMs} />
        </StackItem>
      ) : null}
    </Stack>
  );
}

function ShippedMatrix({
  data,
  selectedBundleSeed,
  nowMs,
  expandedSeed,
  onToggleRow,
}: AgentSdlcViewProps & {
  /** Expanded row addressed by bundle seed (FLT-NAV-13). */
  expandedSeed: string | null;
  onToggleRow: (seed: string) => void;
}): React.ReactElement {
  const intl = useIntl();
  const mergedLine = (issue: AgentSdlcShippedIssue): string | null =>
    formatMergedLine(intl, issue, nowMs);
  const { hubClusters, rows } = data.matrix;
  const columnCount = 2 + hubClusters.length + 1;

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
              // Address the expanded row by a stable key, not its index, so a
              // background refresh reorder keeps it expanded. The Unreleased row has
              // no seed, so it uses a reserved synthetic token and stays expandable.
              const expandKey = bundleRowExpandKey(row);
              const isExpanded = expandedSeed === expandKey;
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
                          onToggleRow(expandKey);
                        },
                      }}
                    />
                    <Td>
                      {row.unreleased ? (
                        <FormattedMessage {...messages.sdlcUnreleased} />
                      ) : seed !== null ? (
                        <NavLinkButton
                          action={{ type: "openBundleOnMap", seed }}
                          icon={<BundleIdenticon seed={seed} />}
                        >
                          <Inline>
                            {identiName(seed)}
                            <ArrowRightIcon aria-hidden />
                          </Inline>
                        </NavLinkButton>
                      ) : (
                        <Inline>
                          <BundleIdenticon seed={row.version ?? "?"} />
                          {row.version ?? ""}
                        </Inline>
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
                      {row.unreleased ? (
                        <Label isCompact>{row.issues.length}</Label>
                      ) : (
                        <Label isCompact>
                          {intl.formatMessage(messages.sdlcBundleSummary, {
                            issues: intl.formatMessage(
                              messages.sdlcBundleIssueCount,
                              { issues: row.issues.length },
                            ),
                            prs: intl.formatMessage(
                              messages.sdlcBundlePrCount,
                              { prs: bundlePRCount(row) },
                            ),
                          })}
                        </Label>
                      )}
                    </Td>
                  </Tr>
                  <Tr isExpanded={isExpanded}>
                    <Td />
                    <Td colSpan={columnCount - 1}>
                      <ExpandableRowContent>
                        <MatrixBundleContents
                          row={row}
                          mergedLine={mergedLine}
                          nowMs={nowMs}
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

/** The details side panel (SDLC-VIS-10): opens on card click, shows the card's
 *  current state and its full label history (state over time). */
function CardDetailPanel({
  card,
  onClose,
  onBack,
}: {
  card: AgentSdlcCard;
  onClose: () => void;
  /** When opened from the escalations drawer, returns to that list instead of the board. */
  onBack?: () => void;
}): React.ReactElement {
  const intl = useIntl();
  const phaseMessage = PHASE_MESSAGE[card.phase];
  const panelTitle = `#${String(card.number)} ${card.title}`;
  const esc = card.escalation;
  const escMessage = esc ? ESCALATION_MESSAGE[esc] : undefined;
  const rawVariant = esc ? escVariant(esc) : "info";
  const alertVariant = rawVariant === "custom" ? "info" : rawVariant;
  const showRework = card.rework !== null && esc !== ESCALATION_BLOCKED;
  const spacerMd = "var(--pf-t--global--spacer--md, 16px)";
  return (
    <DrawerPanelContent isResizable defaultSize="33%" minSize="360px">
      <DrawerHead>
        {/* The whole issue identifier is the prominent GitHub link (SDLC-VIS-10). */}
        <Title headingLevel="h3" size="md">
          <Button
            component="a"
            variant="link"
            isInline
            href={card.url}
            target="_blank"
            rel="noreferrer"
            icon={<ExternalLinkAltIcon />}
            iconPosition="end"
          >
            {panelTitle}
          </Button>
        </Title>
        <DrawerActions>
          <DrawerCloseButton onClick={onClose} />
        </DrawerActions>
      </DrawerHead>
      <DrawerPanelBody>
        {onBack ? (
          <Button
            variant="link"
            isInline
            icon={<AngleLeftIcon />}
            onClick={onBack}
            style={{ marginBottom: spacerMd }}
          >
            <FormattedMessage {...messages.sdlcBackToEscalations} />
          </Button>
        ) : null}
        {esc && escMessage ? (
          <Alert
            variant={alertVariant}
            isInline
            title={intl.formatMessage(escMessage)}
            style={{ marginBottom: spacerMd }}
          />
        ) : null}
        <DescriptionList isHorizontal isCompact>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <FormattedMessage {...messages.sdlcDetailPhase} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              <Label isCompact icon={phaseIconEl(card.phase)}>
                {phaseMessage ? (
                  <FormattedMessage {...phaseMessage} />
                ) : (
                  card.phase
                )}
              </Label>
            </DescriptionListDescription>
          </DescriptionListGroup>
          {card.scope ? (
            <DescriptionListGroup>
              <DescriptionListTerm>
                <FormattedMessage {...messages.sdlcFilterScope} />
              </DescriptionListTerm>
              <DescriptionListDescription>
                {scopeShort(card.scope)}
              </DescriptionListDescription>
            </DescriptionListGroup>
          ) : null}
          <DescriptionListGroup>
            <DescriptionListTerm>
              <FormattedMessage {...messages.sdlcAxisTitle} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              {formatAge(intl, card.timeInPhaseSeconds)}
            </DescriptionListDescription>
          </DescriptionListGroup>
          {showRework ? (
            <DescriptionListGroup>
              <DescriptionListTerm>
                <FormattedMessage {...messages.sdlcDetailRework} />
              </DescriptionListTerm>
              <DescriptionListDescription>
                <ReworkMeter rework={card.rework} />
              </DescriptionListDescription>
            </DescriptionListGroup>
          ) : null}
        </DescriptionList>
        <Divider style={{ margin: `${spacerMd} 0` }} />
        <Title
          headingLevel="h4"
          size="md"
          style={{ marginBottom: "var(--pf-t--global--spacer--sm, 8px)" }}
        >
          <FormattedMessage {...messages.sdlcHistoryTitle} />
        </Title>
        <LabelHistory card={card} />
      </DrawerPanelBody>
    </DrawerPanelContent>
  );
}

/** A shipped issue selected from the matrix. */
interface ShippedSelection {
  readonly issue: AgentSdlcShippedIssue;
  readonly version: string | null;
}

/**
 * Resolve a board/escalation card from its issue number (FLT-NAV-13): a stable
 * identity that survives a background board reorder. Returns null when the addressed
 * issue is no longer present (FLT-NAV-14).
 */
function resolveCard(
  data: AgentSdlcData,
  number: number | null,
): AgentSdlcCard | null {
  if (number === null) {
    return null;
  }
  return [...data.cards, ...data.lane].find((c) => c.number === number) ?? null;
}

/** Resolve a shipped selection from its issue number and version context. */
function resolveShipped(
  data: AgentSdlcData,
  number: number | null,
  version: string | null,
): ShippedSelection | null {
  if (number === null) {
    return null;
  }
  let fallback: ShippedSelection | null = null;
  for (const row of data.matrix.rows) {
    const issue = row.issues.find((i) => i.number === number);
    if (issue) {
      if (version === null || row.version === version) {
        return { issue, version: row.version };
      }
      fallback ??= { issue, version: row.version };
    }
  }
  return fallback;
}

/** Compact details panel for a shipped issue (lighter than a board card: no live
 *  phase history), so clicking a matrix issue opens the same side panel (SDLC-VIS-10). */
function ShippedDetailPanel({
  selection,
  nowMs,
  onClose,
}: {
  selection: ShippedSelection;
  nowMs?: number;
  onClose: () => void;
}): React.ReactElement {
  const intl = useIntl();
  const { issue, version } = selection;
  const panelTitle = `#${String(issue.number)} ${issue.title}`;
  const mergedAgo = formatMergedLine(intl, issue, nowMs);
  return (
    <DrawerPanelContent isResizable defaultSize="33%" minSize="360px">
      <DrawerHead>
        <Title headingLevel="h3" size="md">
          <Button
            component="a"
            variant="link"
            isInline
            href={issue.url}
            target="_blank"
            rel="noreferrer"
            icon={<ExternalLinkAltIcon />}
            iconPosition="end"
          >
            {panelTitle}
          </Button>
        </Title>
        <DrawerActions>
          <DrawerCloseButton onClick={onClose} />
        </DrawerActions>
      </DrawerHead>
      <DrawerPanelBody>
        <DescriptionList isHorizontal isCompact>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <FormattedMessage {...messages.sdlcDetailPhase} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              <Label isCompact icon={phaseIconEl("shipped")}>
                <FormattedMessage {...messages.sdlcPhaseShipped} />
              </Label>
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <FormattedMessage {...messages.sdlcDetailBundle} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              {version ?? <FormattedMessage {...messages.sdlcUnreleased} />}
            </DescriptionListDescription>
          </DescriptionListGroup>
          {mergedAgo ? (
            <DescriptionListGroup>
              <DescriptionListTerm>
                <FormattedMessage {...messages.sdlcDetailMerged} />
              </DescriptionListTerm>
              <DescriptionListDescription>
                {mergedAgo}
              </DescriptionListDescription>
            </DescriptionListGroup>
          ) : null}
        </DescriptionList>
      </DrawerPanelBody>
    </DrawerPanelContent>
  );
}

/** A full-card phase-transition flight ready to render: the cloned card node plus the
 *  measured source rect, the arc translate samples, and the landing burst point - all in
 *  viewport coordinates so board/column scroll is handled for free (SDLC-VIS-20/SDLC-VIS-21). */
interface ActiveCardFlight {
  /** Stable React key so a re-render never restarts an in-flight animation. */
  readonly key: string;
  readonly number: number;
  /** A board card that flew into the Shipped matrix (no destination column to hold). */
  readonly toShipped: boolean;
  /** Source top-left (viewport px) - where the flying clone starts. */
  readonly left: number;
  readonly top: number;
  readonly width: number;
  readonly height: number;
  /** Arc translate keyframe samples (deltas from source to destination). */
  readonly travelVars: Record<string, string>;
  /** Landing burst centre (viewport px). */
  readonly burstX: number;
  readonly burstY: number;
  /** A 1:1 detached clone of the real card element, positioned to fill the flight box. */
  readonly clone: HTMLElement;
}

/** One captured board card: its live rect and a detached clone for a flying copy. */
interface CapturedCard {
  readonly rect: DOMRect;
  readonly clone: HTMLElement;
}

/** Snapshot every rendered board card (rect + a 1:1 clone) so a later transition can
 *  measure the source slot and fly the exact card, even once it has left the DOM. */
function captureCards(board: HTMLElement | null): Map<number, CapturedCard> {
  const out = new Map<number, CapturedCard>();
  if (!board) {
    return out;
  }
  board.querySelectorAll<HTMLElement>("[data-card-number]").forEach((el) => {
    const n = Number(el.dataset.cardNumber);
    if (!Number.isFinite(n)) {
      return;
    }
    const clone = el.cloneNode(true) as HTMLElement;
    // Neutralise the card's own absolute positioning + aging transition so the clone
    // fills the flight box and never animates its `top` while airborne.
    clone.style.position = "absolute";
    clone.style.inset = "0";
    clone.style.left = "0";
    clone.style.top = "0";
    clone.style.right = "0";
    clone.style.width = "100%";
    clone.style.height = "100%";
    clone.style.margin = "0";
    clone.style.transition = "none";
    clone.removeAttribute("data-card-number");
    clone.setAttribute("aria-hidden", "true");
    out.set(n, { rect: el.getBoundingClientRect(), clone });
  });
  return out;
}

export interface BoardFlightsState {
  readonly flights: readonly ActiveCardFlight[];
  /** Numbers whose flight is still airborne to a destination COLUMN, so the board holds
   *  (skips) them until the flying copy lands - the analogue of the map's holdSeedById.
   *  Shipped flights are not held here (the card has already left the board). */
  readonly heldNumbers: ReadonlySet<number>;
}

let cardFlightSeq = 0;

/** Fallback flight-box size when no real card rect is measurable yet (jsdom, or a card
 *  that was already held when its next transition arrived). */
const FALLBACK_CARD_W = 200;
const FALLBACK_CARD_H = 84;

/**
 * Turn the pure phase diff into renderable flights by measuring geometry: the source slot
 * (the previous commit's rect), the destination slot (the new column slot, or the matrix
 * landing for a ship hop), and the shared arc between them.
 *
 * Rects degrade gracefully through per-card then per-column anchors (SDLC-VIS-21): the
 * freshest card rect wins, then the last capture before the card was held, then the
 * from/to column anchor. This keeps a flight correct even when the card is NOT in the DOM
 * at transition time - a ship hop (card already in the matrix) or a chained transition
 * that lands while an earlier flight still holds the card out of its column. A brand-new
 * intake card has no source slot, so it flies down into its slot from the top of the board.
 */
function buildCardFlights(
  found: readonly CardFlight[],
  prevCards: Map<number, CapturedCard>,
  current: Map<number, CapturedCard>,
  lastKnown: Map<number, CapturedCard>,
  columnRects: Map<string, DOMRect>,
  boardRect: DOMRect | null,
  matrixRect: DOMRect | null,
): ActiveCardFlight[] {
  const active: ActiveCardFlight[] = [];
  for (const cf of found) {
    // A 1:1 card node to fly: freshest DOM copy, else the last capture before it was held.
    // Clone it again so the flight owns its node (the base copy may also live in lastKnown
    // and in an earlier still-airborne flight).
    const base = (current.get(cf.number) ?? lastKnown.get(cf.number))?.clone;
    if (!base) {
      continue;
    }
    const clone = base.cloneNode(true) as HTMLElement;

    // Card size: any real card rect we have seen; else a sensible default.
    const cardRect =
      prevCards.get(cf.number)?.rect ??
      current.get(cf.number)?.rect ??
      lastKnown.get(cf.number)?.rect ??
      null;
    const width = cardRect?.width ?? FALLBACK_CARD_W;
    const height = cardRect?.height ?? FALLBACK_CARD_H;

    // Source top-left: the slot the card just left, else its last-known slot, else the
    // from-phase column anchor, else it flies down from the top of the board (intake).
    const srcRect =
      prevCards.get(cf.number)?.rect ??
      lastKnown.get(cf.number)?.rect ??
      (cf.fromPhase ? (columnRects.get(cf.fromPhase) ?? null) : null);
    let srcLeft: number;
    let srcTop: number;
    if (srcRect) {
      srcLeft = srcRect.left;
      srcTop = srcRect.top;
    } else if (boardRect) {
      srcLeft = boardRect.left;
      srcTop = boardRect.top;
    } else {
      continue;
    }

    // Destination top-left: the matrix landing for a ship hop, else the new column slot,
    // else the to-phase column anchor (the card is held out of the DOM).
    let dLeft: number;
    let dTop: number;
    if (cf.toShipped) {
      if (!matrixRect) {
        continue;
      }
      dLeft = matrixRect.left + 20;
      dTop = matrixRect.top + 10;
    } else {
      const destRect =
        current.get(cf.number)?.rect ?? columnRects.get(cf.toPhase);
      if (!destRect) {
        continue;
      }
      dLeft = destRect.left;
      dTop = destRect.top;
    }

    const deltaX = dLeft - srcLeft;
    const deltaY = dTop - srcTop;
    const { cx, cy } = arcControl(0, 0, deltaX, deltaY);
    active.push({
      key: `card-flight-${String(cardFlightSeq++)}`,
      number: cf.number,
      toShipped: cf.toShipped,
      left: srcLeft,
      top: srcTop,
      width,
      height,
      travelVars: arcTravelVars(0, 0, deltaX, deltaY, cx, cy),
      burstX: dLeft + width / 2,
      burstY: dTop + height / 2,
      clone,
    });
  }
  return active;
}

/** Snapshot each board column's track rect, keyed by phase, so a flight can fall back to a
 *  column anchor when the card itself is not in the DOM (SDLC-VIS-21 position anchors). */
function captureColumns(board: HTMLElement | null): Map<string, DOMRect> {
  const out = new Map<string, DOMRect>();
  if (!board) {
    return out;
  }
  board.querySelectorAll<HTMLElement>("[data-phase-col]").forEach((el) => {
    const key = el.dataset.phaseCol;
    if (key) {
      out.set(key, el.getBoundingClientRect());
    }
  });
  return out;
}

/**
 * Drives the board's full-card phase-transition flights (SDLC-VIS-20/SDLC-VIS-21). On each
 * server snapshot it diffs phases (computeCardFlights - pure, keyed on server phase, so
 * the one-second continuous-aging re-render never triggers it), measures the source slot
 * (captured on the previous commit) and the destination slot, and registers a flight that
 * the BoardFlights overlay animates along the shared arc with the shared dust burst.
 *
 * Mirrors the map's useFlights registry discipline: stable keys, self-expiring timers held
 * in a ref, and NO effect-cleanup cancellation, so an unrelated poll or the aging
 * re-render landing mid-flight never restarts or strands a flight. Honours
 * prefers-reduced-motion by launching nothing (cards snap).
 */
function useCardFlights(
  data: AgentSdlcData,
  boardRef: React.RefObject<HTMLDivElement | null>,
  matrixRef: React.RefObject<HTMLDivElement | null>,
): BoardFlightsState {
  const prevDataRef = useRef<AgentSdlcData | null>(null);
  // The previous commit's captured cards, so a transition can read the source slot after
  // React has already moved/removed the card in the new commit.
  const lastRef = useRef<Map<number, CapturedCard>>(new Map());
  // The most recent capture of EVERY card ever seen (not reset when a card is held), so a
  // transition that lands while the card is held out of the DOM still has a source rect and
  // a clone to fly (SDLC-VIS-21).
  const lastKnownRef = useRef<Map<number, CapturedCard>>(new Map());
  const [flights, setFlights] = useState<readonly ActiveCardFlight[]>([]);
  // Cull timers, held in a ref so they survive re-renders (browser timers are numbers).
  const timers = useRef<Set<number>>(new Set());

  // Clear timers ONLY on unmount - never per render - so an aging re-render or unrelated
  // poll cannot clearTimeout a pending cull and strand a frozen flying clone (the same
  // trap the map's useFlights documents).
  useEffect(() => {
    const pending = timers.current;
    return () => {
      pending.forEach((t) => {
        window.clearTimeout(t);
      });
      pending.clear();
    };
  }, []);

  // Keyed on the server snapshot [data] ONLY, so the one-second continuous-aging
  // re-render (which changes the aged copy, never `data`) can neither trigger nor restart
  // a flight (SDLC-VIS-21). Over a poll interval a card's slot drifts well under a pixel
  // (aging maps onto an hours-long scale), so the previous commit's source rect is an
  // accurate launch point. useLayoutEffect (not useEffect) commits the hold before paint,
  // so a landing card never flashes in its new column for a frame before the flight hides
  // it.
  useLayoutEffect(() => {
    // Refresh the source snapshot and prior-data baseline FIRST (like the map's
    // useFlights), so every early-return path still advances them for the next transition.
    const prevData = prevDataRef.current;
    const prevCards = lastRef.current;
    const current = captureCards(boardRef.current);
    const columnRects = captureColumns(boardRef.current);
    lastRef.current = current;
    prevDataRef.current = data;
    // Merge (never delete) so a card held out of the DOM keeps its last-known capture.
    const lastKnown = lastKnownRef.current;
    for (const [n, cap] of current) {
      lastKnown.set(n, cap);
    }

    if (
      !prevData ||
      prefersReducedMotion() ||
      typeof document === "undefined"
    ) {
      return;
    }
    const found = computeCardFlights(prevData, data);
    if (found.length === 0) {
      return;
    }
    const active = buildCardFlights(
      found,
      prevCards,
      current,
      lastKnown,
      columnRects,
      boardRef.current?.getBoundingClientRect() ?? null,
      matrixRef.current?.getBoundingClientRect() ?? null,
    );
    if (active.length === 0) {
      return;
    }

    const keys = new Set(active.map((a) => a.key));
    setFlights((cur) => [...cur, ...active]);
    // Each flight self-expires via a tracked timer; this effect returns NO cleanup so a
    // later commit cannot cancel an in-flight cull (mirrors the map's useFlights).
    const track = timers.current;
    const id = window.setTimeout(() => {
      track.delete(id);
      setFlights((cur) => cur.filter((a) => !keys.has(a.key)));
    }, FLIGHT_MS + DUST_MS);
    track.add(id);
  }, [data, boardRef, matrixRef]);

  // A column-bound flight holds its destination card until the copy lands (no double
  // render). Ship hops are excluded: the card has already left the board.
  const heldNumbers = useMemo(() => {
    const held = new Set<number>();
    for (const fl of flights) {
      if (!fl.toShipped) {
        held.add(fl.number);
      }
    }
    return held;
  }, [flights]);

  return { flights, heldNumbers };
}

/** Attaches a detached clone node into the flight box exactly once. */
function attachClone(clone: HTMLElement): (el: HTMLDivElement | null) => void {
  return (el) => {
    if (!el || el.firstChild) {
      return;
    }
    el.appendChild(clone);
  };
}

/** The board flight overlay (SDLC-VIS-20): for each active flight, a 1:1 clone of the
 *  card flies along the shared arc from its old slot to its new one, landing with the
 *  shared impact + dust burst. Rendered into document.body (like the old ship ghost) so a
 *  transformed Drawer ancestor cannot clip or offset the fixed-position copies. */
function BoardFlights({
  flights,
}: {
  flights: readonly ActiveCardFlight[];
}): React.ReactElement | null {
  if (typeof document === "undefined" || flights.length === 0) {
    return null;
  }
  return createPortal(
    <div aria-hidden="true">
      {flights.map((fl) => (
        <div key={fl.key}>
          <div
            className={styles.flightClone}
            data-card-flight={fl.number}
            style={{
              left: `${String(fl.left)}px`,
              top: `${String(fl.top)}px`,
              width: `${String(fl.width)}px`,
              height: `${String(fl.height)}px`,
            }}
          >
            <div className={motion.flightTravel} style={fl.travelVars}>
              <div
                className={motion.flightScaleCard}
                style={{ height: "100%" }}
              >
                <div
                  ref={attachClone(fl.clone)}
                  style={{
                    position: "relative",
                    width: "100%",
                    height: "100%",
                  }}
                />
              </div>
            </div>
          </div>
          <div
            className={styles.flightBurst}
            style={{
              left: `${String(fl.burstX)}px`,
              top: `${String(fl.burstY)}px`,
            }}
          >
            <span
              className={[styles.flightRing, motion.impactRing].join(" ")}
            />
            {DUST.map((p, i) => (
              <span
                key={i}
                className={[styles.flightDust, motion.dust].join(" ")}
                style={
                  {
                    "--dust-dx": p.dx,
                    "--dust-dy": p.dy,
                  } as React.CSSProperties
                }
              />
            ))}
          </div>
        </div>
      ))}
    </div>,
    document.body,
  );
}

/** The shared vertical time-in-phase axis to the left of the board: one scale that
 *  reads across every column (SDLC-VIS-08). "now" at the top, stale toward the floor. */
function AgingYAxis({ scale }: { scale: number }): React.ReactElement {
  const intl = useIntl();
  const ratios = [0, 0.25, 0.5, 0.75, 1];
  return (
    <div className={styles.axis} aria-hidden>
      <div className={styles.axisHeader}>
        <FormattedMessage {...messages.sdlcAxisTitle} />
      </div>
      <div className={styles.axisTicks}>
        {ratios.map((r) => (
          <span
            key={r}
            className={styles.axisTick}
            style={{ top: `${String(r * 90)}%` }}
          >
            {r === 0
              ? intl.formatMessage(messages.sdlcAxisNow)
              : formatAge(intl, r * scale)}
          </span>
        ))}
      </div>
    </div>
  );
}

/** The agent/scope/* label without its prefix, for a compact filter chip. */
function scopeShort(scope: string): string {
  return scope.startsWith("agent/scope/")
    ? `@${scope.slice("agent/scope/".length)}`
    : scope;
}

/** Scope filter: a typeahead, checkbox multi-select with removable chips. Scales to
 *  many scopes where a toggle group would not (NN/g: dropdown + search past ~7-10;
 *  PatternFly faceted-filter pattern). Empty selection = all scopes. */
function ScopeFilter({
  scopes,
  selected,
  onChange,
}: {
  scopes: readonly string[];
  selected: readonly string[];
  onChange: (next: string[]) => void;
}): React.ReactElement {
  const intl = useIntl();
  const [isOpen, setOpen] = useState(false);
  const [input, setInput] = useState("");
  const filtered = scopes.filter((s) =>
    scopeShort(s).toLowerCase().includes(input.toLowerCase()),
  );
  const toggleVal = (v: string) => {
    onChange(
      selected.includes(v) ? selected.filter((x) => x !== v) : [...selected, v],
    );
  };
  return (
    <Select
      isOpen={isOpen}
      selected={[...selected]}
      onSelect={(_e, value) => {
        if (typeof value === "string") {
          toggleVal(value);
        }
      }}
      onOpenChange={(open) => {
        setOpen(open);
      }}
      toggle={(toggleRef) => (
        <MenuToggle
          ref={toggleRef}
          variant="typeahead"
          aria-label={intl.formatMessage(messages.sdlcFilterScope)}
          onClick={() => {
            setOpen(!isOpen);
          }}
          isExpanded={isOpen}
          // Fixed width so adding chips never reflows the neighbouring search box;
          // the chip group collapses to "+N more" past a couple (see numLabels).
          style={{ width: "260px" }}
        >
          <TextInputGroup isPlain>
            <TextInputGroupMain
              value={input}
              onClick={() => {
                setOpen(true);
              }}
              onChange={(_e, v) => {
                setInput(v);
                setOpen(true);
              }}
              placeholder={
                selected.length > 0
                  ? intl.formatMessage(messages.sdlcScopeSelected, {
                      count: selected.length,
                    })
                  : intl.formatMessage(messages.sdlcScopePlaceholder)
              }
              autoComplete="off"
              role="combobox"
              isExpanded={isOpen}
            />
          </TextInputGroup>
        </MenuToggle>
      )}
    >
      <SelectList>
        {filtered.length === 0 ? (
          <SelectOption isDisabled>
            <FormattedMessage {...messages.sdlcScopeNoResults} />
          </SelectOption>
        ) : (
          filtered.map((s) => (
            <SelectOption
              key={s}
              value={s}
              hasCheckbox
              isSelected={selected.includes(s)}
            >
              {scopeShort(s)}
            </SelectOption>
          ))
        )}
      </SelectList>
    </Select>
  );
}

/**
 * The agent detail panel's 24h history tiles - queue depth, items processed, and
 * items failed - reusing the map node panel's shared MetricTile (SDLC-AGT-09): the
 * same square-aspect, 180px-capped tile with an area sparkline. No shared cursor
 * here (the panel has no companion widget to synchronise with), so each tile shows
 * its live scalar with the full 24h band underneath.
 */
function AgentMetricTiles({
  agent,
}: {
  readonly agent: SdlcAgentMetrics;
}): React.ReactElement {
  return (
    <div className={tileStyles.metricTiles}>
      <MetricTile
        label={messages.agentsColQueue}
        value={agent.queueDepth}
        history={agent.series.queueDepth}
        color={USER_COLOR}
        active={null}
      />
      <MetricTile
        label={messages.agentsColDone}
        value={agent.itemsProcessed}
        history={agent.series.itemsProcessed}
        color={TEXT_COLOR}
        active={null}
      />
      <MetricTile
        label={messages.agentsColFailed}
        value={agent.itemsFailed}
        history={agent.series.itemsFailed}
        color={TONE_COLOR.danger}
        active={null}
      />
    </div>
  );
}

function AgentDetailPanel({
  agent,
  onClose,
}: {
  agent: SdlcAgentMetrics;
  onClose: () => void;
}): React.ReactElement {
  return (
    <DrawerPanelContent isResizable defaultSize="33%" minSize="360px">
      <DrawerHead>
        <span style={{ fontWeight: 700, fontSize: 18 }}>
          <Inline>
            <RobotIcon aria-hidden />
            <span>{agent.name}</span>
          </Inline>
        </span>
        <DrawerActions>
          <DrawerCloseButton onClick={onClose} />
        </DrawerActions>
      </DrawerHead>
      <DrawerPanelBody>
        <DescriptionList isHorizontal isCompact>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <FormattedMessage {...messages.columnEnvironment} />
            </DescriptionListTerm>
            <DescriptionListDescription>{agent.env}</DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <FormattedMessage {...messages.agentsColReplicas} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              <span
                style={{
                  color:
                    agent.replicasOn > 0 ? TONE_COLOR.success : METRIC_SUBTLE,
                  fontWeight: 700,
                }}
              >
                {agent.replicasOn}
              </span>
              <span style={{ color: METRIC_SUBTLE }}>
                <FormattedMessage {...messages.replicasRatioSeparator} />
                {agent.replicasTotal}
              </span>
            </DescriptionListDescription>
          </DescriptionListGroup>
        </DescriptionList>
        {/* Queue, done, and failed have 24h history, so they render as sparkline
            tiles (SDLC-AGT-09) rather than duplicating their current value in a
            second, history-less DescriptionList row. */}
        <div className="pf-v6-u-mt-md">
          <AgentMetricTiles agent={agent} />
        </div>
      </DrawerPanelBody>
    </DrawerPanelContent>
  );
}

// useLiveNow returns a wall-clock timestamp that ticks every second so cards age
// continuously between the plane's refetches, not just in a jump when new data lands.
// An explicit override (tests, the dev harness) freezes it for deterministic output
// and never starts a timer.
function useLiveNow(override?: number): number {
  const [now, setNow] = useState<number>(() => override ?? Date.now());
  useEffect(() => {
    if (override !== undefined) {
      return;
    }
    const id = setInterval(() => {
      setNow(Date.now());
    }, 1_000);
    return () => {
      clearInterval(id);
    };
  }, [override]);
  return override ?? now;
}

export function AgentSdlcView(props: AgentSdlcViewProps): React.ReactElement {
  const { data } = props;
  const intl = useIntl();
  const boardRef = useRef<HTMLDivElement>(null);
  const matrixRef = useRef<HTMLDivElement>(null);
  // Navigation, selection, and filter state come from the URL (FLT-NAV-05,
  // FLT-NAV-13); selections are addressed by issue number and resolved back to the
  // live record so they survive a background board reorder (FLT-NAV-14).
  const route = useRoute();
  const { navigate } = useRouter();
  const showNotifications = route.sdlc.escalations;
  const selectedScopes = route.sdlc.scopes;
  const query = route.sdlc.query;
  const selected = resolveCard(data, route.sdlc.card);
  const selectedShipped = resolveShipped(
    data,
    route.sdlc.shipped,
    route.sdlc.shippedVersion,
  );
  useHorizontalWheel(boardRef);
  // Full-card phase-transition flights: diff the server snapshot, fly the whole card
  // between columns (and into the matrix on ship), and hold the destination until it lands
  // (SDLC-VIS-20/SDLC-VIS-21). Suppressed under prefers-reduced-motion (cards snap).
  const { flights: cardFlights, heldNumbers } = useCardFlights(
    data,
    boardRef,
    matrixRef,
  );

  // Agent runtime telemetry keyed by agent name for O(1) column lookups.
  const agentByName = useMemo(() => {
    if (!props.agents) {
      return new Map<string, SdlcAgentMetrics>();
    }
    return props.agents;
  }, [props.agents]);

  // Local ephemeral state for the agent detail panel (SDLC-AGT-08). A route-driven
  // panel (card detail, shipped detail, notifications) supersedes it. Reset during
  // render (the React-recommended "adjust state when a prop changes" pattern,
  // precedented in NodeDetails's cursor reset) rather than in an effect, so there
  // is no extra render pass and no synchronous setState-in-effect.
  const [selectedAgentName, setSelectedAgentName] = useState<string | null>(
    null,
  );
  const routePanelActive =
    route.sdlc.card !== null ||
    route.sdlc.shipped !== null ||
    showNotifications;
  const [wasRoutePanelActive, setWasRoutePanelActive] =
    useState(routePanelActive);
  if (routePanelActive !== wasRoutePanelActive) {
    setWasRoutePanelActive(routePanelActive);
    if (routePanelActive) {
      setSelectedAgentName(null);
    }
  }
  const selectedAgent =
    selectedAgentName !== null
      ? (agentByName.get(selectedAgentName) ?? null)
      : null;

  // Escape closes the side panel (card detail or escalations). An open select menu
  // handles its own Escape first (guarded on an open listbox), so it isn't swallowed.
  const panelOpen =
    selected !== null ||
    selectedShipped !== null ||
    showNotifications ||
    selectedAgent !== null;
  useEffect(() => {
    if (!panelOpen) {
      return;
    }
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "Escape") {
        return;
      }
      if (
        typeof document !== "undefined" &&
        document.querySelector('[role="listbox"]')
      ) {
        return;
      }
      if (selectedAgent !== null) {
        setSelectedAgentName(null);
        return;
      }
      navigate({ type: "sdlcClosePanel" });
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [panelOpen, navigate, selectedAgent]);

  const phases = boardPhases(data.phases);
  // The vertical time axis is derived from the snapshot ages so it stays stable
  // between refetches; cards then slide continuously within it as the clock advances.
  const scale = globalAgingScaleSeconds(phases, data.cards);

  // Continuous aging: advance each record's time-in-phase by the seconds elapsed since
  // this snapshot arrived, so cards slide smoothly and minutes tick up live. The
  // baseline recomputes whenever new data lands (resetting the delta to zero), and an
  // explicit nowMs override (tests/harness) pins the delta to zero for determinism.
  const liveNowMs = useLiveNow(props.nowMs);
  // Baseline = the clock value when this snapshot arrived, taken from the ticking
  // state (not an impure Date.now in render). Reset it in render when new data lands
  // (the sanctioned "adjust state on prop change" pattern), so the delta returns to
  // zero and cards re-anchor to the fresh snapshot.
  const [baseline, setBaseline] = useState<{
    data: AgentSdlcData;
    ms: number;
  }>(() => ({ data, ms: liveNowMs }));
  if (baseline.data !== data) {
    setBaseline({ data, ms: liveNowMs });
  }
  const deltaSeconds = Math.max(0, (liveNowMs - baseline.ms) / 1000);
  const agedCards = useMemo(
    () =>
      deltaSeconds <= 0
        ? data.cards
        : data.cards.map((c) => ({
            ...c,
            timeInPhaseSeconds: c.timeInPhaseSeconds + deltaSeconds,
          })),
    [data.cards, deltaSeconds],
  );
  const agedLane = useMemo(
    () =>
      deltaSeconds <= 0
        ? data.lane
        : data.lane.map((c) => ({
            ...c,
            timeInPhaseSeconds: c.timeInPhaseSeconds + deltaSeconds,
          })),
    [data.lane, deltaSeconds],
  );

  // Scope options are data, derived from the records in scope (no compiled-in list).
  const scopes = [
    ...new Set(
      [...data.cards, ...data.lane]
        .map((c) => c.scope)
        .filter((s): s is string => s !== null),
    ),
  ].sort((a, b) => a.localeCompare(b));

  const matches = (c: AgentSdlcCard): boolean => {
    if (
      selectedScopes.length > 0 &&
      (c.scope === null || !selectedScopes.includes(c.scope))
    ) {
      return false;
    }
    if (query.trim() !== "") {
      const hay = `#${String(c.number)} ${c.title}`.toLowerCase();
      if (!hay.includes(query.trim().toLowerCase())) {
        return false;
      }
    }
    return true;
  };
  // Hold a card out of its destination column while its flight is airborne, so the card
  // is never shown twice (once resting, once flying); it reappears at its current aged slot
  // when the flight lands (SDLC-VIS-21).
  const cards = agedCards.filter(
    (c) => matches(c) && !heldNumbers.has(c.number),
  );

  const livePanel: React.ReactNode =
    selectedAgent !== null ? (
      <AgentDetailPanel
        agent={selectedAgent}
        onClose={() => {
          setSelectedAgentName(null);
        }}
      />
    ) : selected !== null ? (
      <CardDetailPanel
        card={selected}
        onClose={() => {
          navigate({ type: "sdlcClosePanel" });
        }}
        onBack={
          showNotifications
            ? () => {
                // Return to the still-open escalation list (FLT-NAV-13).
                navigate({ type: "sdlcCard", card: null });
              }
            : undefined
        }
      />
    ) : selectedShipped !== null ? (
      <ShippedDetailPanel
        selection={selectedShipped}
        nowMs={props.nowMs}
        onClose={() => {
          navigate({ type: "sdlcShipped", shipped: null });
        }}
      />
    ) : showNotifications ? (
      <NotificationsPanel
        lane={agedLane}
        onClose={() => {
          navigate({ type: "sdlcClosePanel" });
        }}
        onSelect={(card) => {
          // Keep the list open so browser Back and the detail's Back-to-list both
          // return to it (FLT-NAV-13).
          navigate({ type: "sdlcOpenEscalationCard", card: card.number });
        }}
      />
    ) : null;
  // Always keep a panel mounted (live content, or an empty placeholder) so the Drawer
  // runs its width transition on open/close rather than popping in. PatternFly animates
  // the width of the always-mounted panel; the placeholder collapses to zero width.
  const panelContent: React.ReactNode = livePanel ?? <DrawerPanelContent />;

  return (
    <Drawer isExpanded={panelOpen} className={styles.drawer}>
      <DrawerContent panelContent={panelContent}>
        <DrawerContentBody>
          <div className={styles.view}>
            <Toolbar
              className={styles.toolbar}
              inset={{ default: "insetNone" }}
              collapseListedFiltersBreakpoint="md"
              clearAllFilters={() => {
                navigate({ type: "sdlcScopes", scopes: [] });
                navigate({ type: "sdlcQuery", value: "" });
              }}
            >
              <ToolbarContent>
                <ToolbarToggleGroup toggleIcon={<FilterIcon />} breakpoint="md">
                  <ToolbarGroup variant="filter-group">
                    {scopes.length > 0 ? (
                      <ToolbarFilter
                        labels={selectedScopes.map((s) => ({
                          key: s,
                          node: scopeShort(s),
                        }))}
                        deleteLabel={(_category, label) => {
                          const key =
                            typeof label === "string" ? label : label.key;
                          navigate({
                            type: "sdlcScopes",
                            scopes: selectedScopes.filter((x) => x !== key),
                          });
                        }}
                        deleteLabelGroup={() => {
                          navigate({ type: "sdlcScopes", scopes: [] });
                        }}
                        categoryName={intl.formatMessage(
                          messages.sdlcFilterScope,
                        )}
                      >
                        <ScopeFilter
                          scopes={scopes}
                          selected={selectedScopes}
                          onChange={(scopes) => {
                            navigate({ type: "sdlcScopes", scopes });
                          }}
                        />
                      </ToolbarFilter>
                    ) : null}
                    <ToolbarItem>
                      <SearchInput
                        placeholder={intl.formatMessage(
                          messages.sdlcSearchPlaceholder,
                        )}
                        value={query}
                        onChange={(_event, value) => {
                          navigate({ type: "sdlcQuery", value });
                        }}
                        onClear={() => {
                          navigate({ type: "sdlcQuery", value: "" });
                        }}
                      />
                    </ToolbarItem>
                  </ToolbarGroup>
                </ToolbarToggleGroup>
                <ToolbarItem align={{ default: "alignEnd" }}>
                  <NotificationBadge
                    variant={escalationBadgeVariant(data.lane)}
                    count={data.lane.length}
                    isExpanded={showNotifications}
                    aria-label={intl.formatMessage(
                      messages.sdlcNotificationsTitle,
                    )}
                    onClick={() => {
                      navigate({ type: "sdlcToggleEscalations" });
                    }}
                  />
                </ToolbarItem>
              </ToolbarContent>
            </Toolbar>
            <div className={styles.boardRow}>
              <AgingYAxis scale={scale} />
              <div className={styles.board} ref={boardRef}>
                {phases.map((phase) => (
                  <AgingColumn
                    key={phase.key}
                    phase={phase}
                    cards={cardsInPhase(cards, phase.key)}
                    scale={scale}
                    onSelect={(card) => {
                      navigate({ type: "sdlcCard", card: card.number });
                    }}
                    agentSummary={agentByName.get(PHASE_AGENT[phase.key] ?? "")}
                    onSelectAgent={(name) => {
                      navigate({ type: "sdlcClosePanel" });
                      setSelectedAgentName(name);
                    }}
                  />
                ))}
              </div>
            </div>
            <BoardFlights flights={cardFlights} />
            <div className={styles.bottom}>
              <div ref={matrixRef}>
                <ShippedMatrix
                  {...props}
                  expandedSeed={route.sdlc.shippedRow}
                  onToggleRow={(seed) => {
                    navigate({ type: "sdlcShippedRow", seed });
                  }}
                />
              </div>
            </div>
          </div>
        </DrawerContentBody>
      </DrawerContent>
    </Drawer>
  );
}
