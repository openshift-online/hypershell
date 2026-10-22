import type { GateExecutionProps } from "../e2e-run/gate-execution";
// The interactive promotion-topology map: the spec's centrepiece visual. It
// composes the pure model/layout (buildMapModel + computeLayout) with the viewport
// controller (pan/zoom/momentum) and the SVG layers (provider lane bands, promotion
// rails + gates on the centered hub spine, instance node cards), plus a minimap, a
// release "freight" bar and a detail panel. It holds ZERO fleet knowledge: everything
// is derived from the promotion + fleet payloads at runtime (firewall -
// data-architecture.spec §3.5).

import {
  Button,
  Drawer,
  DrawerContent,
  DrawerContentBody,
  DrawerPanelBody,
  DrawerPanelContent,
} from "@patternfly/react-core";
import CompressArrowsAltIcon from "@patternfly/react-icons/dist/esm/icons/compress-arrows-alt-icon";
import ExclamationCircleIcon from "@patternfly/react-icons/dist/esm/icons/exclamation-circle-icon";
import SearchMinusIcon from "@patternfly/react-icons/dist/esm/icons/search-minus-icon";
import SearchPlusIcon from "@patternfly/react-icons/dist/esm/icons/search-plus-icon";
import { useEffect, useMemo, useRef, useState } from "react";
import { useIntl } from "react-intl";

import type { SdlcAgentMetrics } from "../../../domain/sdlc-agents";
import { deployedFor, seedForBundle } from "../../../domain/map/bundles";
import { computeLayout, type NodeBox } from "../../../domain/map/layout";
import { buildMapModel } from "../../../domain/map/model";
import type { FleetData } from "../../../domain/fleet";
import type { PromotionData } from "../../../domain/promotion";
import type { TopologyData } from "../../../domain/topology";
import { isHealthUnavailable } from "../../../domain/status";
import { messages } from "../../../messages";
import {
  EDGE_STROKE,
  LANE_STROKE,
  promotionRing,
  TEXT_COLOR,
  TEXT_SUBTLE,
  TONE_COLOR,
} from "./colors";
import { useRoute, useRouter } from "../../routing/router";
import { MapEdges } from "./edges";
import { MapFlights } from "./flights";
import { MapDetails, type MapSelection } from "./map-details";
import { MapNodeCard } from "./map-node";
import { MiniMap } from "./minimap";
import { f } from "./svg";
import styles from "./topology-map.module.css";
import { useFlights } from "./use-flights";
import { useMapViewport } from "./use-map-viewport";

export interface TopologyMapProps {
  readonly e2e?: Omit<GateExecutionProps, "target">;
  readonly promotion: PromotionData;
  readonly fleet: FleetData;
  /** Per-hub spoke layout, used to attribute gateways/sandboxes to their spoke. */
  readonly topology: TopologyData;
  // The release freight bar is a single shared control lifted into the dashboard
  // shell so it stays in lockstep with the Agent SDLC tab (SDLC-VIS-07). The map
  // consumes the selected bundle as a controlled value and reports changes up.
  readonly selectedBundleSeed: string | null;
  readonly onSelectBundle: (seed: string) => void;
  /** SDLC agent rows grouped by environment, for the per-cluster Agents tab. */
  readonly agentsByEnv?: ReadonlyMap<string, readonly SdlcAgentMetrics[]>;
}

/** Cubic bezier with horizontal control handles, from (x1,y1) to (x2,y2). */
function hBezier(x1: number, y1: number, x2: number, y2: number): string {
  const midX = (x1 + x2) / 2;
  return `M ${f(x1)} ${f(y1)} C ${f(midX)} ${f(y1)}, ${f(midX)} ${f(y2)}, ${f(x2)} ${f(y2)}`;
}

export function TopologyMap({
  e2e,
  promotion,
  fleet,
  topology,
  selectedBundleSeed,
  onSelectBundle,
  agentsByEnv,
}: TopologyMapProps): React.ReactElement {
  const intl = useIntl();
  const model = useMemo(
    () => buildMapModel(promotion, fleet, topology),
    [promotion, fleet, topology],
  );
  const layout = useMemo(() => computeLayout(model), [model]);

  // Fleet-level severe signal: how many instances report NO Argo health at all (the
  // "unknown" tone). Distinct from "Degraded" - absent health means the dashboard can't
  // reach the cluster's Argo, so its state is genuinely unknown. Surfaced as a banner
  // ONLY when there is at least one, because it's a loud "something may be unreachable"
  // alert, not a routine status (per the node-level flashing ring in map-node).
  const unavailableCount = useMemo(
    () => model.nodes.filter((n) => isHealthUnavailable(n.argoHealth)).length,
    [model.nodes],
  );

  // Track the canvas box's live pixel aspect ratio so the viewBox can be re-fit to
  // it: the box fills the screen height (constant) while its width flexes when the
  // drawer opens, and the content fills that box edge-to-edge at any width.
  const canvasRef = useRef<HTMLDivElement | null>(null);
  const [containerAspect, setContainerAspect] = useState(0);
  useEffect(() => {
    const el = canvasRef.current;
    if (!el || typeof ResizeObserver === "undefined") {
      return;
    }
    const ro = new ResizeObserver((entries) => {
      const box = entries[0]?.contentRect;
      if (box && box.width > 0 && box.height > 0) {
        setContainerAspect(box.width / box.height);
      }
    });
    ro.observe(el);
    return () => {
      ro.disconnect();
    };
  }, []);

  const {
    svgRef,
    viewBox,
    viewport,
    onWheel,
    onPointerDown,
    onPointerMove,
    onPointerUp,
    zoomIn,
    zoomOut,
    fit,
    recenter,
    didPan,
  } = useMapViewport(layout.width, layout.height, containerAspect);
  // The node/gate selection lives in the URL (FLT-NAV-09); the shared bundle
  // selection arrives as a prop (route.bundle). The drawer's selection is derived:
  // a node/gate takes the drawer, otherwise the shared bundle fills it. One source of
  // truth per concern (the URL), no mirrored local state (FLT-NAV-05).
  const route = useRoute();
  const { navigate } = useRouter();
  const selection: MapSelection | null = useMemo(() => {
    if (route.map.node !== null) {
      return { kind: "node", id: route.map.node };
    }
    if (route.map.gate !== null) {
      return { kind: "gate", id: route.map.gate };
    }
    if (selectedBundleSeed !== null) {
      return { kind: "bundle", id: selectedBundleSeed };
    }
    return null;
  }, [route.map.node, route.map.gate, selectedBundleSeed]);

  // Close the drawer, clearing the node/gate selection and the shared bundle that
  // otherwise fills it (FLT-NAV-09).
  const closeSelection = () => {
    navigate({ type: "mapClose" });
  };

  // Esc closes the details drawer. PatternFly's inline Drawer does not trap focus
  // or handle Escape itself, so we listen at the document level, but only while a
  // selection is open, leaving Esc untouched when the panel is closed. A nested
  // dialog (the expanded log Modal in the gate drawer) must handle Escape first, so
  // we ignore the key when it targets an open dialog and leave the gate drawer open
  // (mirrors the E2E run view guard).
  useEffect(() => {
    if (selection === null) {
      return;
    }
    const onKeyDown = (e: KeyboardEvent) => {
      const target = e.target;
      const inDialog =
        target instanceof Element && target.closest('[role="dialog"]') !== null;
      if (e.key === "Escape" && !e.defaultPrevented && !inDialog) {
        navigate({ type: "mapClose" });
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [selection, navigate]);

  // Promotion fly-ins: identicons arc from the promoting-FROM node to the node they
  // land on, and each landing jolts the whole canvas (screen shake). Both collapse to
  // nothing under prefers-reduced-motion (handled in useFlights + CSS).
  const { flights, holdSeedById } = useFlights(model);

  // Promotion-state ring colour per node, so a flying copy's outline matches the ring the
  // card will show at rest (no colour jump when the copy lands and the card takes over).
  const ringByNodeId = useMemo(
    () => new Map(model.nodes.map((n) => [n.id, promotionRing(n.state)])),
    [model.nodes],
  );

  const highlighted = useMemo(() => {
    if (!selectedBundleSeed) {
      return new Set<string>();
    }
    const bundle = Object.values(promotion.releaseByDigest).find(
      (b) => seedForBundle(b) === selectedBundleSeed,
    );
    if (!bundle) {
      return new Set<string>();
    }
    return new Set(deployedFor(bundle, model.nodes).map((n) => n.id));
  }, [promotion.releaseByDigest, model.nodes, selectedBundleSeed]);

  // Selecting a node/gate is a discrete navigation step (FLT-NAV-09). The pan guard
  // keeps a drag from registering as a click; the reducer handles toggle-off and the
  // node/gate mutual exclusion. The shared bundle stays selected but a node/gate wins
  // the drawer (see the `selection` derivation above), so one thing shows at a time.
  const selectNode = (id: string) => {
    if (didPan()) {
      return;
    }
    navigate({ type: "mapSelectNode", id });
  };
  const selectGate = (id: string) => {
    if (didPan()) {
      return;
    }
    navigate({ type: "mapSelectGate", id });
  };
  // Bundle selection lives in the shared freight bar; its shell-provided handler
  // toggles the seed and the reducer drops any node/gate selection so the bundle
  // takes the drawer.
  const selectBundle = (seed: string) => {
    onSelectBundle(seed);
  };
  // From a node-card identicon: suppress when the press was actually a pan.
  const selectBundleOnMap = (seed: string) => {
    if (didPan()) {
      return;
    }
    selectBundle(seed);
  };

  const firstCol = layout.columns[0];
  const changeX = firstCol ? firstCol.x - 90 : 40;
  const changeY = layout.hubSpineY;
  // Column headers sit in the top margin, above the topmost node row.
  const topNodeY = layout.nodes.reduce(
    (min, n) => Math.min(min, n.y),
    Number.POSITIVE_INFINITY,
  );
  const headerY = Math.max(
    (Number.isFinite(topNodeY) ? topNodeY : 96) - 40,
    24,
  );
  const changePts = [
    `${f(changeX)},${f(changeY - 28)}`,
    `${f(changeX + 34)},${f(changeY)}`,
    `${f(changeX)},${f(changeY + 28)}`,
    `${f(changeX - 34)},${f(changeY)}`,
  ].join(" ");
  // Rails from the change diamond into the first column's node(s): the spine starts
  // at the change, so it visibly feeds the root stage like every later gate does.
  const boxById = new Map<string, NodeBox>(layout.nodes.map((b) => [b.id, b]));
  const changeEdges = (model.columns[0]?.nodeIds ?? [])
    .map((id) => boxById.get(id))
    .filter((b): b is NodeBox => b !== undefined)
    .map((b) => ({
      id: b.id,
      d: hBezier(changeX + 34, changeY, b.x, b.cy),
    }));

  return (
    <div className={styles.wrap}>
      <div
        className={styles.stage}
        style={
          {
            "--stage-aspect": String(layout.width / Math.max(1, layout.height)),
          } as React.CSSProperties
        }
      >
        <Drawer isExpanded={selection !== null} isInline position="end">
          <DrawerContent
            panelContent={
              <DrawerPanelContent
                isResizable
                defaultSize={selection?.kind === "gate" ? "560px" : "420px"}
                minSize="320px"
              >
                <DrawerPanelBody className={styles.panelBody}>
                  {selection ? (
                    <MapDetails
                      e2e={e2e}
                      model={model}
                      releaseByDigest={promotion.releaseByDigest}
                      selection={selection}
                      onClose={closeSelection}
                      onSelectBundle={selectBundle}
                      agentsByEnv={agentsByEnv}
                      onSelectNode={(id) => {
                        // From a "Deployed on" chip: jump to that node (not a map
                        // click, so no pan guard). The reducer clears any gate; the
                        // node then wins the drawer over the shared bundle.
                        navigate({ type: "mapSelectNode", id });
                      }}
                    />
                  ) : null}
                </DrawerPanelBody>
              </DrawerPanelContent>
            }
          >
            <DrawerContentBody>
              <div ref={canvasRef} className={styles.canvas}>
                {unavailableCount > 0 ? (
                  <div
                    className={styles.healthAlert}
                    role="status"
                    style={{ background: TONE_COLOR.danger }}
                  >
                    <ExclamationCircleIcon />
                    <span>
                      {intl.formatMessage(messages.mapHealthUnavailable, {
                        count: unavailableCount,
                      })}
                    </span>
                  </div>
                ) : null}

                <div className={styles.toolbar}>
                  <Button
                    variant="control"
                    aria-label={intl.formatMessage(messages.mapZoomIn)}
                    onClick={zoomIn}
                    icon={<SearchPlusIcon />}
                  />
                  <Button
                    variant="control"
                    aria-label={intl.formatMessage(messages.mapZoomOut)}
                    onClick={zoomOut}
                    icon={<SearchMinusIcon />}
                  />
                  <Button
                    variant="control"
                    aria-label={intl.formatMessage(messages.mapFit)}
                    onClick={fit}
                    icon={<CompressArrowsAltIcon />}
                  />
                </div>

                <svg
                  ref={svgRef}
                  className={styles.svg}
                  viewBox={viewBox}
                  role="application"
                  aria-label={intl.formatMessage(messages.mapRegion)}
                  onWheel={onWheel}
                  onPointerDown={onPointerDown}
                  onPointerMove={onPointerMove}
                  onPointerUp={onPointerUp}
                  onPointerCancel={onPointerUp}
                >
                  {/* provider lane bands + labels (behind everything); one lane per
              cloud provider, grouping its clusters. The hub-hosting lane straddles the
              centered spine; the others stack above and below it. */}
                  {layout.lanes.map((lane) => (
                    <g key={lane.key}>
                      <rect
                        x={0}
                        y={lane.y - 8}
                        width={layout.width}
                        height={lane.height + 16}
                        fill="none"
                        stroke={LANE_STROKE}
                        strokeDasharray="2 6"
                      />
                      <text
                        x={14}
                        y={lane.centerY}
                        fontSize={12}
                        fontWeight={700}
                        fill={TEXT_SUBTLE}
                      >
                        {(lane.provider ?? "unassigned").toUpperCase()}
                      </text>
                    </g>
                  ))}

                  {/* env-type band headers: group the columns sharing an env-type (int /
              stage / prod) into one label spanning them (server data) */}
                  {layout.bands.map((band) => (
                    <text
                      key={`${band.label}@${f(band.x)}`}
                      x={band.centerX}
                      y={headerY}
                      textAnchor="middle"
                      fontSize={13}
                      fontWeight={700}
                      fill={TEXT_COLOR}
                    >
                      {band.label.toUpperCase()}
                    </text>
                  ))}

                  {/* rails from the change diamond into the first column's nodes */}
                  {changeEdges.map((e) => (
                    <path
                      key={`change:${e.id}`}
                      d={e.d}
                      fill="none"
                      stroke={EDGE_STROKE}
                      strokeWidth={1.25}
                      strokeOpacity={0.7}
                    />
                  ))}

                  {/* change diamond: head of the promotion spine, left of the root column */}
                  <g>
                    <polygon
                      points={changePts}
                      fill="none"
                      stroke={EDGE_STROKE}
                      strokeWidth={1.5}
                    />
                    <text
                      x={changeX}
                      y={changeY}
                      textAnchor="middle"
                      dominantBaseline="central"
                      fontSize={12}
                      fontWeight={700}
                      fill={TEXT_SUBTLE}
                    >
                      {intl.formatMessage(messages.mapChange)}
                    </text>
                  </g>

                  <MapEdges
                    model={model}
                    layout={layout}
                    selectedGateId={
                      selection?.kind === "gate" ? selection.id : null
                    }
                    onSelectGate={selectGate}
                  />

                  {layout.nodes.map((box) => {
                    const node = model.nodes.find((n) => n.id === box.id);
                    if (!node) {
                      return null;
                    }
                    return (
                      <MapNodeCard
                        key={box.id}
                        node={node}
                        box={box}
                        selected={
                          selection?.kind === "node" && selection.id === box.id
                        }
                        highlighted={highlighted.has(box.id)}
                        onSelect={selectNode}
                        onSelectBundle={selectBundleOnMap}
                        displaySeed={holdSeedById.get(box.id)}
                      />
                    );
                  })}

                  {/* promotion fly-ins ride ABOVE the cards so the bundle visibly
                      travels over the map before slotting into its landing node. A
                      brand-new bundle flies in from the CHANGE diamond's right tip. */}
                  <MapFlights
                    flights={flights}
                    boxById={boxById}
                    ringByNodeId={ringByNodeId}
                    changeAnchor={{ x: changeX + 34, y: changeY }}
                  />
                </svg>

                <div className={styles.mini}>
                  <MiniMap
                    layout={layout}
                    viewport={viewport}
                    onRecenter={recenter}
                  />
                </div>
              </div>
            </DrawerContentBody>
          </DrawerContent>
        </Drawer>
      </div>
    </div>
  );
}
