import { PageSection, Tab, Tabs, TabTitleText } from "@patternfly/react-core";

import { MockupShell } from "../../shell/mockup-template";
import { DeliveryPipelinePageSurface } from "./delivery-pipeline-surface";
import type { DeliveryState } from "./delivery-pipeline-fixtures";

// Design-review wrapper: the shared shell plus the Glass top-level tabs, which
// are shell chrome owned by the Glass app, not by the Delivery page
// (FLT-DLV-01). The parity story renders the page surface without them.
function GlassTabs() {
  return (
    <PageSection hasBodyWrapper={false} type="tabs" variant="default">
      <Tabs activeKey="delivery" aria-label="Glass views" usePageInsets>
        <Tab eventKey="map" title={<TabTitleText>Fleet Map</TabTitleText>} />
        <Tab eventKey="sdlc" title={<TabTitleText>Agent SDLC</TabTitleText>} />
        <Tab eventKey="e2e" title={<TabTitleText>E2E Runs</TabTitleText>} />
        <Tab
          eventKey="delivery"
          title={<TabTitleText>Delivery</TabTitleText>}
        />
      </Tabs>
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
