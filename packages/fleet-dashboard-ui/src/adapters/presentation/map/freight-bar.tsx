// The "freight" bar: one card per release bundle (newest -> oldest), each showing
// the bundle's identicon + identiname, version, short digest and a count of how
// many instances currently run it. Selecting a card highlights its deployments and
// opens the bundle in the drawer. Bundle identity is opaque server data, so no
// fleet knowledge leaks here.

import { Badge, Flex, FlexItem } from "@patternfly/react-core";
import { useEffect, useRef } from "react";
import { useIntl } from "react-intl";

import {
  bundleList,
  deployedCount,
  seedForBundle,
} from "../../../domain/map/bundles";
import { shortDigest } from "../../../domain/map/digest";
import { identiName } from "../../../domain/map/identiname";
import type { MapNode } from "../../../domain/map/model";
import type { ReleaseBundle } from "../../../domain/promotion";
import { messages } from "../../../messages";
import { CARD_BG, CARD_STROKE, TONE_COLOR } from "./colors";
import { Identicon } from "./identicon";
import { ReleaseTime } from "./release-time";
import styles from "./topology-map.module.css";

const ICON = 34;

export interface FreightBarProps {
  readonly releaseByDigest: Readonly<Record<string, ReleaseBundle>>;
  readonly nodes: readonly MapNode[];
  readonly selectedSeed: string | null;
  readonly onSelectBundle: (seed: string) => void;
}

export function FreightBar({
  releaseByDigest,
  nodes,
  selectedSeed,
  onSelectBundle,
}: FreightBarProps): React.ReactElement | null {
  const intl = useIntl();

  // Translate a vertical mouse wheel into horizontal scroll so a plain mouse (no
  // horizontal wheel/trackpad) can still page through the strip. Attached as a
  // non-passive native listener because React's onWheel is passive and can't
  // preventDefault the page scroll we're redirecting.
  const scrollRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      // Respect genuine horizontal intent (trackpad swipe / shift+wheel) and let the
      // page scroll normally once the strip has no hidden overflow left to consume.
      if (Math.abs(e.deltaY) <= Math.abs(e.deltaX)) return;
      if (el.scrollWidth <= el.clientWidth) return;
      el.scrollLeft += e.deltaY;
      e.preventDefault();
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => {
      el.removeEventListener("wheel", onWheel);
    };
  }, []);

  const bundles = bundleList(releaseByDigest);
  if (bundles.length === 0) {
    return null;
  }

  return (
    <Flex
      ref={scrollRef}
      aria-label={intl.formatMessage(messages.sectionReleases)}
      // flexShrink:0 so the release strip keeps its natural height inside the map card's
      // flex column: the stage below has min-height, so without this the freight cards get
      // squeezed and their bottoms (deploy count + date) clip under overflowX:auto.
      style={{ overflowX: "auto", paddingBottom: 4, flexShrink: 0 }}
      flexWrap={{ default: "nowrap" }}
    >
      {bundles.map((bundle) => {
        const seed = seedForBundle(bundle);
        const count = deployedCount(bundle, nodes);
        const selected = selectedSeed === seed;
        const select = () => {
          onSelectBundle(seed);
        };
        const onKeyDown = (e: React.KeyboardEvent) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            select();
          }
        };
        return (
          <FlexItem key={seed} className={styles.freightSlot}>
            {/* key={seed} on the FlexItem means React mounts a fresh node only for a
                NEWLY-arrived bundle, so the entry animation plays just for the new card -
                existing cards keep their DOM and don't replay. The slot opens first
                (.freightSlot, sliding siblings right), then the card drops into it
                (.freightDrop). */}
            <div
              role="button"
              tabIndex={0}
              aria-pressed={selected}
              aria-label={bundle.version}
              onClick={select}
              onKeyDown={onKeyDown}
              className={styles.freightDrop}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 8,
                minWidth: 190,
                padding: "6px 10px",
                background: CARD_BG,
                border: `${selected ? "2" : "1"}px solid ${
                  selected ? TONE_COLOR.info : CARD_STROKE
                }`,
                borderRadius: 8,
                cursor: "pointer",
              }}
            >
              <svg width={ICON} height={ICON} aria-hidden="true">
                <Identicon seed={seed} x={0} y={0} size={ICON} />
              </svg>
              <div style={{ lineHeight: 1.3 }}>
                <div style={{ fontWeight: 700 }}>{bundle.version}</div>
                <div style={{ fontSize: 11, opacity: 0.7 }}>
                  {identiName(seed)}
                </div>
                {bundle.digest ? (
                  <div
                    style={{ fontSize: 10, opacity: 0.55 }}
                    title={bundle.digest}
                  >
                    {shortDigest(bundle.digest)}
                  </div>
                ) : null}
                {bundle.date ? (
                  <ReleaseTime
                    iso={bundle.date}
                    mode="relative"
                    style={{ fontSize: 10, opacity: 0.55 }}
                  />
                ) : null}
              </div>
              <Badge isRead={count === 0} screenReaderText="">
                {intl.formatMessage(messages.freightDeployed, { count })}
              </Badge>
            </div>
          </FlexItem>
        );
      })}
    </Flex>
  );
}
