// Composition of the dashboard shell. The interactive promotion-topology map is
// the single visible view; the promotion-path table is still rendered, but only
// for assistive tech (visually hidden) so the graph's content stays available to
// screen readers (data-architecture.spec §3.5 a11y). Holds no fleet knowledge.

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

import { useFleet, usePromotion } from "../query/hooks";
import type { FleetData } from "../../domain/fleet";
import { messages } from "../../messages";
import { TopologyMap } from "./map/topology-map";
import { PlaneSection } from "./plane-section";
import { PromotionTable } from "./promotion-table";

const EMPTY_FLEET: FleetData = { instances: [] };

/** Visually hidden, but present in the accessibility tree (sr-only pattern). */
const srOnly: React.CSSProperties = {
  position: "absolute",
  width: 1,
  height: 1,
  padding: 0,
  margin: -1,
  overflow: "hidden",
  clip: "rect(0, 0, 0, 0)",
  whiteSpace: "nowrap",
  border: 0,
};

export function App(): React.ReactElement {
  const promotion = usePromotion();
  const fleet = useFleet();

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
            <div style={srOnly}>
              <PlaneSection
                titleMessage={messages.sectionPromotion}
                query={promotion}
              >
                {(data) => <PromotionTable promotion={data} />}
              </PlaneSection>
            </div>
          </StackItem>
        </Stack>
      </PageSection>
    </Page>
  );
}
