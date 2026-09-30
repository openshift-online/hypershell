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

import { useInstances, usePromotion } from "../query/hooks";
import { messages } from "../../messages";
import { InstancesTable } from "./instances-table";
import { PlaneSection } from "./plane-section";
import { PromotionTable } from "./promotion-table";

export function App(): React.ReactElement {
  const promotion = usePromotion();
  const instances = useInstances();

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
