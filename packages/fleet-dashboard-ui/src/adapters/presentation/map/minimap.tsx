// A scaled overview of the whole scene with a viewport rectangle reflecting the
// current pan/zoom. Click or drag anywhere on it to recenter the main view on that
// point (prototype miniNav behaviour). Driven by the layout + current viewport.

import { useCallback, useRef } from "react";

import type { MapLayout } from "../../../domain/map/layout";
import { CARD_BG, CARD_STROKE, TEXT_SUBTLE, TONE_COLOR } from "./colors";
import { viewBox as toViewBox } from "./svg";
import type { Viewport } from "./use-map-viewport";

const MINI_W = 176;

export interface MiniMapProps {
  readonly layout: MapLayout;
  readonly viewport: Viewport;
  /** Recenter the main view on a world point (minimap click/drag navigation). */
  readonly onRecenter: (wx: number, wy: number) => void;
}

export function MiniMap({
  layout,
  viewport,
  onRecenter,
}: MiniMapProps): React.ReactElement {
  const scale = MINI_W / Math.max(1, layout.width);
  const miniH = layout.height * scale;

  const svgRef = useRef<SVGSVGElement | null>(null);
  const draggingRef = useRef(false);

  const recenterFromEvent = useCallback(
    (clientX: number, clientY: number) => {
      const svg = svgRef.current;
      if (!svg) {
        return;
      }
      const r = svg.getBoundingClientRect();
      const wx = ((clientX - r.left) / Math.max(1, r.width)) * layout.width;
      const wy = ((clientY - r.top) / Math.max(1, r.height)) * layout.height;
      onRecenter(wx, wy);
    },
    [layout.width, layout.height, onRecenter],
  );

  const onPointerDown = useCallback(
    (e: React.PointerEvent) => {
      draggingRef.current = true;
      try {
        e.currentTarget.setPointerCapture(e.pointerId);
      } catch {
        // capture unsupported; drag still tracks via move events.
      }
      recenterFromEvent(e.clientX, e.clientY);
    },
    [recenterFromEvent],
  );

  const onPointerMove = useCallback(
    (e: React.PointerEvent) => {
      if (!draggingRef.current) {
        return;
      }
      recenterFromEvent(e.clientX, e.clientY);
    },
    [recenterFromEvent],
  );

  const onPointerUp = useCallback((e: React.PointerEvent) => {
    draggingRef.current = false;
    try {
      e.currentTarget.releasePointerCapture(e.pointerId);
    } catch {
      // capture may already be gone; ignore.
    }
  }, []);

  return (
    <svg
      ref={svgRef}
      width={MINI_W}
      height={miniH}
      viewBox={toViewBox(0, 0, layout.width, layout.height)}
      role="presentation"
      aria-hidden="true"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      style={{
        background: CARD_BG,
        border: `1px solid ${CARD_STROKE}`,
        borderRadius: 6,
        cursor: "pointer",
        touchAction: "none",
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
