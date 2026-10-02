// A scaled overview of the whole scene with a viewport rectangle reflecting the
// current pan/zoom. Click or drag anywhere on it to recenter the main view on that
// point (prototype miniNav behaviour). Driven by the layout + current viewport.

import { useCallback, useRef } from "react";

import type { MapLayout } from "../../../domain/map/layout";
import { CARD_BG, CARD_STROKE, TEXT_SUBTLE, TONE_COLOR } from "./colors";
import { f, viewBox as toViewBox } from "./svg";
import type { Viewport } from "./use-map-viewport";

// A fixed, roughly-square minimap BOX. The (wide) scene is scaled to CONTAIN within
// it and centred, so the overview reads as a compact panel rather than a thin,
// over-compressed sliver the width of the whole scene.
const BOX_W = 184;
const BOX_H = 132;

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
  // Contain the scene in the box, centred: `scale` world->box px, (offX, offY) the
  // letterbox padding that keeps the aspect ratio honest.
  const scale = Math.min(
    BOX_W / Math.max(1, layout.width),
    BOX_H / Math.max(1, layout.height),
  );
  const contentW = layout.width * scale;
  const contentH = layout.height * scale;
  const offX = (BOX_W - contentW) / 2;
  const offY = (BOX_H - contentH) / 2;

  const svgRef = useRef<SVGSVGElement | null>(null);
  const draggingRef = useRef(false);

  const recenterFromEvent = useCallback(
    (clientX: number, clientY: number) => {
      const svg = svgRef.current;
      if (!svg) {
        return;
      }
      const r = svg.getBoundingClientRect();
      // Box px -> content px (subtract padding) -> world units.
      const px = ((clientX - r.left) / Math.max(1, r.width)) * BOX_W;
      const py = ((clientY - r.top) / Math.max(1, r.height)) * BOX_H;
      const wx = (px - offX) / Math.max(0.0001, scale);
      const wy = (py - offY) / Math.max(0.0001, scale);
      onRecenter(wx, wy);
    },
    [offX, offY, scale, onRecenter],
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
      width={BOX_W}
      height={BOX_H}
      viewBox={toViewBox(0, 0, BOX_W, BOX_H)}
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
      <g transform={`translate(${f(offX)}, ${f(offY)}) scale(${f(scale)})`}>
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
      </g>
    </svg>
  );
}
