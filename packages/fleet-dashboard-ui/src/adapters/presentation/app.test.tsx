import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { describe, expect, it } from "vitest";

import type { FleetData } from "../../domain/fleet";
import type { Plane } from "../../domain/plane";
import type { PromotionData } from "../../domain/promotion";
import type {
  FleetApi,
  InstancesData,
  TopologyData,
} from "../../application/ports";
import { FleetApiProvider } from "../query/api-context";
import { App } from "./app";

function plane<T>(data: T): Plane<T> {
  return {
    data,
    generatedAt: "2026-01-01T00:00:00Z",
    stale: false,
    error: null,
  };
}

function renderApp(api: FleetApi): void {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <IntlProvider locale="en" defaultLocale="en">
      <QueryClientProvider client={queryClient}>
        <FleetApiProvider value={api}>
          <App />
        </FleetApiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("App", () => {
  it("renders promotion environments and governing instances straight from the payload", async () => {
    const promotion: PromotionData = {
      order: ["staging", "canary"],
      environments: {
        canary: {
          name: "canary",
          activeRelease: "v2026",
          proposedRelease: null,
          gates: [
            { name: "g", phase: "success", governingInstance: "gov-from-data" },
          ],
        },
        staging: {
          name: "staging",
          activeRelease: "v2025",
          proposedRelease: null,
          gates: [],
        },
      },
      releases: ["v2026", "v2025"],
    };
    const empty = {
      getFleet: () => Promise.resolve(plane<FleetData>({ instances: [] })),
      getPromotion: () => Promise.resolve(plane(promotion)),
      getTopology: () =>
        Promise.resolve(plane<TopologyData>({ nodes: [], edges: [] })),
      getInstances: () =>
        Promise.resolve(plane<InstancesData>({ instances: [] })),
    } satisfies FleetApi;

    renderApp(empty);

    // Server-defined order: staging before canary.
    expect(await screen.findByText("staging")).toBeDefined();
    expect(await screen.findByText("canary")).toBeDefined();
    // Governing instance came from gate data, not any hard-coded mapping.
    expect(await screen.findByText("gov-from-data")).toBeDefined();
  });

  it("renders instance rows from the payload", async () => {
    const api = {
      getFleet: () => Promise.resolve(plane<FleetData>({ instances: [] })),
      getPromotion: () =>
        Promise.resolve(
          plane<PromotionData>({ order: [], environments: {}, releases: [] }),
        ),
      getTopology: () =>
        Promise.resolve(plane<TopologyData>({ nodes: [], edges: [] })),
      getInstances: () =>
        Promise.resolve(
          plane<InstancesData>({
            instances: [
              {
                name: "inst-alpha",
                role: "spoke",
                provider: "example-cloud",
                region: "somewhere-1",
                health: "Healthy",
              },
            ],
          }),
        ),
    } satisfies FleetApi;

    renderApp(api);

    expect(await screen.findByText("inst-alpha")).toBeDefined();
    expect(await screen.findByText("example-cloud")).toBeDefined();
  });
});
