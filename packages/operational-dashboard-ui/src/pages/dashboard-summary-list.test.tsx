/* eslint-disable formatjs/no-literal-string-in-jsx */
import { render, screen } from "@testing-library/react";
import { TrendUpIcon } from "@patternfly/react-icons";
import { describe, expect, it } from "vitest";

import { DashboardSummaryList } from "./dashboard-widget";

describe("DashboardSummaryList", () => {
  it("renders labels, bold values, and trend indicators", () => {
    const { container } = render(
      <DashboardSummaryList
        aria-label="Resource summary"
        items={[
          {
            indicator: {
              content: <TrendUpIcon aria-hidden />,
              tone: "positive",
            },
            key: "gateways",
            label: "Gateways",
            value: "12",
          },
          {
            key: "healthy-resources",
            label: "Healthy resources",
            value: "35",
          },
        ]}
      />,
    );

    expect(screen.getByText("Gateways")).toBeTruthy();
    expect(screen.getByText("12")).toBeTruthy();
    expect(screen.getByText("Healthy resources")).toBeTruthy();
    expect(screen.getByText("35")).toBeTruthy();
    expect(
      container.querySelector(
        ".hypershell-dashboard-summary-list__indicator--positive",
      ),
    ).toBeTruthy();
    expect(container.querySelector("strong")?.textContent).toBe("12");
  });
});
