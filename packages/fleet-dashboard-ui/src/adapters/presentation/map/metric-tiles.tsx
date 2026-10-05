import { useId } from "react";
import type { MessageDescriptor } from "react-intl";
import { FormattedMessage, useIntl } from "react-intl";

import type { MapNode } from "../../../domain/map/model";
import { messages } from "../../../messages";
import type { SvgColor } from "./colors";
import { LOGINS_COLOR, SANDBOX_COLOR, USER_COLOR } from "./colors";
import styles from "./map-details.module.css";

/**
 * A single population metric as a square-aspect tile: an uppercase eyebrow label, a
 * big headline number, and a mini area sparkline of the last day's history. The three
 * tiles (sandboxes, users, logins) sit in a row under the gateways donut so each
 * population reads as its own compact, comparable card. The number is the accessible
 * value; the sparkline is decorative (aria-hidden) context.
 */
function MetricTile({
  label,
  value,
  history,
  color,
}: {
  readonly label: MessageDescriptor;
  readonly value: number;
  readonly history: readonly number[];
  readonly color: SvgColor;
}): React.ReactElement {
  const intl = useIntl();
  const aria = intl.formatMessage(messages.metricTileAria, {
    label: intl.formatMessage(label),
    value,
  });
  return (
    <div className={styles.metricTile} role="group" aria-label={aria}>
      <span className={styles.metricTileLabel}>
        <FormattedMessage {...label} />
      </span>
      <span className={styles.metricTileValue}>{value}</span>
      <TileSparkline history={history} color={color} />
    </div>
  );
}

/**
 * A filled area sparkline that scales to the tile's width. Drawn in a fixed 100x28
 * user-space box and stretched (preserveAspectRatio="none") so it fills whatever the
 * tile grid gives it. Oldest sample on the left, newest on the right. Renders nothing
 * until there are two samples to draw a band between, so fresh/idle tiles stay clean.
 */
function TileSparkline({
  history,
  color,
}: {
  readonly history: readonly number[];
  readonly color: SvgColor;
}): React.ReactElement | null {
  const gradId = useId();
  if (history.length < 2) return null;

  const W = 100;
  const H = 28;
  const n = history.length;
  const step = W / (n - 1);
  const max = Math.max(1, ...history);
  const xAt = (i: number): number => i * step;
  const yAt = (v: number): number => H - (v / max) * H;

  const top = history.map(
    (v, i) => `${xAt(i).toFixed(2)} ${yAt(v).toFixed(2)}`,
  );
  const area = `M 0 ${String(H)} L ${top.join(" L ")} L ${String(W)} ${String(H)} Z`;
  const line = `M ${top.join(" L ")}`;

  return (
    <svg
      className={styles.metricTileSpark}
      viewBox={`0 0 ${String(W)} ${String(H)}`}
      preserveAspectRatio="none"
      aria-hidden="true"
      focusable="false"
    >
      <defs>
        <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity={0.45} />
          <stop offset="100%" stopColor={color} stopOpacity={0.04} />
        </linearGradient>
      </defs>
      <path d={area} fill={`url(#${gradId})`} stroke="none" />
      <path
        d={line}
        fill="none"
        stroke={color}
        strokeWidth={1.5}
        strokeLinejoin="round"
        strokeLinecap="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}

/**
 * The detail panel's population tiles - sandboxes, registered users and 7-day unique
 * logins - as a row of square cards under the gateways donut. The per-cluster sandbox
 * breakdown is intentionally gone: a detail panel always shows exactly one cluster, so
 * a "by cluster" bar would only ever draw a single row. Each population keeps its own
 * accent hue (teal / purple / blue) so the tiles never read as gateway status phases.
 */
export function MetricTiles({ node }: { node: MapNode }): React.ReactElement {
  return (
    <div className={styles.metricTiles}>
      <MetricTile
        label={messages.sectionSandboxes}
        value={node.sandboxes}
        history={node.sandboxHistory}
        color={SANDBOX_COLOR}
      />
      <MetricTile
        label={messages.sectionUsers}
        value={node.users ?? 0}
        history={node.userHistory}
        color={USER_COLOR}
      />
      <MetricTile
        label={messages.sectionLogins}
        value={node.logins ?? 0}
        history={node.loginsHistory}
        color={LOGINS_COLOR}
      />
    </div>
  );
}
