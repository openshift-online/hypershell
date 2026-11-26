// The deployed bundles in the masthead (SDLC-VIS-07): one compact entry per release
// bundle that runs on at least one instance, showing its identicon and deployment
// count. It keeps the deployed bundles one click away while the freight bar is
// hidden. Activating an entry selects or clears the bundle, like its freight card.

import { Badge, Button, Flex, FlexItem, Tooltip } from "@patternfly/react-core";
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
import { BundleIcon } from "../shared/bundle-icon";

export interface DeployedBundlesProps {
  readonly releaseByDigest: Readonly<Record<string, ReleaseBundle>>;
  readonly nodes: readonly MapNode[];
  readonly selectedSeed: string | null;
  readonly onSelectBundle: (seed: string) => void;
}

export function DeployedBundles({
  releaseByDigest,
  nodes,
  selectedSeed,
  onSelectBundle,
}: DeployedBundlesProps): React.ReactElement | null {
  const intl = useIntl();
  const deployed = bundleList(releaseByDigest)
    .map((bundle) => ({
      bundle,
      seed: seedForBundle(bundle),
      count: deployedCount(bundle, nodes),
    }))
    .filter((entry) => entry.count > 0);
  if (deployed.length === 0) {
    return null;
  }
  return (
    <Flex
      role="group"
      aria-label={intl.formatMessage(messages.freightDeployedBundles)}
      alignItems={{ default: "alignItemsCenter" }}
      spaceItems={{ default: "spaceItemsXs" }}
      flexWrap={{ default: "nowrap" }}
    >
      {deployed.map(({ bundle, seed, count }) => {
        const label = intl.formatMessage(messages.freightDeployedBundle, {
          version: bundle.version,
          alias: identiName(seed),
          count,
        });
        return (
          <FlexItem key={seed}>
            <Tooltip content={label}>
              <Button
                variant="plain"
                aria-label={label}
                aria-pressed={selectedSeed === seed}
                isClicked={selectedSeed === seed}
                onClick={() => {
                  onSelectBundle(seed);
                }}
              >
                <Flex
                  alignItems={{ default: "alignItemsCenter" }}
                  spaceItems={{ default: "spaceItemsXs" }}
                  flexWrap={{ default: "nowrap" }}
                >
                  <BundleIcon seed={seed} size={24} />
                  <Badge isRead screenReaderText="">
                    {count}
                  </Badge>
                </Flex>
              </Button>
            </Tooltip>
          </FlexItem>
        );
      })}
    </Flex>
  );
}
