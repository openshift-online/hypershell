import { render, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { describe, expect, it } from "vitest";
import { IntlProvider } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { messages } from "../messages";
import {
  ApiReliabilityTrendCard,
  ReliabilitySummaryCard,
} from "./reliability-dashboard-widgets";

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

const requestRateMetric: OperationalMetric = {
  hourlyTrend: {
    points: [
      { label: "2026-09-15T12:00", value: 10 },
      { label: "2026-09-15T13:00", value: 12 },
    ],
  },
  id: "api-request-rate",
  unit: "req/s",
  value: "12.50",
};

const errorRateMetric: OperationalMetric = {
  id: "api-error-rate",
  unit: "%",
  value: "1.25",
};

const latencyMetric: OperationalMetric = {
  hourlyTrend: {
    points: [
      { label: "2026-09-15T12:00", value: 0.09 },
      { label: "2026-09-15T13:00", value: 0.084 },
    ],
  },
  id: "api-latency",
  unit: "sec",
  value: "0.084",
};

describe("ReliabilitySummaryCard", () => {
  it("lists current values for all three reliability metrics", () => {
    renderWithIntl(
      <ReliabilitySummaryCard
        metrics={[requestRateMetric, errorRateMetric, latencyMetric]}
      />,
    );

    expect(screen.getByText("Request rate")).toBeTruthy();
    expect(screen.getByText("Error rate")).toBeTruthy();
    expect(screen.getByText("Median latency")).toBeTruthy();
    expect(screen.getByText(/12\.50/u)).toBeTruthy();
    expect(screen.getByText(/1\.25/u)).toBeTruthy();
    expect(screen.getByText(/0\.084/u)).toBeTruthy();
  });

  it("shows a trend indicator when hourly change meets the threshold", () => {
    renderWithIntl(
      <ReliabilitySummaryCard metrics={[requestRateMetric, errorRateMetric]} />,
    );

    expect(
      screen.getByRole("button", { name: /20% increase in Request rate/u }),
    ).toBeTruthy();
  });

  it("shows metric unavailable when a summary metric is omitted", () => {
    renderWithIntl(<ReliabilitySummaryCard metrics={[requestRateMetric]} />);

    expect(screen.getAllByText("Metric unavailable")).toHaveLength(2);
  });
});

describe("ApiReliabilityTrendCard", () => {
  const requestRateTitle = "API request rate";
  const errorRateTitle = "API error rate";
  const latencyTitle = "API latency";

  it("renders the current value and sparkline when hourly trend exists", () => {
    renderWithIntl(
      <ApiReliabilityTrendCard
        metric={requestRateMetric}
        title={requestRateTitle}
      />,
    );

    expect(screen.getAllByText(/API request rate/u).length).toBeGreaterThan(0);
    expect(screen.getByText("Last 24 hours")).toBeTruthy();
  });

  it("omits the sparkline when hourly trend is absent", () => {
    renderWithIntl(
      <ApiReliabilityTrendCard
        metric={errorRateMetric}
        title={errorRateTitle}
      />,
    );

    expect(screen.getAllByText(/API error rate/u).length).toBeGreaterThan(0);
    expect(screen.queryByText("Last 24 hours")).toBeNull();
  });

  it("renders the metric unavailable empty state when metric is missing", () => {
    renderWithIntl(
      <ApiReliabilityTrendCard metric={undefined} title={latencyTitle} />,
    );

    expect(screen.getByText("Metric unavailable")).toBeTruthy();
  });
});
