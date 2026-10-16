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
import { useEffect, useRef, useState } from "react";
import type { IntlShape, MessageDescriptor } from "react-intl";
import { FormattedMessage, useIntl } from "react-intl";

import {
  agingRatio,
  boardPhases,
  bundleRowSeed,
  cardsInPhase,
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
  type AgentSdlcPhase,
  type AgentSdlcRework,
  type AgentSdlcShippedIssue,
} from "../../../domain/agent-sdlc";
import { messages } from "../../../messages";
import { TONE_COLOR } from "../map/colors";
import { Identicon } from "../map/identicon";
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

function AgingColumn({
  phase,
  cards,
  scale,
  onSelect,
}: {
  phase: AgentSdlcPhase;
  cards: readonly AgentSdlcCard[];
  scale: number;
  onSelect: (card: AgentSdlcCard) => void;
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
          <Label isCompact>{cards.length}</Label>
        </div>
        {agent ? (
          <div className={styles.columnAgent}>
            <Inline>
              <RobotIcon aria-hidden />
              <span>{agent}</span>
            </Inline>
          </div>
        ) : null}
      </div>
      <div className={styles.track} ref={trackRef}>
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
            {clusters.map((cl, idx) => (
              <ClusterChip
                key={`cluster-${String(idx)}-${String(cl.top)}`}
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
    <DrawerPanelContent widths={{ default: "width_33" }}>
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

function MatrixIssues({
  issues,
  mergedLine,
  onSelect,
}: {
  issues: readonly AgentSdlcShippedIssue[];
  mergedLine: (issue: AgentSdlcShippedIssue) => string | null;
  onSelect: (issue: AgentSdlcShippedIssue) => void;
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
        const ago = mergedLine(issue);
        const issueLabel = `#${String(issue.number)} ${issue.title}`;
        return (
          <StackItem key={issue.number} className={styles.shipEnter}>
            <Button
              variant="link"
              isInline
              onClick={() => {
                onSelect(issue);
              }}
            >
              {issueLabel}
            </Button>
            {ago ? <Content component="small"> {ago}</Content> : null}
          </StackItem>
        );
      })}
    </Stack>
  );
}

function ShippedMatrix({
  data,
  selectedBundleSeed,
  onSelectBundle,
  onJumpToMap,
  nowMs,
  onSelectShipped,
}: AgentSdlcViewProps & {
  onSelectShipped: (selection: ShippedSelection) => void;
}): React.ReactElement {
  const intl = useIntl();
  const mergedLine = (issue: AgentSdlcShippedIssue): string | null =>
    formatMergedLine(intl, issue, nowMs);
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
                          icon={<BundleIdenticon seed={seed} />}
                          onClick={() => {
                            openBundle(row);
                          }}
                        >
                          <Inline>
                            {row.version ?? seed}
                            <ArrowRightIcon aria-hidden />
                          </Inline>
                        </Button>
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
                      <Label isCompact>{row.issues.length}</Label>
                    </Td>
                  </Tr>
                  <Tr isExpanded={isExpanded}>
                    <Td />
                    <Td colSpan={columnCount - 1}>
                      <ExpandableRowContent>
                        <MatrixIssues
                          issues={row.issues}
                          mergedLine={mergedLine}
                          onSelect={(issue) => {
                            onSelectShipped({ issue, version: row.version });
                          }}
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
    <DrawerPanelContent widths={{ default: "width_33" }}>
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
    <DrawerPanelContent widths={{ default: "width_33" }}>
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

/**
 * Flies a ghost card from the board into the matrix when an issue ships (leaves the
 * board and appears in a matrix row). Best-effort and purely decorative: it reads
 * live element rects, respects prefers-reduced-motion, and no-ops if refs are unset.
 */
function useShipFlight(
  data: AgentSdlcData,
  boardRef: React.RefObject<HTMLDivElement | null>,
  matrixRef: React.RefObject<HTMLDivElement | null>,
): void {
  const prevBoard = useRef<Set<number> | null>(null);
  useEffect(() => {
    const board = new Set(data.cards.map((c) => c.number));
    const shippedNow = new Set(
      data.matrix.rows.flatMap((r) => r.issues.map((i) => i.number)),
    );
    const prev = prevBoard.current;
    prevBoard.current = board;
    if (prev === null) {
      return;
    }
    const reduce =
      typeof window !== "undefined" &&
      typeof window.matchMedia === "function" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    if (reduce || typeof document === "undefined") {
      return;
    }
    const boardEl = boardRef.current;
    const matrixEl = matrixRef.current;
    if (!boardEl || !matrixEl) {
      return;
    }
    const flew = [...prev].filter((n) => !board.has(n) && shippedNow.has(n));
    if (flew.length === 0) {
      return;
    }
    const from = boardEl.getBoundingClientRect();
    const to = matrixEl.getBoundingClientRect();
    const ghost = document.createElement("div");
    ghost.className = styles.flight ?? "";
    ghost.textContent = "→";
    ghost.style.left = `${String(from.right - 60)}px`;
    ghost.style.top = `${String(from.bottom - 40)}px`;
    document.body.appendChild(ghost);
    requestAnimationFrame(() => {
      const dx = to.left + 20 - (from.right - 60);
      const dy = to.top + 10 - (from.bottom - 40);
      ghost.style.transform = `translate(${String(dx)}px, ${String(dy)}px) scale(0.6)`;
      ghost.style.opacity = "0";
    });
    window.setTimeout(() => {
      ghost.remove();
    }, 700);
  }, [data, boardRef, matrixRef]);
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

export function AgentSdlcView(props: AgentSdlcViewProps): React.ReactElement {
  const { data } = props;
  const intl = useIntl();
  const boardRef = useRef<HTMLDivElement>(null);
  const matrixRef = useRef<HTMLDivElement>(null);
  const [selected, setSelected] = useState<AgentSdlcCard | null>(null);
  const [selectedShipped, setSelectedShipped] =
    useState<ShippedSelection | null>(null);
  const [showNotifications, setShowNotifications] = useState<boolean>(false);
  const [selectedScopes, setSelectedScopes] = useState<string[]>([]);
  const [query, setQuery] = useState<string>("");
  useHorizontalWheel(boardRef);
  useShipFlight(data, boardRef, matrixRef);

  // Escape closes the side panel (card detail or escalations). An open select menu
  // handles its own Escape first (guarded on an open listbox), so it isn't swallowed.
  const panelOpen =
    selected !== null || selectedShipped !== null || showNotifications;
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
      setSelected(null);
      setSelectedShipped(null);
      setShowNotifications(false);
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [panelOpen]);

  const phases = boardPhases(data.phases);
  const scale = globalAgingScaleSeconds(phases, data.cards);

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
  const cards = data.cards.filter(matches);

  const livePanel: React.ReactNode =
    selected !== null ? (
      <CardDetailPanel
        card={selected}
        onClose={() => {
          setSelected(null);
          setShowNotifications(false);
        }}
        onBack={
          showNotifications
            ? () => {
                setSelected(null);
              }
            : undefined
        }
      />
    ) : selectedShipped !== null ? (
      <ShippedDetailPanel
        selection={selectedShipped}
        nowMs={props.nowMs}
        onClose={() => {
          setSelectedShipped(null);
        }}
      />
    ) : showNotifications ? (
      <NotificationsPanel
        lane={data.lane}
        onClose={() => {
          setShowNotifications(false);
        }}
        onSelect={(card) => {
          // keep showNotifications set so the detail's Back returns to the list
          setSelected(card);
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
                setSelectedScopes([]);
                setQuery("");
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
                          setSelectedScopes((prev) =>
                            prev.filter((x) => x !== key),
                          );
                        }}
                        deleteLabelGroup={() => {
                          setSelectedScopes([]);
                        }}
                        categoryName={intl.formatMessage(
                          messages.sdlcFilterScope,
                        )}
                      >
                        <ScopeFilter
                          scopes={scopes}
                          selected={selectedScopes}
                          onChange={setSelectedScopes}
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
                          setQuery(value);
                        }}
                        onClear={() => {
                          setQuery("");
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
                      setSelected(null);
                      setShowNotifications((v) => !v);
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
                    onSelect={setSelected}
                  />
                ))}
              </div>
            </div>
            <div className={styles.bottom}>
              <div ref={matrixRef}>
                <ShippedMatrix
                  {...props}
                  onSelectShipped={(sel) => {
                    setSelected(null);
                    setShowNotifications(false);
                    setSelectedShipped(sel);
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
