import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { IntlProvider } from "react-intl";

import { createDashboardOperations } from "../application/dashboard-operations";
import { DashboardUiProvider } from "../dashboard-ui-provider";
import { messages } from "../messages";
import {
  DashboardSecondaryNav,
  type DashboardSecondaryNavActive,
} from "./dashboard-secondary-nav";

const messageCatalog = Object.fromEntries(
  Object.values(messages).map((message) => [
    message.id,
    message.defaultMessage,
  ]),
);

function renderNav(active: DashboardSecondaryNavActive) {
  const navigate = vi.fn();
  const dashboard = createDashboardOperations({
    controlPlane: {
      getOperationalMetrics: () =>
        Promise.resolve({
          lastSuccessfulRefresh: new Date(),
          metrics: [],
        }),
      getReliabilityMetrics: () =>
        Promise.resolve({
          lastSuccessfulRefresh: new Date(),
          metrics: [],
        }),
    },
  });

  render(
    <IntlProvider locale="en" messages={messageCatalog}>
      <DashboardUiProvider
        dashboard={dashboard}
        navigation={{ collectionHref: "/", navigate }}
      >
        <DashboardSecondaryNav active={active} />
      </DashboardUiProvider>
    </IntlProvider>,
  );

  return { navigate };
}

describe("DashboardSecondaryNav", () => {
  it("marks Operational as active and targets both dashboard routes", () => {
    renderNav("operational");

    const operational = screen.getByRole("link", { name: "Operational" });
    const reliability = screen.getByRole("link", { name: "Reliability" });

    expect(operational.getAttribute("href")).toBe("/dashboard");
    expect(operational.getAttribute("aria-current")).toBe("page");
    expect(reliability.getAttribute("href")).toBe("/dashboard/reliability");
    expect(reliability.getAttribute("aria-current")).toBeNull();
  });

  it("marks Reliability as active on the reliability route", () => {
    renderNav("reliability");

    expect(
      screen
        .getByRole("link", { name: "Reliability" })
        .getAttribute("aria-current"),
    ).toBe("page");
    expect(
      screen
        .getByRole("link", { name: "Operational" })
        .getAttribute("aria-current"),
    ).toBeNull();
  });

  it("navigates through the host navigation port", () => {
    const { navigate } = renderNav("operational");

    fireEvent.click(screen.getByRole("link", { name: "Reliability" }));

    expect(navigate).toHaveBeenCalledWith("/dashboard/reliability");
  });
});
