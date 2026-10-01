// A scaled overview of the whole scene with a viewport rectangle reflecting the
// current pan/zoom. Display-only: it orients the user within a scene larger than the
// screen. Driven purely by the layout + current viewport (no imperative refs).

import type { MapLayout } from "../../../domain/map/layout";
import { CARD_BG, CARD_STROKE, TEXT_SUBTLE, TONE_COLOR } from "./colors";
import { viewBox as toViewBox } from "./svg";
import type { Viewport } from "./use-map-viewport";

const MINI_W = 176;

export interface MiniMapProps {
  readonly layout: MapLayout;
  readonly viewport: Viewport;
}

export function MiniMap({
  layout,
  viewport,
}: MiniMapProps): React.ReactElement {
  const scale = MINI_W / Math.max(1, layout.width);
  const miniH = layout.height * scale;

  return (
    <svg
      width={MINI_W}
      height={miniH}
      viewBox={toViewBox(0, 0, layout.width, layout.height)}
      role="presentation"
      aria-hidden="true"
      style={{
        background: CARD_BG,
        border: `1px solid ${CARD_STROKE}`,
        borderRadius: 6,
      }}
    >
      {layout.nodes.map((b) => (
        <rect
          key={b.id}
          x={b.x}
          y={b.y}
          width={b.w}
          height={b.h}
          rx={6}
          fill={TEXT_SUBTLE}
          fillOpacity={0.5}
        />
      ))}
      <rect
        x={viewport.x}
        y={viewport.y}
        width={viewport.w}
        height={viewport.h}
        fill={TONE_COLOR.info}
        fillOpacity={0.12}
        stroke={TONE_COLOR.info}
        strokeWidth={Math.max(2, 2 / scale)}
      />
    </svg>
  );
}
