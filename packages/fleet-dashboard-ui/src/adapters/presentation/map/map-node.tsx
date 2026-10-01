// A single instance rendered as an SVG card: provider chip, gateway donut,
// digest-seeded identicon (ringed by promotion state), name + alias + release +
// digest, a "sand" gateway-history sparkline, and a role badge. All text is server
// DATA (names/versions/digests/labels), so this card needs no i18n catalog; the
// accessible, translated equivalent is the sibling promotion/instances tables.

import { identiName } from "../../../domain/map/identiname";
import type { MapNode } from "../../../domain/map/model";
import type { NodeBox } from "../../../domain/map/layout";
import {
  CARD_BG,
  CARD_STROKE,
  promotionRing,
  TEXT_COLOR,
  TEXT_SUBTLE,
  TONE_COLOR,
} from "./colors";
import { GatewayDonut } from "./gateway-donut";
import { Identicon } from "./identicon";
import { SandSparkline } from "./sand-sparkline";

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

      {/* provider chip (top-left) */}
      {node.provider ? (
        <text
          x={x + 12}
          y={y + 20}
          fontSize={10}
          fontWeight={700}
          fill={TEXT_SUBTLE}
        >
          {node.provider.toUpperCase()}
        </text>
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

      {/* role badge (bottom-right) */}
      {node.role ? (
        <text
          x={x + w - 10}
          y={y + h - 8}
          textAnchor="end"
          fontSize={10}
          fontWeight={700}
          fill={TEXT_SUBTLE}
        >
          {node.role}
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
    </g>
  );
}
