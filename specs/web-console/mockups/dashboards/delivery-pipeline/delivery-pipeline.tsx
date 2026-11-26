import {
  Badge,
  Button,
  Flex,
  FlexItem,
  PageSection,
  Tab,
  Tabs,
  TabTitleText,
  Title,
  Tooltip,
} from "@patternfly/react-core";
import { LayerGroupIcon } from "@patternfly/react-icons";

import { MockupShell } from "../../shell/mockup-template";
import { DeliveryPipelinePageSurface } from "./delivery-pipeline-surface";
import type { DeliveryState } from "./delivery-pipeline-fixtures";
import { initialsDataUri } from "./initials";

// Deployed bundles shown in the masthead while the freight bar is hidden
// (SDLC-VIS-07): identicon plus deployment count.
const DEPLOYED_BUNDLES = [
  { alias: "hazy basil", version: "v20261125", count: 10 },
  { alias: "witty pickle", version: "v20261125", count: 2 },
];

// Design-review wrapper: the shared shell plus the Glass masthead, whose
// top-level tabs sit to the right of the page title (SDLC-VIS-07). The tabs are
// shell chrome owned by the Glass app, not by the Delivery page (FLT-DLV-01).
// The parity story renders the page surface without them.
function GlassTabs() {
  return (
    <PageSection hasBodyWrapper={false} variant="default">
      <Flex
        alignItems={{ default: "alignItemsCenter" }}
        spaceItems={{ default: "spaceItemsXl" }}
      >
        <FlexItem>
          <Title headingLevel="h1" size="xl">
            HyperShell Operational Status
          </Title>
        </FlexItem>
        <FlexItem>
          <Tabs activeKey="delivery" aria-label="Glass views">
            <Tab
              eventKey="map"
              title={<TabTitleText>Fleet Map</TabTitleText>}
            />
            <Tab
              eventKey="sdlc"
              title={<TabTitleText>Agent SDLC</TabTitleText>}
            />
            <Tab eventKey="e2e" title={<TabTitleText>E2E Runs</TabTitleText>} />
            <Tab
              eventKey="delivery"
              title={<TabTitleText>Delivery</TabTitleText>}
            />
          </Tabs>
        </FlexItem>
        <FlexItem align={{ default: "alignRight" }}>
          <Flex
            alignItems={{ default: "alignItemsCenter" }}
            spaceItems={{ default: "spaceItemsSm" }}
          >
            {DEPLOYED_BUNDLES.map((b) => (
              <Tooltip
                key={b.alias}
                content={`${b.version} ${b.alias}: ${String(b.count)} deployments`}
              >
                <Button
                  variant="plain"
                  aria-label={`${b.version} ${b.alias}, ${String(b.count)} deployments`}
                  aria-pressed={false}
                >
                  <Flex
                    alignItems={{ default: "alignItemsCenter" }}
                    spaceItems={{ default: "spaceItemsXs" }}
                    flexWrap={{ default: "nowrap" }}
                  >
                    <img
                      src={initialsDataUri(b.alias)}
                      alt=""
                      width={24}
                      height={24}
                      style={{ borderRadius: 4 }}
                    />
                    <Badge isRead>{b.count}</Badge>
                  </Flex>
                </Button>
              </Tooltip>
            ))}
            <Button
              variant="secondary"
              icon={<LayerGroupIcon />}
              aria-expanded={false}
            >
              Release bundles
            </Button>
          </Flex>
        </FlexItem>
      </Flex>
    </PageSection>
  );
}

export function DeliveryPipelineMockup({ state }: { state: DeliveryState }) {
  return (
    <MockupShell>
      <GlassTabs />
      <DeliveryPipelinePageSurface state={state} />
    </MockupShell>
  );
}
