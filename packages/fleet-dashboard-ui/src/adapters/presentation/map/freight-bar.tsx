// The "freight" bar: one card per release bundle (newest -> oldest), each showing
// the bundle's identicon + identiname, version, short digest and a count of how
// many instances currently run it. Selecting a card highlights its deployments and
// opens the bundle in the drawer. Bundle identity is opaque server data, so no
// fleet knowledge leaks here.

import { Badge, Flex, FlexItem } from "@patternfly/react-core";
import { useIntl } from "react-intl";

import {
  bundleList,
  deployedCount,
  seedForBundle,
} from "../../../domain/map/bundles";
import { identiName } from "../../../domain/map/identiname";
import type { MapNode } from "../../../domain/map/model";
import type { ReleaseBundle } from "../../../domain/promotion";
import { messages } from "../../../messages";
import { CARD_BG, CARD_STROKE, TONE_COLOR } from "./colors";
import { Identicon } from "./identicon";

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
  const bundles = bundleList(releaseByDigest);
  if (bundles.length === 0) {
    return null;
  }

  return (
    <Flex
      aria-label={intl.formatMessage(messages.sectionReleases)}
      style={{ overflowX: "auto", paddingBottom: 4 }}
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
          <FlexItem key={seed}>
            <div
              role="button"
              tabIndex={0}
              aria-pressed={selected}
              aria-label={bundle.version}
              onClick={select}
              onKeyDown={onKeyDown}
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
                  <div style={{ fontSize: 10, opacity: 0.55 }}>
                    {bundle.digest}
                  </div>
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
