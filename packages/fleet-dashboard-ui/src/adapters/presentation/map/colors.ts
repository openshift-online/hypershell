// Shared SVG palette for the map. SVG fills/strokes can't use PatternFly's
// component props, so we centralise the handful of status + accent colours here as
// PF v6 design-token references (with hex fallbacks for non-browser test renders).
// State is ALWAYS paired with text/!shape elsewhere, so colour is never the sole
// signal (brand spec UI-BRAND-03).

import { fnv1a } from "../../../domain/map/hash";
import type { SemanticTone } from "../../../domain/status";

/** A CSS colour value usable as an SVG fill/stroke. */
export type SvgColor = string;

function token(name: string, fallback: string): SvgColor {
  return `var(${name}, ${fallback})`;
}

/** Status colours, keyed by the domain's semantic tone. */
export const TONE_COLOR: Record<SemanticTone, SvgColor> = {
  success: token("--pf-t--global--color--status--success--default", "#3d7317"),
  warning: token("--pf-t--global--color--status--warning--default", "#dca614"),
  danger: token("--pf-t--global--color--status--danger--default", "#b1380b"),
  info: token("--pf-t--global--color--status--info--default", "#0066cc"),
  unknown: token("--pf-t--global--icon--color--subtle", "#8a8d90"),
};

/** Gateway phase colours (running = success, provisioning = warning, failed = danger). */
export const GATEWAY_COLOR = {
  running: TONE_COLOR.success,
  provisioning: TONE_COLOR.warning,
  failed: TONE_COLOR.danger,
  idle: TONE_COLOR.unknown,
} as const;

/** Surfaces/strokes for cards, lanes and edges on the dark canvas. */
export const CARD_BG = token(
  "--pf-t--global--background--color--secondary--default",
  "#1b1d21",
);
export const CARD_STROKE = token(
  "--pf-t--global--border--color--default",
  "#444548",
);
export const EDGE_STROKE = token(
  "--pf-t--global--border--color--default",
  "#444548",
);
export const LANE_STROKE = token(
  "--pf-t--global--border--color--subtle",
  "#333539",
);
export const TEXT_COLOR = token(
  "--pf-t--global--text--color--regular",
  "#e0e0e0",
);
export const TEXT_SUBTLE = token(
  "--pf-t--global--text--color--subtle",
  "#9a9da0",
);
export const ICON_BG = "#ffffff";

/**
 * Brand hues for the handful of widely-known public cloud vendors, so their chips
 * read in the colour people expect (AWS orange, IBM Cloud blue) rather than an
 * arbitrary hash hue. These are generic, public vendor names - NOT fleet-identifying
 * values - so they stay clear of the identity firewall (data-architecture.spec §3.5);
 * any provider not listed here still falls back to the deterministic hash hue, so no
 * fleet-specific provider inventory is baked into source.
 */
const BRAND_HUE: Record<string, number> = {
  aws: 36, // AWS orange (#ff9900)
  ibm: 216, // IBM Cloud blue (#0f62fe)
};

/**
 * A deterministic hue (0-359) for an opaque label string. Known public cloud
 * vendors get their brand hue; everything else is derived from a visual hash so a
 * given provider/role always draws the same colour WITHOUT baking any fleet-specific
 * provider or role list into source (firewall - data-architecture.spec §3.5).
 */
function hueFor(label: string): number {
  return BRAND_HUE[label.toLowerCase()] ?? fnv1a(`hue:${label}`) % 360;
}

/** Chip colours (dark fill + bright same-hue text) for a cloud provider label. */
export function providerChip(provider: string): {
  readonly bg: SvgColor;
  readonly fg: SvgColor;
} {
  const h = hueFor(provider);
  return {
    bg: `hsl(${String(h)}, 55%, 20%)`,
    fg: `hsl(${String(h)}, 85%, 75%)`,
  };
}

/** Role-badge fill (light, same-hue family) for an opaque role label. */
export function roleBadgeFill(role: string): SvgColor {
  return `hsl(${String(hueFor(role))}, 68%, 66%)`;
}

/** Promotion-state ring colour for a node's identicon. */
export function promotionRing(
  state: "up-to-date" | "promoting" | "behind",
): SvgColor {
  switch (state) {
    case "up-to-date":
      return TONE_COLOR.success;
    case "promoting":
      return TONE_COLOR.warning;
    default:
      return TONE_COLOR.unknown;
  }
}
