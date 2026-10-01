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
import { healthBadge } from "../../../domain/status";
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
import { Identicon } from "./identicon";
import { SandSparkline } from "./sand-sparkline";
import styles from "./topology-map.module.css";

/** True when the node warrants the red "attention" ring: degraded Argo health or
 *  any failed gateway. Purely a function of server-reported state. */
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
}

const ICON = 30;

export function MapNodeCard({
  node,
  box,
  selected,
  highlighted,
  onSelect,
  onSelectBundle,
}: MapNodeCardProps): React.ReactElement {
  const { x, y, w, h } = box;
  const issues = hasIssues(node);
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

      {/* provider chip (top-left) - colour DERIVED from the provider label
          (brand hue for known public clouds, hash hue otherwise) */}
      {chip && node.provider ? (
        <g>
          <rect
            x={x + 10}
            y={y + 7}
            width={42}
            height={16}
            rx={4}
            fill={chip.bg}
          />
          <text
            x={x + 31}
            y={y + 19}
            textAnchor="middle"
            fontSize={10}
            fontWeight={700}
            fill={chip.fg}
          >
            {node.provider.toUpperCase()}
          </text>
        </g>
      ) : null}

      {/* hub/spoke role indicator, right of the provider chip - fill DERIVED from
          the role label (no fleet role list baked in) */}
      {node.role ? (
        <g>
          <rect
            x={x + 56}
            y={y + 7}
            width={node.role.length * 6.5 + 12}
            height={16}
            rx={4}
            fill={roleBadgeFill(node.role)}
          />
          <text
            x={x + 56 + (node.role.length * 6.5 + 12) / 2}
            y={y + 19}
            textAnchor="middle"
            fontSize={9}
            fontWeight={700}
            fill="#10202f"
          >
            {node.role.toUpperCase()}
          </text>
        </g>
      ) : null}

      {/* identicon (top-right) with promotion-state ring. Clickable: opens the
          release bundle it identifies (not the instance). */}
      <g
        role="button"
        tabIndex={0}
        aria-label={identiName(node.seed)}
        onClick={selectBundle}
        onKeyDown={onIconKeyDown}
        style={{ cursor: "pointer" }}
      >
        <rect
          x={x + w - ICON - 11}
          y={y + 5}
          width={ICON + 6}
          height={ICON + 6}
          rx={6}
          fill="none"
          stroke={promotionRing(node.state)}
          strokeWidth={3}
        />
        <Identicon
          seed={node.seed}
          x={x + w - ICON - 8}
          y={y + 8}
          size={ICON}
        />
      </g>

      {/* identity block (left) */}
      <text
        x={x + 12}
        y={y + 52}
        fontSize={13}
        fontWeight={700}
        fill={TEXT_COLOR}
      >
        {node.id}
      </text>
      <text x={x + 12} y={y + 66} fontSize={10} fill={TEXT_SUBTLE}>
        {identiName(node.seed)}
      </text>
      {node.version ? (
        <text x={x + 12} y={y + 82} fontSize={11} fill={TEXT_COLOR}>
          {node.version}
        </text>
      ) : null}

      {/* gateway-history sparkline (bottom strip) */}
      <SandSparkline
        history={node.gatewayHistory}
        x={x + 10}
        y={y + h - 20}
        width={w - 56}
        height={12}
      />

      {/* gateway count (bottom-right): total gateways on this instance. The sand
          sparkline above carries the per-phase breakdown; this is just the tally. */}
      <g>
        <circle
          cx={x + w - 20}
          cy={y + h - 20}
          r={13}
          fill={CARD_BG}
          stroke={CARD_STROKE}
        />
        <text
          x={x + w - 20}
          y={y + h - 20}
          textAnchor="middle"
          dominantBaseline="central"
          fontSize={12}
          fontWeight={700}
          fill={TEXT_COLOR}
        >
          {totalGateways(node.gateways)}
        </text>
      </g>
    </g>
  );
}
