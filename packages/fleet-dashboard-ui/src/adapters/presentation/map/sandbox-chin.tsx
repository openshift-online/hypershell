// The per-cluster sandbox "chin": a compact horizontal bar chart that sits beneath
// the gateways widget in the detail panel. One row per managed cluster - a truncated
// cluster label, a proportional teal bar, and the count - so the spread of active
// sandboxes across an instance's clusters reads at a glance. Pure CSS bars (no SVG
// chart lib): bar width is the cluster's share of the busiest cluster, so the
// largest bar always fills the track and the rest scale relative to it.

import { useIntl } from "react-intl";

import type { SandboxClusterCount } from "../../../domain/fleet";
import { messages } from "../../../messages";
import { SANDBOX_COLOR, SANDBOX_TRACK } from "./colors";
import styles from "./map-details.module.css";

export interface SandboxChinProps {
  readonly clusters: readonly SandboxClusterCount[];
}

export function SandboxChin({
  clusters,
}: SandboxChinProps): React.ReactElement | null {
  const intl = useIntl();
  if (clusters.length === 0) {
    return null;
  }
  // Scale bars to the busiest cluster so the chart uses the full width; floor the
  // divisor at 1 so an all-zero snapshot never divides by zero.
  const max = Math.max(1, ...clusters.map((c) => c.count));
  return (
    <ul className={styles.sandboxChin}>
      {clusters.map((c) => {
        const pct = Math.round((c.count / max) * 100);
        const rowLabel = intl.formatMessage(messages.sandboxClusterRow, {
          cluster: c.cluster,
          count: c.count,
        });
        return (
          <li
            key={c.cluster}
            className={styles.sandboxRow}
            aria-label={rowLabel}
          >
            <span className={styles.sandboxRowName} title={c.cluster}>
              {c.cluster}
            </span>
            <span
              className={styles.sandboxRowTrack}
              style={{ background: SANDBOX_TRACK }}
              aria-hidden="true"
            >
              <span
                className={styles.sandboxRowBar}
                style={{ background: SANDBOX_COLOR, width: `${String(pct)}%` }}
              />
            </span>
            <span className={styles.sandboxRowCount} aria-hidden="true">
              {c.count}
            </span>
          </li>
        );
      })}
    </ul>
  );
}
