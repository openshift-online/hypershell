// Composition of the dashboard shell. Two top-level views, switched by tab: the
// interactive promotion-topology map and the fleet-users usage page. In the map
// view the promotion-path table is still rendered, but only for assistive tech
// (visually hidden) so the graph's content stays available to screen readers
// (data-architecture.spec §3.5 a11y). Holds no fleet knowledge.

import {
  Masthead,
  MastheadBrand,
  MastheadMain,
  Page,
  PageSection,
  Stack,
  StackItem,
  Content,
  Tab,
  Tabs,
  TabTitleText,
} from "@patternfly/react-core";
import { useState } from "react";
import { FormattedMessage } from "react-intl";

import { useFleet, usePromotion } from "../query/hooks";
import type { FleetData } from "../../domain/fleet";
import { messages } from "../../messages";
import { TopologyMap } from "./map/topology-map";
import { PlaneSection } from "./plane-section";
import { PromotionTable } from "./promotion-table";
import { UsersPage } from "./users-page";
import styles from "./app.module.css";

const EMPTY_FLEET: FleetData = { instances: [] };

/** Top-level views; values are Tab event keys. */
type ViewKey = "map" | "users";

/** Views that can be deep-linked via ?view=<key>; map is the default. */
const DEEP_LINKABLE: readonly ViewKey[] = ["map", "users"];

/** Initial view from the ?view= URL parameter (invalid values fall back to map). */
function initialView(): ViewKey {
  const requested = new URLSearchParams(window.location.search).get("view");
  return DEEP_LINKABLE.find((v) => v === requested) ?? "map";
}

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
  const [view, setView] = useState<ViewKey>(initialView);

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
    // isContentFilled: without it PF 6.6.1 gives .pf-v6-c-page__main-container
    // align-self:start, so the main area collapses to content height and a void
    // opens below the panel. isContentFilled applies .pf-m-fill (align-self:stretch),
    // stretching the grid's "main" row to the full viewport (top of the fill chain).
    <Page masthead={masthead} isContentFilled>
      {/* View switch. Plain section (no fill) so the tabs bar hugs the masthead;
          the filled section below keeps the map's fill chain intact. */}
      <PageSection hasBodyWrapper={false} variant="secondary">
        <Tabs
          activeKey={view}
          onSelect={(_event, key) => {
            const next = key as ViewKey;
            setView(next);
            // Keep the URL in sync so the active view survives reload/share.
            const url = new URL(window.location.href);
            url.searchParams.set("view", next);
            window.history.replaceState(null, "", url);
          }}
        >
          <Tab
            eventKey="map"
            title={
              <TabTitleText>
                <FormattedMessage {...messages.tabMap} />
              </TabTitleText>
            }
          />
          <Tab
            eventKey="users"
            title={
              <TabTitleText>
                <FormattedMessage {...messages.tabUsers} />
              </TabTitleText>
            }
          />
        </Tabs>
      </PageSection>
      {/* hasBodyWrapper={false}: PF 6.6.1's PageSection wraps children in a
          .pf-v6-c-page__main-body div that has no flex/grow rule, which severs the
          fill chain (the filled section grows, but the inert body wrapper collapses
          to content height, so Stack's flex:1 has no tall parent to resolve against).
          Rendering the Stack as a direct child of the flex-column .pf-m-fill section
          restores the fill chain down to the canvas. */}
      <PageSection isFilled hasBodyWrapper={false} className={styles.section}>
        {view === "map" ? (
          <Stack hasGutter className={styles.stack}>
            <StackItem isFilled className={styles.mapItem}>
              <PlaneSection
                titleMessage={messages.sectionMap}
                query={promotion}
              >
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
        ) : (
          <UsersPage />
        )}
      </PageSection>
    </Page>
  );
}
