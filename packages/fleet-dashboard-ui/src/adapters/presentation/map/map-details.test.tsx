import { render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { describe, expect, it } from "vitest";

import type { FleetData } from "../../../domain/fleet";
import type {
  PromotionData,
  PromotionEnvironment,
} from "../../../domain/promotion";
import { buildMapModel } from "../../../domain/map/model";
import { createMemoryHistory } from "../../routing/history-port";
import { RouterProvider } from "../../routing/router";
import { MapDetails } from "./map-details";

// Server-supplied fields every PromotionEnvironment carries; only syncRetries
// matters here, so the rest default to absent.
function env(
  overrides: Partial<PromotionEnvironment> & { name: string },
): PromotionEnvironment {
  return {
    activeRelease: null,
    proposedRelease: null,
    gates: [],
    activeDigest: null,
    proposedDigest: null,
    role: null,
    provider: null,
    envLabel: null,
    cluster: null,
    argoHealth: null,
    argoSync: null,
    syncRetries: null,
    consoleUrl: null,
    grafanaUrl: null,
    argoUrl: null,
    prState: null,
    prUrl: null,
    analysisUrl: null,
    upToDate: false,
    ...overrides,
  };
}

const emptyFleet: FleetData = { instances: [] };

const noop = (): void => undefined;

function renderGate(syncRetries: number | null): void {
  const promotion: PromotionData = {
    order: ["hi", "hp"],
    environments: {
      hi: env({ name: "hi", role: "hub", syncRetries }),
      hp: env({ name: "hp", role: "hub" }),
    },
    releases: [],
    releaseByDigest: {},
    frontier: null,
  };
  const model = buildMapModel(promotion, emptyFleet);
  render(
    <IntlProvider locale="en" defaultLocale="en">
      <RouterProvider
        history={createMemoryHistory({ path: "/", search: "?gate=hi-%3Ehp" })}
      >
        <MapDetails
          model={model}
          releaseByDigest={{}}
          selection={{ kind: "gate", id: "hi->hp" }}
          onClose={noop}
          onSelectBundle={noop}
          onSelectNode={noop}
        />
      </RouterProvider>
    </IntlProvider>,
  );
}

describe("MapDetails - gate sync attempts", () => {
  it("shows the Sync attempts row with the count when known", () => {
    renderGate(9);
    expect(screen.getByText("Sync attempts: 9")).toBeTruthy();
  });

  it("omits the Sync attempts row when the count is unknown", () => {
    renderGate(null);
    expect(screen.queryByText("Sync attempts")).toBeNull();
  });

  it("shows a genuine 0 rather than omitting it", () => {
    renderGate(0);
    expect(screen.getByText("Sync attempts: 0")).toBeTruthy();
  });
});
