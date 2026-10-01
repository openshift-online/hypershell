// Pan / zoom / momentum for the SVG map, ported from the prototype's viewBox
// controller but reworked to React's state model (and the react-compiler rules): the
// viewBox lives in state, a rAF loop eases the live view toward a target and decays
// release velocity into a fling, and all mutable bookkeeping (drag start, velocity,
// the raf handle, the current view mirror) lives in refs that are only ever touched
// from event handlers / the rAF callback - never during render. Reduced-motion
// users get instant snaps with no momentum.

import { useCallback, useEffect, useRef, useState } from "react";

import { viewBox as toViewBox } from "./svg";

export interface Viewport {
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
}

/** Zoom factors (w multipliers): <1 zooms in, >1 zooms out. */
const WHEEL_IN = 0.88;
const WHEEL_OUT = 1 / WHEEL_IN;
const BUTTON_IN = 0.8;
const BUTTON_OUT = 1.25;
/** Zoom clamp as a fraction of the content width. */
const MIN_W_FRAC = 0.15;
const MAX_W_FRAC = 5;
/** rAF easing toward the target viewBox, and fling decay + stop threshold. */
const EASE = 0.28;
const DECAY = 0.9;
const VEL_CUTOFF = 0.35;
const SETTLE = 0.5;

export interface MapViewport {
  readonly svgRef: React.RefObject<SVGSVGElement | null>;
  /** The current viewBox string, derived from state (drives the <svg>). */
  readonly viewBox: string;
  /** Current viewport (for the minimap rect + aria). */
  readonly viewport: Viewport;
  readonly onWheel: (e: React.WheelEvent) => void;
  readonly onPointerDown: (e: React.PointerEvent) => void;
  readonly onPointerMove: (e: React.PointerEvent) => void;
  readonly onPointerUp: (e: React.PointerEvent) => void;
  readonly zoomIn: () => void;
  readonly zoomOut: () => void;
  readonly fit: () => void;
  /** Recenter the view on a world point (minimap click/drag navigation). */
  readonly recenter: (wx: number, wy: number) => void;
  /** True once a drag has actually moved (so a click can be suppressed). */
  readonly didPan: () => boolean;
}

interface Vec {
  x: number;
  y: number;
}

/**
 * The smallest viewBox with the CONTAINER's pixel aspect ratio that still contains
 * the whole `cw x ch` content, centred. Matching the viewBox aspect to the box means
 * the SVG (preserveAspectRatio "meet") fills the box edge-to-edge with no letterbox,
 * and pointer<->world mapping stays linear across the whole surface. When the box is
 * taller than the content (wide scene, tall canvas), the content centres vertically
 * with symmetric world-space padding.
 */
function containFit(cw: number, ch: number, aspect: number): Viewport {
  const a =
    Number.isFinite(aspect) && aspect > 0 ? aspect : cw / Math.max(1, ch);
  let w = cw;
  let h = w / a;
  if (h < ch) {
    h = ch;
    w = h * a;
  }
  return { x: (cw - w) / 2, y: (ch - h) / 2, w, h };
}

/** True when two viewports are within a pixel of each other on every axis. */
function viewsClose(a: Viewport, b: Viewport): boolean {
  return (
    Math.abs(a.x - b.x) < 1 &&
    Math.abs(a.y - b.y) < 1 &&
    Math.abs(a.w - b.w) < 1 &&
    Math.abs(a.h - b.h) < 1
  );
}

function prefersReducedMotion(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

export function useMapViewport(
  contentWidth: number,
  contentHeight: number,
  containerAspect: number,
): MapViewport {
  const baseW = Math.max(1, contentWidth);
  const baseH = Math.max(1, contentHeight);

  const [view, setView] = useState<Viewport>(() =>
    containFit(baseW, baseH, containerAspect),
  );

  const viewRef = useRef<Viewport>(view);
  const targetRef = useRef<Viewport>(view);
  // Live container aspect (updated each render) + the last fit we snapped to, so a
  // resize only re-fits when the user has not manually panned/zoomed since.
  const aspectRef = useRef(containerAspect);
  const fittedRef = useRef<Viewport>(view);
  const velRef = useRef<Vec>({ x: 0, y: 0 });
  const rafRef = useRef(0);
  const draggingRef = useRef(false);
  const movedRef = useRef(false);
  const capturedRef = useRef(false);
  const lastRef = useRef<Vec>({ x: 0, y: 0 });
  const svgRef = useRef<SVGSVGElement | null>(null);
  const stepRef = useRef<(() => void) | undefined>(undefined);

  const commit = useCallback((next: Viewport) => {
    viewRef.current = next;
    setView(next);
  }, []);

  // Keep the animation step closure fresh each render (writing a ref in an effect
  // is allowed; it reads refs + schedules the next frame when invoked via rAF).
  useEffect(() => {
    stepRef.current = () => {
      const cur = viewRef.current;
      let t = targetRef.current;
      let vel = velRef.current;
      if (vel.x !== 0 || vel.y !== 0) {
        t = { ...t, x: t.x + vel.x, y: t.y + vel.y };
        vel = { x: vel.x * DECAY, y: vel.y * DECAY };
        if (Math.hypot(vel.x, vel.y) < VEL_CUTOFF) {
          vel = { x: 0, y: 0 };
        }
        targetRef.current = t;
        velRef.current = vel;
      }
      const ease = prefersReducedMotion() ? 1 : EASE;
      const next: Viewport = {
        x: cur.x + (t.x - cur.x) * ease,
        y: cur.y + (t.y - cur.y) * ease,
        w: cur.w + (t.w - cur.w) * ease,
        h: cur.h + (t.h - cur.h) * ease,
      };
      const settled =
        vel.x === 0 &&
        vel.y === 0 &&
        Math.abs(t.x - next.x) < SETTLE &&
        Math.abs(t.y - next.y) < SETTLE &&
        Math.abs(t.w - next.w) < SETTLE &&
        Math.abs(t.h - next.h) < SETTLE;
      if (settled) {
        commit({ ...t });
        rafRef.current = 0;
        return;
      }
      commit(next);
      rafRef.current = requestAnimationFrame(() => stepRef.current?.());
    };
  });

  useEffect(
    () => () => {
      if (rafRef.current) {
        cancelAnimationFrame(rafRef.current);
      }
    },
    [],
  );

  const ensureLoop = useCallback(() => {
    if (!rafRef.current) {
      rafRef.current = requestAnimationFrame(() => stepRef.current?.());
    }
  }, []);

  const clampW = useCallback(
    (w: number) =>
      Math.min(Math.max(w, baseW * MIN_W_FRAC), baseW * MAX_W_FRAC),
    [baseW],
  );

  const zoomAround = useCallback(
    (px: number, py: number, k: number) => {
      const t = targetRef.current;
      const newW = clampW(t.w * k);
      const ratio = newW / t.w;
      if (ratio === 1) {
        return;
      }
      targetRef.current = {
        x: px - (px - t.x) * ratio,
        y: py - (py - t.y) * ratio,
        w: newW,
        h: t.h * ratio,
      };
      ensureLoop();
    },
    [clampW, ensureLoop],
  );

  const worldFromPointer = useCallback(
    (clientX: number, clientY: number): Vec => {
      const svg = svgRef.current;
      const v = viewRef.current;
      if (!svg) {
        return { x: v.x + v.w / 2, y: v.y + v.h / 2 };
      }
      const r = svg.getBoundingClientRect();
      return {
        x: v.x + ((clientX - r.left) / r.width) * v.w,
        y: v.y + ((clientY - r.top) / r.height) * v.h,
      };
    },
    [],
  );

  const onWheel = useCallback(
    (e: React.WheelEvent) => {
      const p = worldFromPointer(e.clientX, e.clientY);
      zoomAround(p.x, p.y, e.deltaY < 0 ? WHEEL_IN : WHEEL_OUT);
    },
    [worldFromPointer, zoomAround],
  );

  const onPointerDown = useCallback((e: React.PointerEvent) => {
    // No eager setPointerCapture: capturing on press retargets the pointer to the
    // svg, so a plain click on a child node <g> never reaches its onClick. We
    // capture lazily on the first real move (onPointerMove) instead, so taps still
    // select nodes while drags still pan.
    draggingRef.current = true;
    movedRef.current = false;
    capturedRef.current = false;
    velRef.current = { x: 0, y: 0 };
    lastRef.current = { x: e.clientX, y: e.clientY };
  }, []);

  const onPointerMove = useCallback(
    (e: React.PointerEvent) => {
      if (!draggingRef.current) {
        return;
      }
      const svg = svgRef.current;
      if (!svg) {
        return;
      }
      const r = svg.getBoundingClientRect();
      const v = viewRef.current;
      const dxScreen = e.clientX - lastRef.current.x;
      const dyScreen = e.clientY - lastRef.current.y;
      if (Math.abs(dxScreen) + Math.abs(dyScreen) > 2) {
        movedRef.current = true;
        if (!capturedRef.current) {
          // First real move: now claim the pointer so the drag keeps tracking even
          // if it leaves the svg. (Deferred from onPointerDown to keep clicks live.)
          try {
            e.currentTarget.setPointerCapture(e.pointerId);
            capturedRef.current = true;
          } catch {
            // capture unsupported / element gone; drag still works via move events.
          }
        }
      }
      const dxWorld = (dxScreen / r.width) * v.w;
      const dyWorld = (dyScreen / r.height) * v.h;
      const next: Viewport = { ...v, x: v.x - dxWorld, y: v.y - dyWorld };
      // Drag is 1:1: move the live view and the target together, no easing.
      targetRef.current = { ...targetRef.current, x: next.x, y: next.y };
      velRef.current = { x: -dxWorld, y: -dyWorld };
      lastRef.current = { x: e.clientX, y: e.clientY };
      commit(next);
    },
    [commit],
  );

  const onPointerUp = useCallback(
    (e: React.PointerEvent) => {
      if (!draggingRef.current) {
        return;
      }
      draggingRef.current = false;
      if (capturedRef.current) {
        capturedRef.current = false;
        try {
          e.currentTarget.releasePointerCapture(e.pointerId);
        } catch {
          // capture may already be gone; ignore.
        }
      }
      if (prefersReducedMotion()) {
        velRef.current = { x: 0, y: 0 };
        return;
      }
      // Hand the last drag velocity to the fling loop.
      ensureLoop();
    },
    [ensureLoop],
  );

  const centerZoom = useCallback(
    (k: number) => {
      const t = targetRef.current;
      zoomAround(t.x + t.w / 2, t.y + t.h / 2, k);
    },
    [zoomAround],
  );

  const zoomIn = useCallback(() => {
    centerZoom(BUTTON_IN);
  }, [centerZoom]);
  const zoomOut = useCallback(() => {
    centerZoom(BUTTON_OUT);
  }, [centerZoom]);

  const fit = useCallback(() => {
    const next = containFit(baseW, baseH, aspectRef.current);
    targetRef.current = next;
    fittedRef.current = next;
    velRef.current = { x: 0, y: 0 };
    ensureLoop();
  }, [baseW, baseH, ensureLoop]);

  // Re-fit when the container's aspect changes (e.g. the drawer opens and narrows
  // the box, or the window resizes) - but only while the view is still the last fit,
  // so a user's own pan/zoom is never yanked away under them. Snaps without momentum.
  useEffect(() => {
    aspectRef.current = containerAspect;
    if (viewsClose(viewRef.current, fittedRef.current)) {
      const next = containFit(baseW, baseH, containerAspect);
      fittedRef.current = next;
      targetRef.current = next;
      velRef.current = { x: 0, y: 0 };
      commit(next);
    }
  }, [containerAspect, baseW, baseH, commit]);

  const recenter = useCallback(
    (wx: number, wy: number) => {
      const t = targetRef.current;
      targetRef.current = { ...t, x: wx - t.w / 2, y: wy - t.h / 2 };
      velRef.current = { x: 0, y: 0 };
      ensureLoop();
    },
    [ensureLoop],
  );

  const didPan = useCallback(() => movedRef.current, []);

  return {
    svgRef,
    viewBox: toViewBox(view.x, view.y, view.w, view.h),
    viewport: view,
    onWheel,
    onPointerDown,
    onPointerMove,
    onPointerUp,
    zoomIn,
    zoomOut,
    fit,
    recenter,
    didPan,
  };
}
