// A single instance rendered as an SVG card: a provider chip paired with a
// hub/spoke role indicator (top-left), a digest-seeded identicon (ringed by
// promotion state, top-right), name + alias + release, a "sand" gateway-history
// sparkline, and the live gateway count (bottom-right). All text is server DATA
// (names/versions/labels), so this card needs no i18n catalog; the accessible,
// translated equivalent is the sibling promotion/instances tables.

import { totalGateways } from "../../../domain/fleet";
import { capabilityValue } from "../../../domain/map/capabilities";
import { identiName } from "../../../domain/map/identiname";
import type { MapNode } from "../../../domain/map/model";
import type { NodeBox } from "../../../domain/map/layout";
import { healthBadge, isHealthUnavailable } from "../../../domain/status";
import {
  capabilityChip,
  CARD_BG,
  CARD_STROKE,
  promotionRing,
  providerChip,
  roleBadgeFill,
  type SvgColor,
  TEXT_COLOR,
  TEXT_SUBTLE,
  TONE_COLOR,
} from "./colors";
import { SANDBOX_COLOR } from "./colors";
import { Identicon } from "./identicon";
import { measureText, truncateToWidth } from "./measure-text";
import { SandSparkline } from "./sand-sparkline";
import { SandboxSparkline } from "./sandbox-sparkline";
import styles from "./topology-map.module.css";

// One chip size system shared by EVERY card pill (provider, role, runtime) so
// they read as one family: only width (measured) and colour
// vary (FLT-CAP-04). Geometry is presentation-only; the pure layout never measures.
const GUTTER = 10; // left inset: chips + chin sparklines start at x + GUTTER
const CHIP_H = 16;
const CHIP_FONT = 9;
const CHIP_WEIGHT = 700;
const CHIP_PAD_X = 7; // horizontal padding each side
const CHIP_GAP = 6; // gap between chips in a row
const CHIP_RX = 4; // corner radius
const BADGE_ROW_Y = 54; // y-offset of the provider/role row from the card top
const CAP_ROW_Y = 74; // y-offset of the runtime row

/** Measured pill width for a chip label: its glyph width plus padding each side. */
function chipWidth(label: string): number {
  return Math.ceil(measureText(label, CHIP_FONT, CHIP_WEIGHT)) + CHIP_PAD_X * 2;
}

/** Left x of each chip in a row, given each chip's width and the row's start x.
 *  A pure helper so the running cursor is a local, never reassigned render state
 *  (the React-compiler lint forbids mutating a captured var inside a closure). */
function chipLefts(widths: readonly number[], startX: number): number[] {
  const lefts: number[] = [];
  let acc = startX;
  for (const w of widths) {
    lefts.push(acc);
    acc += w + CHIP_GAP;
  }
  return lefts;
}

interface ChipSpec {
  readonly key: string;
  readonly label: string;
  readonly bg: SvgColor;
  readonly fg: SvgColor;
}

/** One row of chips, each pill sized to its measured text so nothing overflows. */
function ChipRow({
  chips,
  x,
  y,
}: {
  chips: readonly ChipSpec[];
  x: number;
  y: number;
}): React.ReactElement {
  const widths = chips.map((c) => chipWidth(c.label));
  const lefts = chipLefts(widths, x);
  return (
    <>
      {chips.map((c, i) => {
        const w = widths[i] ?? 0;
        const left = lefts[i] ?? x;
        return (
          <g key={c.key}>
            <rect
              x={left}
              y={y}
              width={w}
              height={CHIP_H}
              rx={CHIP_RX}
              fill={c.bg}
            />
            <text
              x={left + w / 2}
              y={y + CHIP_H / 2}
              textAnchor="middle"
              dominantBaseline="central"
              fontSize={CHIP_FONT}
              fontWeight={CHIP_WEIGHT}
              fill={c.fg}
            >
              {c.label}
            </text>
          </g>
        );
      })}
    </>
  );
}

/** True when the node warrants the SOLID red "attention" ring: a confirmed problem -
 *  degraded Argo health or any failed gateway. Purely a function of server-reported
 *  state. Distinct from unavailable health (see isHealthUnavailable), which is "we
 *  can't tell" rather than "it's bad" and gets a DASHED red ring instead. */
function hasIssues(node: MapNode): boolean {
  return (
    healthBadge(node.argoHealth).tone === "danger" ||
    (node.gateways.failed ?? 0) > 0
  );
}

export interface MapNodeCardProps {
  readonly node: MapNode;
  readonly box: NodeBox;
  readonly selected: boolean;
  /** Runs the selected release bundle - gets a dashed accent ring. */
  readonly highlighted: boolean;
  readonly onSelect: (id: string) => void;
  /** Selects the node's active release bundle (clicking its identicon). */
  readonly onSelectBundle: (seed: string) => void;
  /** While a promotion is flying IN to this node, the seed to keep showing until the
   *  flying copy lands (the node's pre-promotion bundle). Absent = show the live seed. */
  readonly displaySeed?: string;
}

const ICON = 30;

// Glanceable, non-translated flags on the card itself (like the provider/role chips,
// which are raw DATA). Their accessible, translated equivalents are the "Version drift"
// and "Health" rows in the detail drawer.
const DRIFT_LABEL = "DRIFT";
// Non-color carrier (brand-color.spec UIS-BRND-03: color is never the sole signal) for
// unavailable Argo health, paired with the dashed red ring. Scoped to health, with the
// "?" marking uncertainty rather than a confirmed-bad state.
const HEALTH_UNAVAILABLE_LABEL = "HEALTH?";

export function MapNodeCard({
  node,
  box,
  selected,
  highlighted,
  onSelect,
  onSelectBundle,
  displaySeed,
}: MapNodeCardProps): React.ReactElement {
  const { x, y, w, h } = box;
  // The seed to RENDER: while a promotion is inbound, hold the pre-promotion bundle so
  // the card doesn't switch icons before the flying copy lands. Clicks still target the
  // live bundle (node.seed). Switching displaySeed -> node.seed remounts the identicon
  // (key below), replaying the landing shake exactly as the new bundle settles in.
  const shownSeed = displaySeed ?? node.seed;
  const issues = hasIssues(node);
  // Unavailable/unknown Argo health: a severe "we can't see this cluster" signal,
  // shown as a DASHED red flashing ring (vs the SOLID red issues ring). When a
  // confirmed issue is also present, the solid ring wins (more actionable); the
  // HEALTH? caption still flags the missing health either way.
  const healthUnknown = isHealthUnavailable(node.argoHealth);
  const total = totalGateways(node.gateways);
  // Signature of the latest gateway-history sample: changes (and so replays the
  // sand ease-in) only when a new sample actually lands, never on an unchanged poll.
  const hist = node.gatewayHistory;
  const last = hist.length > 0 ? hist[hist.length - 1] : null;
  const historySig = last
    ? [hist.length, last.running, last.provisioning, last.failed].join(":")
    : "none";
  // Same idea for the sandbox sparkline: replay the conveyor slide only when a new
  // sandbox sample actually lands.
  const sbxHist = node.sandboxHistory;
  const sbxSig =
    sbxHist.length > 0
      ? [sbxHist.length, sbxHist[sbxHist.length - 1]].join(":")
      : "none";
  // Row 1 (provider + role) and row 2 (runtime) are built as chip specs; an absent
  // capability simply drops its chip and the row collapses around what remains
  // (FLT-CAP-03). Colour derives from the values alone and provider and runtime
  // are upper-cased (FLT-CAP-05).
  const providerColors = node.provider ? providerChip(node.provider) : null;
  const caps = node.capabilities ?? [];
  const runtime = capabilityValue(caps, "runtime");

  const badgeChips: ChipSpec[] = [];
  if (providerColors && node.provider) {
    badgeChips.push({
      key: "provider",
      label: node.provider.toUpperCase(),
      bg: providerColors.bg,
      fg: providerColors.fg,
    });
  }
  if (node.role) {
    // The role chip is the one INVERTED pill: a light same-hue fill with dark text,
    // so hub/spoke reads as structural rather than another coloured capability.
    badgeChips.push({
      key: "role",
      label: node.role.toUpperCase(),
      bg: roleBadgeFill(node.role),
      fg: "#10202f",
    });
  }

  const capChips: ChipSpec[] = [];
  if (runtime) {
    const c = capabilityChip(runtime);
    capChips.push({
      key: "runtime",
      label: runtime.toUpperCase(),
      bg: c.bg,
      fg: c.fg,
    });
  }

  // Title/alias/cluster truncate with an ellipsis so a long name never overflows the
  // card edge (FLT-CAP-04). The identity block sits right of the identicon.
  const textX = x + 50;
  const textMaxW = w - 58;
  const titleText = truncateToWidth(node.id, 13, 700, textMaxW);
  const aliasText = truncateToWidth(identiName(shownSeed), 10, 400, textMaxW);
  const clusterText = node.cluster
    ? truncateToWidth(node.cluster, 11, 700, w - 12)
    : null;

  const select = () => {
    onSelect(node.id);
  };
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      select();
    }
  };
  // The identicon is a nested target: it opens the release bundle it stands for,
  // rather than the instance. stopPropagation keeps the click off the card button.
  const selectBundle = (e: React.SyntheticEvent) => {
    e.stopPropagation();
    onSelectBundle(node.seed);
  };
  const onIconKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      selectBundle(e);
    }
  };

  return (
    <g
      role="button"
      tabIndex={0}
      aria-label={node.id}
      aria-pressed={selected}
      onClick={select}
      onKeyDown={onKeyDown}
      style={{ cursor: "pointer" }}
    >
      {/* cluster label above the card (data; empty -> omitted), truncated so a long
          name never runs past the card edge */}
      {clusterText ? (
        <text
          x={x + w / 2}
          y={y - 6}
          textAnchor="middle"
          fontSize={11}
          fontWeight={700}
          fill={TEXT_SUBTLE}
        >
          {clusterText}
        </text>
      ) : null}

      {highlighted ? (
        <rect
          x={x - 4}
          y={y - 4}
          width={w + 8}
          height={h + 8}
          rx={12}
          fill="none"
          stroke={TONE_COLOR.info}
          strokeWidth={2}
          strokeDasharray="5 4"
        />
      ) : null}
      <rect
        x={x}
        y={y}
        width={w}
        height={h}
        rx={10}
        fill={CARD_BG}
        stroke={selected ? TONE_COLOR.info : CARD_STROKE}
        strokeWidth={selected ? 2.5 : 1}
      />

      {/* attention ring: degraded health or a failed gateway. Flashes red, but
          collapses to a static ring under prefers-reduced-motion (CSS). */}
      {issues ? (
        <rect
          className={styles.attention}
          x={x + 1}
          y={y + 1}
          width={w - 2}
          height={h - 2}
          rx={9}
          fill="none"
          stroke={TONE_COLOR.danger}
          strokeWidth={2}
        />
      ) : null}

      {/* unavailable-health ring: same red flash as a confirmed issue, but DASHED to
          read as "status uncertain / cluster may be unreachable" rather than "confirmed
          bad". Only when there's no solid-red issue, so the two never stack. Reuses the
          .attention flash (reduced-motion safe). */}
      {healthUnknown && !issues ? (
        <rect
          className={styles.attention}
          x={x + 1}
          y={y + 1}
          width={w - 2}
          height={h - 2}
          rx={9}
          fill="none"
          stroke={TONE_COLOR.danger}
          strokeWidth={2}
          strokeDasharray="5 4"
        />
      ) : null}

      {/* Chip row 1 (provider + role): colours DERIVED from the labels (brand hue
          for known public clouds, hash hue otherwise); the role pill is inverted.
          Each pill is sized to its measured text, so nothing spills (FLT-CAP-04). */}
      {badgeChips.length > 0 ? (
        <ChipRow chips={badgeChips} x={x + GUTTER} y={y + BADGE_ROW_Y} />
      ) : null}

      {/* Chip row 2 (runtime): the capability row, one notch dimmer so it reads
          as secondary. Without a runtime capability the row is omitted (FLT-CAP-03). */}
      {capChips.length > 0 ? (
        <ChipRow chips={capChips} x={x + GUTTER} y={y + CAP_ROW_Y} />
      ) : null}

      {/* version-drift flag: this instance runs a different active release bundle
          than its environment's hub, so the env is internally inconsistent. Drawn
          like the red attention ring but in amber (a softer warning), plus a small
          amber caption in the empty band below the name - never over the title. Red
          issues take priority over the ring (more severe); the caption still shows. */}
      {node.driftsFromColumn && !issues && !healthUnknown ? (
        <rect
          x={x + 1}
          y={y + 1}
          width={w - 2}
          height={h - 2}
          rx={9}
          fill="none"
          stroke={TONE_COLOR.warning}
          strokeWidth={2}
        />
      ) : null}
      {/* caption band (between the alias and the provider/role chips): the HEALTH?
          flag for unavailable health takes this slot when present (more severe than
          drift); otherwise the amber DRIFT flag. Both have their full, translated
          equivalents in the detail drawer, so nothing is lost by showing one here. */}
      {healthUnknown ? (
        <text
          x={x + 50}
          y={y + 50}
          fontSize={9}
          fontWeight={700}
          letterSpacing={0.5}
          fill={TONE_COLOR.danger}
        >
          {HEALTH_UNAVAILABLE_LABEL}
        </text>
      ) : node.driftsFromColumn ? (
        <text
          x={x + 50}
          y={y + 50}
          fontSize={9}
          fontWeight={700}
          letterSpacing={0.5}
          fill={TONE_COLOR.warning}
        >
          {DRIFT_LABEL}
        </text>
      ) : null}

      {/* identicon (top-left, left of the env name) with promotion-state ring.
          Clickable: opens the release bundle it identifies (not the instance). */}
      <g
        role="button"
        tabIndex={0}
        aria-label={identiName(shownSeed)}
        onClick={selectBundle}
        onKeyDown={onIconKeyDown}
        style={{ cursor: "pointer" }}
      >
        {/* key={shownSeed}: React remounts this group only when the SHOWN bundle
            changes - which, when a flight's hold releases, is exactly as the new bundle
            lands - replaying the shake as the landing jolt of a promotion. */}
        <g key={shownSeed} className={styles.identShake}>
          <rect
            x={x + 8}
            y={y + 8}
            width={ICON + 6}
            height={ICON + 6}
            rx={6}
            fill="none"
            stroke={promotionRing(node.state)}
            strokeWidth={3}
          />
          <Identicon seed={shownSeed} x={x + 11} y={y + 11} size={ICON} />
        </g>
      </g>

      {/* identity block: env name first, alias beneath it, both to the RIGHT of the
          identicon. The badge row (provider + role) sits below; the raw digest is
          omitted (see above). */}
      <text
        x={textX}
        y={y + 22}
        fontSize={13}
        fontWeight={700}
        fill={TEXT_COLOR}
      >
        {titleText}
      </text>
      <text x={textX} y={y + 36} fontSize={10} fill={TEXT_SUBTLE}>
        {aliasText}
      </text>
      {/* The raw release digest/sha is intentionally NOT shown on the card: the
          identicon + identiname above already identify the deployed bundle, and the
          full digest reads as clutter here. It remains available in the detail
          drawer and the release "freight" bar. */}

      {/* gateway-history sparkline (upper chin). key={historySig}: remounts the
          subtree only when a new history sample lands, which replays the conveyor's
          left-shift (the slide itself lives inside SandSparkline). */}
      <g key={historySig}>
        <SandSparkline
          history={node.gatewayHistory}
          x={x + 10}
          y={y + h - 38}
          width={w - 56}
          height={12}
        />
      </g>

      {/* gateway count (right of the upper chin): total gateways on this instance.
          The sand sparkline carries the per-phase breakdown; this is just the tally.
          key={total}: remounts + replays the pulse only when the tally changes. */}
      <g key={total} className={styles.countPulse}>
        <circle
          cx={x + w - 20}
          cy={y + h - 38}
          r={13}
          fill={CARD_BG}
          stroke={CARD_STROKE}
        />
        <text
          x={x + w - 20}
          y={y + h - 38}
          textAnchor="middle"
          dominantBaseline="central"
          fontSize={12}
          fontWeight={700}
          fill={TEXT_COLOR}
        >
          {total}
        </text>
      </g>

      {/* sandbox history (lower chin): total active-sandbox count over time, on the
          EXACT same x-axis and width as the gateway sand sparkline above (x + 10,
          w - 56), so the two chins line up and are directly comparable. Sandboxes are
          one population, so this is a single teal band rather than a stack. The live
          total sits on the right as a teal tally, mirroring the gateway count circle.
          key={sbxSig}: remounts to replay the conveyor slide only on a new sample. */}
      <g key={sbxSig}>
        <SandboxSparkline
          history={node.sandboxHistory}
          x={x + 10}
          y={y + h - 18}
          width={w - 56}
          height={11}
        />
      </g>
      <text
        x={x + w - 20}
        y={y + h - 12}
        textAnchor="middle"
        dominantBaseline="central"
        fontSize={11}
        fontWeight={700}
        fill={SANDBOX_COLOR}
      >
        {node.sandboxes}
      </text>
    </g>
  );
}
