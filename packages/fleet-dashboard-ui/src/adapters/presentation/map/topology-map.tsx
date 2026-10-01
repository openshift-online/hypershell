// The interactive promotion-topology map: the spec's centrepiece visual. It
// composes the pure model/layout (buildMapModel + computeLayout) with the viewport
// controller (pan/zoom/momentum) and the SVG layers (lane bands, promotion rails +
// gates, instance node cards), plus a minimap, a release "freight" bar and a detail
// panel. It holds ZERO fleet knowledge: everything is derived from the promotion +
// fleet payloads at runtime (firewall - data-architecture.spec §3.5).

import { Button } from "@patternfly/react-core";
import CompressArrowsAltIcon from "@patternfly/react-icons/dist/esm/icons/compress-arrows-alt-icon";
import SearchMinusIcon from "@patternfly/react-icons/dist/esm/icons/search-minus-icon";
import SearchPlusIcon from "@patternfly/react-icons/dist/esm/icons/search-plus-icon";
import { useMemo, useState } from "react";
import { useIntl } from "react-intl";

import { deployedFor, seedForBundle } from "../../../domain/map/bundles";
import { computeLayout } from "../../../domain/map/layout";
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
  } = useMapViewport(layout.width, layout.height);
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

  return (
    <div className={styles.wrap}>
      <FreightBar
        releaseByDigest={promotion.releaseByDigest}
        nodes={model.nodes}
        selectedSeed={selectedBundleSeed}
        onSelectBundle={selectBundle}
      />

      <div className={styles.canvas}>
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
            selectedGateId={selection?.kind === "gate" ? selection.id : null}
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
                selected={selection?.kind === "node" && selection.id === box.id}
                highlighted={highlighted.has(box.id)}
                onSelect={selectNode}
              />
            );
          })}
        </svg>

        <div className={styles.mini}>
          <MiniMap layout={layout} viewport={viewport} onRecenter={recenter} />
        </div>
      </div>

      {selection ? (
        <div className={styles.details}>
          <MapDetails
            model={model}
            releaseByDigest={promotion.releaseByDigest}
            selection={selection}
            onClose={() => {
              setSelection(null);
            }}
          />
        </div>
      ) : null}
    </div>
  );
}
