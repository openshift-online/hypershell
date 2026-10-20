// A single instance rendered as an SVG card: a provider chip paired with a
// hub/spoke role indicator (top-left), a digest-seeded identicon (ringed by
// promotion state, top-right), name + alias + release, a "sand" gateway-history
// sparkline, and the live gateway count (bottom-right). All text is server DATA
// (names/versions/labels), so this card needs no i18n catalog; the accessible,
// translated equivalent is the sibling promotion/instances tables.

import { totalGateways } from "../../../domain/fleet";
import { identiName } from "../../../domain/map/identiname";
import type { MapNode } from "../../../domain/map/model";
import type { NodeBox } from "../../../domain/map/layout";
import { healthBadge, isHealthUnavailable } from "../../../domain/status";
import {
  CARD_BG,
  CARD_STROKE,
  promotionRing,
  providerChip,
  roleBadgeFill,
  TEXT_COLOR,
  TEXT_SUBTLE,
  TONE_COLOR,
} from "./colors";
import { SANDBOX_COLOR } from "./colors";
import { Identicon } from "./identicon";
import { SandSparkline } from "./sand-sparkline";
import { SandboxSparkline } from "./sandbox-sparkline";
import styles from "./topology-map.module.css";

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
// Non-color carrier (brand-color.spec UI-BRAND-03: color is never the sole signal) for
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
  const chip = node.provider ? providerChip(node.provider) : null;
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
      {/* cluster label above the card (data; empty -> omitted) */}
      {node.cluster ? (
        <text
          x={x + w / 2}
          y={y - 6}
          textAnchor="middle"
          fontSize={11}
          fontWeight={700}
          fill={TEXT_SUBTLE}
        >
          {node.cluster}
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

      {/* provider chip - colour DERIVED from the provider label (brand hue for
          known public clouds, hash hue otherwise). Sits on the badge row BELOW the
          env name + alias. */}
      {chip && node.provider ? (
        <g>
          <rect
            x={x + 10}
            y={y + 56}
            width={42}
            height={16}
            rx={4}
            fill={chip.bg}
          />
          <text
            x={x + 31}
            y={y + 68}
            textAnchor="middle"
            fontSize={10}
            fontWeight={700}
            fill={chip.fg}
          >
            {node.provider.toUpperCase()}
          </text>
        </g>
      ) : null}

      {/* hub/spoke role indicator, right of the provider chip on the badge row -
          fill DERIVED from the role label (no fleet role list baked in) */}
      {node.role ? (
        <g>
          <rect
            x={x + 56}
            y={y + 56}
            width={node.role.length * 6.5 + 12}
            height={16}
            rx={4}
            fill={roleBadgeFill(node.role)}
          />
          <text
            x={x + 56 + (node.role.length * 6.5 + 12) / 2}
            y={y + 68}
            textAnchor="middle"
            fontSize={9}
            fontWeight={700}
            fill="#10202f"
          >
            {node.role.toUpperCase()}
          </text>
        </g>
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
        x={x + 50}
        y={y + 22}
        fontSize={13}
        fontWeight={700}
        fill={TEXT_COLOR}
      >
        {node.id}
      </text>
      <text x={x + 50} y={y + 36} fontSize={10} fill={TEXT_SUBTLE}>
        {identiName(shownSeed)}
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
