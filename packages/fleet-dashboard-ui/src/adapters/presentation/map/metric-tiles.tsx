import { Tooltip } from "@patternfly/react-core";
import InfoAltIcon from "@patternfly/react-icons/dist/esm/icons/info-alt-icon";
import { useId, useRef } from "react";
import type { MessageDescriptor } from "react-intl";
import { FormattedMessage, useIntl } from "react-intl";

import type { MapNode } from "../../../domain/map/model";
import { messages } from "../../../messages";
import type { SvgColor } from "./colors";
import { LOGINS_COLOR, SANDBOX_COLOR, USER_COLOR } from "./colors";
import styles from "./map-details.module.css";

/** Clamp an index into [0, n-1]. */
function clampIndex(i: number, n: number): number {
  return Math.max(0, Math.min(n - 1, i));
}

/** The population value at the cursor's sample, or the live scalar when the cursor
 *  is not engaged / the history does not reach that sample. */
function valueAt(
  history: readonly number[],
  active: number | null,
  fallback: number,
): number {
  if (active !== null && active < history.length) {
    return history[active] ?? fallback;
  }
  return fallback;
}

/** A field's info "i": a keyboard-focusable PatternFly tooltip explaining the metric.
 *  Mirrors the detail panel's field-row help so the Users/Logins tiles are
 *  self-explanatory. */
function InfoTip({ info }: { info: MessageDescriptor }): React.ReactElement {
  const intl = useIntl();
  return (
    <Tooltip content={<FormattedMessage {...info} />}>
      <span
        className={styles.infoTip}
        role="button"
        tabIndex={0}
        aria-label={intl.formatMessage(messages.moreInfo)}
      >
        <InfoAltIcon />
      </span>
    </Tooltip>
  );
}

/**
 * A single population metric as a square-aspect tile: an uppercase eyebrow label (with
 * an optional info "i"), a big headline number, and a mini area sparkline of the last
 * day's history. When the shared cursor is engaged the headline shows the value at the
 * hovered sample and the sparkline draws a crosshair + marker at that point, so all
 * three tiles read the same moment in time.
 */
function MetricTile({
  label,
  info,
  value,
  history,
  color,
  active,
}: {
  readonly label: MessageDescriptor;
  readonly info?: MessageDescriptor;
  readonly value: number;
  readonly history: readonly number[];
  readonly color: SvgColor;
  readonly active: number | null;
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
        {info ? <InfoTip info={info} /> : null}
      </span>
      <span className={styles.metricTileValue}>{value}</span>
      <div className={styles.metricTileSparkWrap}>
        <TileSparkline history={history} color={color} />
        <TileCursor history={history} active={active} color={color} />
      </div>
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
 * The shared temporal cursor over one sparkline: a vertical dash at the active sample
 * and a round marker on the line at that point. Positioned with CSS percentages (not
 * inside the stretched SVG) so the dash stays a crisp 1.5px and the marker stays round
 * regardless of the tile's width. All three tiles draw the cursor at the same sample
 * index, so the dash reads as one shared moment across the row. Purely decorative - the
 * accessible value is the headline number and the live readout above the tiles.
 */
function TileCursor({
  history,
  active,
  color,
}: {
  readonly history: readonly number[];
  readonly active: number | null;
  readonly color: SvgColor;
}): React.ReactElement | null {
  if (active === null || history.length < 2 || active >= history.length) {
    return null;
  }
  const n = history.length;
  const leftPct = (active / (n - 1)) * 100;
  const max = Math.max(1, ...history);
  const bottomPct = ((history[active] ?? 0) / max) * 100;
  return (
    <>
      <span
        className={styles.metricTileCursor}
        style={{ left: `${String(leftPct)}%` }}
        aria-hidden="true"
      />
      <span
        className={styles.metricTileDot}
        style={{
          left: `${String(leftPct)}%`,
          bottom: `${String(bottomPct)}%`,
          background: color,
        }}
        aria-hidden="true"
      />
    </>
  );
}

/**
 * The detail panel's population tiles - sandboxes, registered users and 7-day unique
 * logins - as a row of square cards under the gateways donut, with a Grafana-style
 * shared temporal cursor. Hovering (mouse/pen), dragging (touch) or arrowing (keyboard)
 * over the row selects one sample on the shared 24h history grid: all three tiles show
 * that sample's value and draw a crosshair at it, the hovered date/time is shown above
 * the row, and the gateways donut (a sibling, driven by the same `active`) updates to
 * the gateway phase mix at that moment. A live region announces the sample for screen
 * readers. Each population keeps its own accent hue (teal / purple / blue).
 */
export function MetricTiles({
  node,
  active,
  onActive,
}: {
  readonly node: MapNode;
  readonly active: number | null;
  readonly onActive: (index: number | null) => void;
}): React.ReactElement {
  const intl = useIntl();
  const rowRef = useRef<HTMLDivElement>(null);
  const n = node.historyTimes.length;
  const interactive = n >= 2;

  const setFromClientX = (clientX: number): void => {
    const el = rowRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    if (rect.width <= 0) return;
    const frac = (clientX - rect.left) / rect.width;
    onActive(clampIndex(Math.round(frac * (n - 1)), n));
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLDivElement>): void => {
    switch (e.key) {
      case "ArrowLeft":
      case "ArrowDown":
        e.preventDefault();
        onActive(active === null ? n - 1 : clampIndex(active - 1, n));
        break;
      case "ArrowRight":
      case "ArrowUp":
        e.preventDefault();
        onActive(active === null ? n - 1 : clampIndex(active + 1, n));
        break;
      case "Home":
        e.preventDefault();
        onActive(0);
        break;
      case "End":
        e.preventDefault();
        onActive(n - 1);
        break;
      case "Escape":
        if (active !== null) {
          e.preventDefault();
          onActive(null);
        }
        break;
      default:
        break;
    }
  };

  const sandboxesNow = valueAt(node.sandboxHistory, active, node.sandboxes);
  const usersNow = valueAt(node.userHistory, active, node.users ?? 0);
  const loginsNow = valueAt(node.loginsHistory, active, node.logins ?? 0);

  const activeTs =
    active !== null && active < node.historyTimes.length
      ? node.historyTimes[active]
      : undefined;
  const timeStr =
    activeTs !== undefined
      ? intl.formatDate(activeTs * 1000, {
          month: "short",
          day: "numeric",
          hour: "numeric",
          minute: "2-digit",
        })
      : "";
  // The scrubber's spoken value: the full sample readout (date + all three populations)
  // while the cursor is engaged, falling back to the discoverability hint when idle, so
  // a keyboard user hears the whole moment on each arrow step from one aria-valuetext.
  const valueText =
    activeTs !== undefined
      ? intl.formatMessage(messages.historyReadout, {
          time: timeStr,
          sandboxes: sandboxesNow,
          users: usersNow,
          logins: loginsNow,
        })
      : intl.formatMessage(messages.historyCursorHint);

  // When there is a shared axis to scrub, the row is a horizontal slider: an interactive
  // role (so it is focusable and announces as a control), min/max/now along the sample
  // index, and the pointer + keyboard handlers. With fewer than two samples there is
  // nothing to scrub, so it degrades to a plain, inert group of tiles.
  const rowProps: React.HTMLAttributes<HTMLDivElement> & {
    tabIndex?: number;
  } = interactive
    ? {
        role: "slider",
        "aria-label": intl.formatMessage(messages.historyCursorGroup),
        "aria-orientation": "horizontal",
        "aria-valuemin": 0,
        "aria-valuemax": n - 1,
        "aria-valuenow": active ?? n - 1,
        "aria-valuetext": valueText,
        tabIndex: 0,
        onKeyDown,
        onPointerMove: (e: React.PointerEvent<HTMLDivElement>) => {
          setFromClientX(e.clientX);
        },
        onPointerDown: (e: React.PointerEvent<HTMLDivElement>) => {
          setFromClientX(e.clientX);
        },
        onPointerLeave: () => {
          onActive(null);
        },
        onPointerUp: (e: React.PointerEvent<HTMLDivElement>) => {
          // A touch/pen tap reads a sample then lifts; clear so the cursor does not
          // stick. A mouse keeps its hover position until the pointer leaves.
          if (e.pointerType !== "mouse") onActive(null);
        },
        onPointerCancel: () => {
          onActive(null);
        },
        onBlur: () => {
          onActive(null);
        },
      }
    : { role: "group" };

  return (
    <div>
      {/* The hovered sample's date/time (or a discoverability hint when idle). The
          scrubber's aria-valuetext carries the same information for assistive tech, so
          this strip is purely visual. */}
      <div className={styles.historyStrip} aria-hidden="true">
        {activeTs !== undefined ? (
          <FormattedMessage
            {...messages.historyAsOf}
            values={{ time: timeStr }}
          />
        ) : interactive ? (
          <span className={styles.historyHint}>
            <FormattedMessage {...messages.historyCursorHint} />
          </span>
        ) : null}
      </div>
      <div ref={rowRef} className={styles.metricTiles} {...rowProps}>
        <MetricTile
          label={messages.sectionSandboxes}
          value={sandboxesNow}
          history={node.sandboxHistory}
          color={SANDBOX_COLOR}
          active={active}
        />
        <MetricTile
          label={messages.sectionUsers}
          info={messages.infoUsers}
          value={usersNow}
          history={node.userHistory}
          color={USER_COLOR}
          active={active}
        />
        <MetricTile
          label={messages.sectionLogins}
          info={messages.infoLogins}
          value={loginsNow}
          history={node.loginsHistory}
          color={LOGINS_COLOR}
          active={active}
        />
      </div>
    </div>
  );
}
