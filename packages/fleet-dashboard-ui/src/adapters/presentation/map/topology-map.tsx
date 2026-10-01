// The interactive promotion-topology map: the spec's centrepiece visual. It
// composes the pure model/layout (buildMapModel + computeLayout) with the viewport
// controller (pan/zoom/momentum) and the SVG layers (lane bands, promotion rails +
// gates, instance node cards), plus a minimap, a release "freight" bar and a detail
// panel. It holds ZERO fleet knowledge: everything is derived from the promotion +
// fleet payloads at runtime (firewall - data-architecture.spec §3.5).

import {
  Button,
  Drawer,
  DrawerContent,
  DrawerContentBody,
  DrawerPanelBody,
  DrawerPanelContent,
} from "@patternfly/react-core";
import CompressArrowsAltIcon from "@patternfly/react-icons/dist/esm/icons/compress-arrows-alt-icon";
import SearchMinusIcon from "@patternfly/react-icons/dist/esm/icons/search-minus-icon";
import SearchPlusIcon from "@patternfly/react-icons/dist/esm/icons/search-plus-icon";
import { useEffect, useMemo, useRef, useState } from "react";
import { useIntl } from "react-intl";

import { deployedFor, seedForBundle } from "../../../domain/map/bundles";
import { computeLayout, type NodeBox } from "../../../domain/map/layout";
import { buildMapModel } from "../../../domain/map/model";
import type { FleetData } from "../../../domain/fleet";
import type { PromotionData } from "../../../domain/promotion";
import { messages } from "../../../messages";
import { EDGE_STROKE, LANE_STROKE, TEXT_COLOR, TEXT_SUBTLE } from "./colors";
import { MapEdges } from "./edges";
import { FreightBar } from "./freight-bar";
import { MapDetails, type MapSelection } from "./map-details";
import { MapNodeCard } from "./map-node";
import { MiniMap } from "./minimap";
import { f } from "./svg";
import styles from "./topology-map.module.css";
import { useMapViewport } from "./use-map-viewport";

export interface TopologyMapProps {
  readonly promotion: PromotionData;
  readonly fleet: FleetData;
}

/** Cubic bezier with horizontal control handles, from (x1,y1) to (x2,y2). */
function hBezier(x1: number, y1: number, x2: number, y2: number): string {
  const midX = (x1 + x2) / 2;
  return `M ${f(x1)} ${f(y1)} C ${f(midX)} ${f(y1)}, ${f(midX)} ${f(y2)}, ${f(x2)} ${f(y2)}`;
}

export function TopologyMap({
  promotion,
  fleet,
}: TopologyMapProps): React.ReactElement {
  const intl = useIntl();
  const model = useMemo(
    () => buildMapModel(promotion, fleet),
    [promotion, fleet],
  );
  const layout = useMemo(() => computeLayout(model), [model]);

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
  const [selection, setSelection] = useState<MapSelection | null>(null);

  const selectedBundleSeed = selection?.kind === "bundle" ? selection.id : null;

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

  const selectNode = (id: string) => {
    if (didPan()) {
      return;
    }
    setSelection((cur) =>
      cur?.kind === "node" && cur.id === id ? null : { kind: "node", id },
    );
  };
  const selectGate = (id: string) => {
    if (didPan()) {
      return;
    }
    setSelection((cur) =>
      cur?.kind === "gate" && cur.id === id ? null : { kind: "gate", id },
    );
  };
  const selectBundle = (seed: string) => {
    setSelection((cur) =>
      cur?.kind === "bundle" && cur.id === seed
        ? null
        : { kind: "bundle", id: seed },
    );
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
  // Column headers sit in the top margin, above the first lane band.
  const headerY = Math.max((layout.lanes[0]?.y ?? 96) - 40, 24);
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
      <FreightBar
        releaseByDigest={promotion.releaseByDigest}
        nodes={model.nodes}
        selectedSeed={selectedBundleSeed}
        onSelectBundle={selectBundle}
      />

      <div className={styles.stage}>
        <Drawer isExpanded={selection !== null} isInline position="end">
          <DrawerContent
            panelContent={
              <DrawerPanelContent isResizable defaultSize="420px" minSize="320px">
                <DrawerPanelBody className={styles.panelBody}>
                  {selection ? (
                    <MapDetails
                      model={model}
                      releaseByDigest={promotion.releaseByDigest}
                      selection={selection}
                      onClose={() => {
                        setSelection(null);
                      }}
                      onSelectBundle={selectBundle}
                    />
                  ) : null}
                </DrawerPanelBody>
              </DrawerPanelContent>
            }
          >
          <DrawerContentBody>
            <div ref={canvasRef} className={styles.canvas}>
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
                {/* lane bands + labels (behind everything); one lane per cloud provider */}
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
                      {(lane.provider ?? lane.key).toUpperCase()}
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
                    />
                  );
                })}
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
