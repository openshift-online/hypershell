// A single instance rendered as an SVG card: provider chip, gateway donut,
// digest-seeded identicon (ringed by promotion state), name + alias + release +
// digest, a "sand" gateway-history sparkline, and a role badge. All text is server
// DATA (names/versions/digests/labels), so this card needs no i18n catalog; the
// accessible, translated equivalent is the sibling promotion/instances tables.

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
import { GatewayDonut } from "./gateway-donut";
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
}

const ICON = 30;

export function MapNodeCard({
  node,
  box,
  selected,
  highlighted,
  onSelect,
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

      {/* provider chip (top-left) - colour DERIVED from the provider label */}
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

      {/* gateway donut (top-centre) */}
      <g fill={TEXT_COLOR}>
        <GatewayDonut
          counts={node.gateways}
          cx={x + w / 2}
          cy={y + 22}
          radius={15}
        />
      </g>

      {/* identicon (top-right) with promotion-state ring */}
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
      <Identicon seed={node.seed} x={x + w - ICON - 8} y={y + 8} size={ICON} />

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
      {node.digest ? (
        <text x={x + 12} y={y + 94} fontSize={9} fill={TEXT_SUBTLE}>
          {node.digest}
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

      {/* role badge circle (bottom-right) - fill DERIVED from the role label */}
      {node.role ? (
        <g>
          <circle
            cx={x + w - 20}
            cy={y + h - 20}
            r={13}
            fill={roleBadgeFill(node.role)}
            stroke="rgba(0,0,0,0.25)"
          />
          <text
            x={x + w - 20}
            y={y + h - 20}
            textAnchor="middle"
            dominantBaseline="central"
            fontSize={12}
            fontWeight={700}
            fill="#10202f"
          >
            {node.role.charAt(0).toUpperCase()}
          </text>
        </g>
      ) : null}
    </g>
  );
}
