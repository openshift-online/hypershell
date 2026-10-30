// Composition of the dashboard shell. Two peer tabs - the interactive
// promotion-topology Fleet Map and the Agent SDLC funnel view - share the masthead
// and a single release "freight" bar (SDLC-VIS-07). The freight bar's selected
// bundle is lifted here so it drives both tabs in lockstep and survives a tab
// switch. The promotion-path table is still rendered for assistive tech (visually
// hidden). Holds no fleet knowledge: everything is derived from the payloads.

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
  TabTitleText,
  Tabs,
} from "@patternfly/react-core";
import { useMemo } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import {
  useE2eRuns,
  useFleet,
  usePromotion,
  useSdlc,
  useSdlcAgents,
  useTopology,
} from "../query/hooks";
import { useRouter } from "../routing/router";
import { useSessionExpired } from "../auth/session-expiry";
import { buildMapModel } from "../../domain/map/model";
import {
  agentRowsByEnv,
  aggregateAgentsByName,
} from "../../domain/sdlc-agents";
import type { FleetData } from "../../domain/fleet";
import type { PromotionData } from "../../domain/promotion";
import type { TopologyData } from "../../domain/topology";
import { messages } from "../../messages";
import { AgentSdlcView } from "./agent-sdlc/agent-sdlc-view";
import { E2eRunView } from "./e2e-run/e2e-run-view";
import { FreightBar } from "./map/freight-bar";
import { TopologyMap } from "./map/topology-map";
import { PlaneSection } from "./plane-section";
import { PromotionTable } from "./promotion-table";
import { SessionExpired } from "./session-expired";
import styles from "./app.module.css";

const EMPTY_FLEET: FleetData = { instances: [] };
const EMPTY_TOPOLOGY: TopologyData = {};
const EMPTY_PROMOTION: PromotionData = {
  order: [],
  environments: {},
  releases: [],
  releaseByDigest: {},
  frontier: null,
};

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
  const intl = useIntl();
  const sessionExpired = useSessionExpired();
  const promotion = usePromotion();
  const fleet = useFleet();
  const topology = useTopology();
  const sdlc = useSdlc();
  const sdlcAgents = useSdlcAgents();
  const e2e = useE2eRuns();

  // The URL is the single source of truth for navigation state (FLT-NAV-05):
  // the active tab and the shared freight-bar bundle both come from the route and
  // all transitions go through typed navigation actions (FLT-NAV-04).
  const { route, navigate } = useRouter();
  const activeTab = route.tab;
  // The shared freight bar's selection drives both tabs and survives a tab switch
  // (SDLC-VIS-07); toggling the same bundle clears it.
  const selectedBundleSeed = route.bundle;
  const onSelectBundle = (seed: string) => {
    navigate({ type: "toggleBundle", seed });
  };

  // The map needs promotion + fleet together; it is driven by the promotion plane
  // (its backbone) and enriches with fleet metrics opportunistically, so a lagging
  // or failed fleet plane degrades the cards' metrics without blanking the map.
  const fleetData = fleet.data?.data ?? EMPTY_FLEET;
  // Topology enriches the map's per-spoke attribution; like fleet it degrades
  // gracefully (a lagging/failed plane leaves the attribution unresolved, not blank).
  const topologyData = topology.data?.data ?? EMPTY_TOPOLOGY;
  // Promotion also backs the SHARED freight bar in the chrome (outside the map's
  // own plane section), so read it here with a safe fallback.
  const promotionData = promotion.data?.data ?? EMPTY_PROMOTION;

  // Freight-bar deployment counts need the map model's nodes; computed here (above
  // the session short-circuit) so the hook order stays stable.
  const model = useMemo(
    () => buildMapModel(promotionData, fleetData, topologyData),
    [promotionData, fleetData, topologyData],
  );

  // Per-cluster SDLC agent rows for the map's Agents tab (SDLC-AGT-11).
  const sdlcAgentsData = sdlcAgents.data?.data;
  const agentsByEnvMap = useMemo(
    () => (sdlcAgentsData ? agentRowsByEnv(sdlcAgentsData) : undefined),
    [sdlcAgentsData],
  );

  // Per-agent-name map for the board's column runtime strip (SDLC-AGT-08). An
  // agent running in multiple environments is summed (aggregateAgentsByName), not
  // collapsed to one arbitrary environment's numbers.
  const agentByName = useMemo(
    () => (sdlcAgentsData ? aggregateAgentsByName(sdlcAgentsData) : undefined),
    [sdlcAgentsData],
  );

  // A stale session (any API 401, detected in the query cache) replaces the whole dashboard
  // with a single sign-in prompt rather than a grid of generic error panels. Hooks above
  // still run (rules of hooks); we just short-circuit the render.
  if (sessionExpired) {
    return <SessionExpired />;
  }

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
    <Page masthead={masthead} isContentFilled>
      <PageSection isFilled hasBodyWrapper={false} className={styles.section}>
        <Stack hasGutter className={styles.stack}>
          {/* The single shared freight bar, in the same location above both tabs. */}
          <StackItem>
            <FreightBar
              releaseByDigest={promotionData.releaseByDigest}
              nodes={model.nodes}
              selectedSeed={selectedBundleSeed}
              onSelectBundle={onSelectBundle}
            />
          </StackItem>
          <StackItem isFilled className={styles.mapItem}>
            <Tabs
              activeKey={activeTab}
              onSelect={(_event, key) => {
                navigate({ type: "tab", tab: key as typeof activeTab });
              }}
              aria-label={intl.formatMessage(messages.sdlcTabs)}
            >
              <Tab
                eventKey="map"
                title={
                  <TabTitleText>
                    <FormattedMessage {...messages.sdlcTabFleetMap} />
                  </TabTitleText>
                }
              >
                <PlaneSection
                  titleMessage={messages.sectionMap}
                  query={promotion}
                >
                  {(data) => (
                    <TopologyMap
                      e2e={{
                        promotion: promotionData,
                        data: e2e.data?.data,
                        unavailable: e2e.isError || e2e.data?.stale,
                        showHistoryLink: true,
                      }}
                      promotion={data}
                      fleet={fleetData}
                      topology={topologyData}
                      selectedBundleSeed={selectedBundleSeed}
                      onSelectBundle={onSelectBundle}
                      agentsByEnv={agentsByEnvMap}
                      agentsPlaneStale={sdlcAgents.data?.stale ?? false}
                    />
                  )}
                </PlaneSection>
              </Tab>
              <Tab
                eventKey="sdlc"
                title={
                  <TabTitleText>
                    <FormattedMessage {...messages.sdlcTabAgent} />
                  </TabTitleText>
                }
              >
                <PlaneSection titleMessage={messages.sdlcSection} query={sdlc}>
                  {(data) => (
                    <AgentSdlcView
                      data={data}
                      selectedBundleSeed={selectedBundleSeed}
                      agents={agentByName}
                      planeStale={sdlcAgents.data?.stale ?? false}
                    />
                  )}
                </PlaneSection>
              </Tab>
              <Tab
                eventKey="e2e"
                title={
                  <TabTitleText>
                    <FormattedMessage {...messages.e2eTabTitle} />
                  </TabTitleText>
                }
              >
                <PlaneSection
                  titleMessage={messages.e2eSection}
                  query={e2e}
                  hideTitle
                >
                  {(data) => (
                    <E2eRunView data={data} promotion={promotionData} />
                  )}
                </PlaneSection>
              </Tab>
            </Tabs>
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
