// Composition of the dashboard shell. Wires each plane's query hook to a section;
// holds no fleet knowledge itself.

import {
  Masthead,
  MastheadBrand,
  MastheadMain,
  Page,
  PageSection,
  Stack,
  StackItem,
  Content,
} from "@patternfly/react-core";
import { FormattedMessage } from "react-intl";

import { useFleet, useInstances, usePromotion } from "../query/hooks";
import type { FleetData } from "../../domain/fleet";
import { messages } from "../../messages";
import { InstancesTable } from "./instances-table";
import { TopologyMap } from "./map/topology-map";
import { PlaneSection } from "./plane-section";
import { PromotionTable } from "./promotion-table";

const EMPTY_FLEET: FleetData = { instances: [] };

export function App(): React.ReactElement {
  const promotion = usePromotion();
  const fleet = useFleet();
  const instances = useInstances();

  // The map needs promotion + fleet together; it is driven by the promotion plane
  // (its backbone) and enriches with fleet metrics opportunistically, so a lagging
  // or failed fleet plane degrades the cards' metrics without blanking the map.
  const fleetData = fleet.data?.data ?? EMPTY_FLEET;

  const masthead = (
    <Masthead>
      <MastheadMain>
        <MastheadBrand>
          <Content component="h1">
            <FormattedMessage {...messages.appTitle} />
          </Content>
        </MastheadBrand>
      </MastheadMain>
    </Masthead>
  );

  return (
    <Page masthead={masthead}>
      <PageSection>
        <Stack hasGutter>
          <StackItem>
            <PlaneSection titleMessage={messages.sectionMap} query={promotion}>
              {(data) => <TopologyMap promotion={data} fleet={fleetData} />}
            </PlaneSection>
          </StackItem>
          <StackItem>
            <PlaneSection
              titleMessage={messages.sectionPromotion}
              query={promotion}
            >
              {(data) => <PromotionTable promotion={data} />}
            </PlaneSection>
          </StackItem>
          <StackItem>
            <PlaneSection
              titleMessage={messages.sectionInstances}
              query={instances}
            >
              {(data) => <InstancesTable instances={data} />}
            </PlaneSection>
          </StackItem>
        </Stack>
      </PageSection>
    </Page>
  );
}
