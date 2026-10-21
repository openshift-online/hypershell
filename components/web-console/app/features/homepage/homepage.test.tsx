import { render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";

import type {
  HomepagePlatformOverview,
  HomepageSummary,
} from "../../adapters/api/homepage";
import { englishMessages } from "../../i18n/catalog";
import { HomepagePageSurface } from "./homepage";

vi.mock("./use-homepage", () => ({
  useHomepagePlatformOverview: vi.fn(),
  useHomepageSummary: vi.fn(),
  useQuickCreateGateway: vi.fn(),
}));

vi.mock("../shell/use-session", () => ({
  useSession: vi.fn(),
}));

const summary: HomepageSummary = {
  gatewayHealth: {
    attention: [
      {
        gatewayId: "gateway-17",
        name: "research-gateway",
        reason: "Connection health is degraded",
        status: "Degraded",
      },
    ],
    attentionCount: 1,
    healthy: 4,
    total: 5,
  },
  supportChannel: {
    channelLabel: "#forum-hypershell",
    description: "Support description",
    title: "Support title",
    url: "https://example.com/support",
  },
};

const overview: HomepagePlatformOverview = {
  clusters: {
    healthy: 5,
    total: 6,
    unhealthyClusters: [
      {
        name: "production-central",
        reason: "Cluster heartbeat is stale",
        status: "Unavailable",
      },
    ],
  },
  gatewayStatuses: {
    available: true,
    counts: { Failed: 2, Provisioning: 5, Running: 35 },
    total: 42,
  },
  sandboxStatuses: {
    available: true,
    counts: { active: 112, expiring: 9, idle: 5, orphaned: 2 },
    total: 128,
  },
};

function renderSurface({
  isAdmin = true,
  platformOverview = overview,
  sessionIsLoading = false,
  summaryValue = summary,
}: {
  isAdmin?: boolean;
  platformOverview?: HomepagePlatformOverview;
  sessionIsLoading?: boolean;
  summaryValue?: HomepageSummary;
} = {}) {
  return render(
    <IntlProvider locale="en" messages={englishMessages}>
      <MemoryRouter>
        <HomepagePageSurface
          isAdmin={isAdmin}
          platformOverview={platformOverview}
          platformOverviewIsLoading={false}
          quickCreate={() => undefined}
          quickCreateError={false}
          quickCreateIsPending={false}
          quickCreateOpen={false}
          sessionIsLoading={sessionIsLoading}
          setQuickCreateOpen={() => undefined}
          summary={summaryValue}
          summaryIsLoading={false}
        />
      </MemoryRouter>
    </IntlProvider>,
  );
}

describe("HomepagePageSurface", () => {
  it("renders API-backed summary and platform values", () => {
    renderSurface();

    expect(screen.getByText("research-gateway")).toBeTruthy();
    expect(screen.getByText("Connection health is degraded")).toBeTruthy();
    expect(
      screen
        .getByRole("link", { name: "research-gateway" })
        .getAttribute("href"),
    ).toBe("/gateways/gateway-17");
    expect(screen.getByText("Support title")).toBeTruthy();
    expect(screen.getByText("Cluster heartbeat is stale")).toBeTruthy();
    expect(screen.getByText("42")).toBeTruthy();
    expect(screen.getByText("128")).toBeTruthy();
  });

  it("renders the non-administrator dashboard card", () => {
    renderSurface({ isAdmin: false, platformOverview: undefined });

    expect(screen.getByText("Your dashboard")).toBeTruthy();
    expect(screen.queryByText("Platform overview")).toBeNull();
  });

  it("keeps the role card neutral while the session loads", () => {
    renderSurface({ sessionIsLoading: true });

    expect(screen.getByText("Dashboard")).toBeTruthy();
    expect(screen.queryByText("Your dashboard")).toBeNull();
    expect(screen.queryByText("Platform overview")).toBeNull();
  });

  it("isolates a failed platform source", () => {
    renderSurface({
      platformOverview: {
        clusters: overview.clusters,
        gatewayStatuses: overview.gatewayStatuses,
        sandboxStatuses: { available: false },
      },
    });

    expect(screen.getByText("42")).toBeTruthy();
    expect(
      screen.getAllByText("Platform overview is unavailable").length,
    ).toBeGreaterThan(0);
  });

  it("shows unavailable states for summary and support data", () => {
    renderSurface({
      summaryValue: { gatewayHealth: { available: false } },
    });

    expect(screen.getByText("Gateway status is unavailable")).toBeTruthy();
    expect(screen.getByText("Support channel is unavailable")).toBeTruthy();
  });
});
