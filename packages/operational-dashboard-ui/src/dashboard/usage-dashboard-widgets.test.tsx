import { render, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { describe, expect, it } from "vitest";
import { IntlProvider } from "react-intl";

import type { UsageMetric } from "../application/dashboard-types";
import { messages } from "../messages";
import {
  UsageGatewayActivityCard,
  UsageSandboxLifetimeCard,
  UsageSandboxUtilizationCard,
  UsageTrendCard,
} from "./usage-dashboard-widgets";

const messageCatalog = Object.fromEntries(
  Object.values(messages).map((message) => [
    message.id,
    message.defaultMessage,
  ]),
);

function renderWithIntl(ui: ReactElement) {
  return render(
    <IntlProvider locale="en" messages={messageCatalog}>
      {ui}
    </IntlProvider>,
  );
}

function expectSummaryRows(container: HTMLElement, count: number) {
  expect(
    container.querySelector(".hypershell-dashboard-summary-list"),
  ).toBeTruthy();
  expect(
    container.querySelectorAll(".hypershell-dashboard-summary-list strong"),
  ).toHaveLength(count);
}

const requestVolumeMetric: UsageMetric = {
  id: "gateway-request-volume",
  trend: {
    isPartial: true,
    points: [
      { label: "2026-10-14", value: 120 },
      { label: "2026-10-15", value: null },
    ],
  },
  unit: "requests",
  value: "1240000",
};

const activeGatewaysMetric: UsageMetric = {
  activeSharePercent: "74.23",
  id: "active-gateways",
  total: "97",
  unit: "gateways",
  value: "72",
};

const sandboxUtilizationMetric: UsageMetric = {
  id: "sandbox-utilization",
  total: "340",
  unit: "sandboxes",
  value: "214",
};

const sandboxLifetimeMetric: UsageMetric = {
  id: "sandbox-lifetime",
  sandboxLifetime: {
    mean: "67320",
    p50: "29520",
    p95: "367200",
  },
  value: "67320",
};

const gatewayRequestVolumeTitle =
  messages.usageWidgetGatewayRequestVolume.defaultMessage;
const gatewayErrorRateTitle =
  messages.usageWidgetGatewayErrorRate.defaultMessage;

describe("usage dashboard widgets", () => {
  it("renders request values and partial-data notices", () => {
    renderWithIntl(
      <UsageTrendCard
        metric={requestVolumeMetric}
        title={gatewayRequestVolumeTitle}
      />,
    );

    expect(screen.getByText(/1240000 requests/u)).toBeTruthy();
    expect(screen.getByText("Requests in the selected range")).toBeTruthy();
    expect(screen.getByText("Partial data")).toBeTruthy();
  });

  it("renders active gateway inventory details", () => {
    const { container } = renderWithIntl(
      <UsageGatewayActivityCard metric={activeGatewaysMetric} />,
    );

    expect(screen.getByText("72 gateways")).toBeTruthy();
    expect(screen.getByText("Provisioned gateways")).toBeTruthy();
    expect(screen.getByText("97")).toBeTruthy();
    expect(screen.getByText("74.23%")).toBeTruthy();
    expectSummaryRows(container, 2);
  });

  it("renders sandbox utilization counts", () => {
    const { container } = renderWithIntl(
      <UsageSandboxUtilizationCard metric={sandboxUtilizationMetric} />,
    );

    expect(screen.getByText("Active sandboxes")).toBeTruthy();
    expect(screen.getAllByText("214").length).toBeGreaterThan(0);
    expect(screen.getByText("Provisioned sandboxes")).toBeTruthy();
    expect(screen.getByText("340")).toBeTruthy();
    expectSummaryRows(container, 2);
    expect(
      container.querySelector(
        ".hypershell-dashboard-utilization-chart__donut svg",
      ),
    ).toBeTruthy();
  });

  it("renders sandbox lifetime percentiles", () => {
    const { container } = renderWithIntl(
      <UsageSandboxLifetimeCard metric={sandboxLifetimeMetric} />,
    );

    expect(screen.getAllByText("18h 42m").length).toBeGreaterThan(0);
    expect(screen.getByText("P50 lifetime")).toBeTruthy();
    expect(screen.getByText("8h 12m")).toBeTruthy();
    expect(screen.getByText("P95 lifetime")).toBeTruthy();
    expect(screen.getByText("4d 6h")).toBeTruthy();
    expectSummaryRows(container, 2);
  });

  it("renders partial lifetime percentiles without inventing a mean", () => {
    renderWithIntl(
      <UsageSandboxLifetimeCard
        metric={{
          ...sandboxLifetimeMetric,
          sandboxLifetime: { p50: "100", p95: "300" },
          value: "NaN",
        }}
      />,
    );

    expect(screen.getByText("Metric could not be determined")).toBeTruthy();
    expect(screen.getByText("1m 40s")).toBeTruthy();
    expect(screen.getByText("5m 00s")).toBeTruthy();
  });

  it("renders unavailable states when metrics are omitted", () => {
    renderWithIntl(
      <UsageTrendCard metric={undefined} title={gatewayErrorRateTitle} />,
    );
    renderWithIntl(<UsageGatewayActivityCard metric={undefined} />);
    renderWithIntl(<UsageSandboxUtilizationCard metric={undefined} />);
    renderWithIntl(<UsageSandboxLifetimeCard metric={undefined} />);

    expect(
      screen.getAllByText("Metric unavailable").length,
    ).toBeGreaterThanOrEqual(4);
  });
});
