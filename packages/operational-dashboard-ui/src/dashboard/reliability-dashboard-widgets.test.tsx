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
  unit: "requests/sec",
  value: "12.500",
};

const errorRateMetric: OperationalMetric = {
  id: "api-error-rate",
  unit: "%",
  value: "1.250",
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

const reconciliationFailuresMetric: OperationalMetric = {
  id: "reconciliation-failures",
  unit: "count",
  value: "23",
};

const reconciliationRetriesMetric: OperationalMetric = {
  id: "reconciliation-retries",
  unit: "count",
  value: "16",
};

const reconciliationLagMetric: OperationalMetric = {
  id: "reconciliation-lag",
  unit: "sec",
  value: "0.047",
};

const staleResourceStatusMetric: OperationalMetric = {
  hourlyTrend: {
    points: [
      { label: "2026-09-15T12:00", value: 18 },
      { label: "2026-09-15T13:00", value: 22 },
    ],
  },
  id: "stale-resource-status-count",
  unit: "count",
  value: "22",
};

describe("ReliabilitySummaryCard", () => {
  it("lists current values for all reliability metrics", () => {
    renderWithIntl(
      <ReliabilitySummaryCard
        metrics={[
          requestRateMetric,
          errorRateMetric,
          latencyMetric,
          reconciliationFailuresMetric,
          reconciliationRetriesMetric,
          reconciliationLagMetric,
          staleResourceStatusMetric,
        ]}
      />,
    );

    expect(screen.getByText("Request rate")).toBeTruthy();
    expect(screen.getByText("Error rate")).toBeTruthy();
    expect(screen.getByText("Median latency")).toBeTruthy();
    expect(screen.getByText(/12\.50/u)).toBeTruthy();
    expect(screen.getByText(/1\.25/u)).toBeTruthy();
    expect(screen.getByText(/0\.084/u)).toBeTruthy();
    expect(screen.getByText("Reconciliation failures")).toBeTruthy();
    expect(screen.getByText("Reconciliation retries")).toBeTruthy();
    expect(screen.getByText("Reconciliation lag")).toBeTruthy();
    expect(screen.getByText("Stale resource status")).toBeTruthy();
    expect(screen.getByText("23 failures")).toBeTruthy();
    expect(screen.getByText("16 retries")).toBeTruthy();
    expect(screen.getByText("0.047 sec")).toBeTruthy();
    expect(screen.getByText("22 count")).toBeTruthy();
  });

  it("shows the stale resource status trend indicator", () => {
    renderWithIntl(
      <ReliabilitySummaryCard metrics={[staleResourceStatusMetric]} />,
    );

    expect(
      screen.getByRole("button", {
        name: /22% increase in Stale resource status/u,
      }),
    ).toBeTruthy();
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

    expect(screen.getAllByText("Metric unavailable")).toHaveLength(6);
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

    expect(screen.getByText("1.250 %")).toBeTruthy();
    expect(screen.getByText("(last 5 minutes)")).toBeTruthy();
    expect(screen.queryByText("Last 24 hours")).toBeNull();
  });

  it("renders the metric unavailable empty state when metric is missing", () => {
    renderWithIntl(
      <ApiReliabilityTrendCard metric={undefined} title={latencyTitle} />,
    );

    expect(screen.getByText("Metric unavailable")).toBeTruthy();
  });
});
