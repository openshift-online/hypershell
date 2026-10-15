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

function renderNav(active: DashboardSecondaryNavActive, isAdmin = true) {
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
        isAdmin={isAdmin}
        navigation={{ collectionHref: "/", navigate }}
      >
        <DashboardSecondaryNav active={active} />
      </DashboardUiProvider>
    </IntlProvider>,
  );

  return { navigate };
}

describe("DashboardSecondaryNav", () => {
  it("marks Overview as active and targets dashboard routes", () => {
    renderNav("operational");

    const operational = screen.getByRole("link", { name: "Overview" });
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
        .getByRole("link", { name: "Overview" })
        .getAttribute("aria-current"),
    ).toBeNull();
  });

  it("marks Usage as active and makes it available to non-admins", () => {
    renderNav("usage", false);

    const usage = screen.getByRole("link", { name: "Usage" });

    expect(usage.getAttribute("href")).toBe("/dashboard/usage");
    expect(usage.getAttribute("aria-current")).toBe("page");
    expect(screen.getByRole("link", { name: "Overview" })).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Reliability" })).toBeNull();
  });

  it("navigates through the host navigation port", () => {
    const { navigate } = renderNav("operational");

    fireEvent.click(screen.getByRole("link", { name: "Reliability" }));

    expect(navigate).toHaveBeenCalledWith("/dashboard/reliability");
  });

  it("limits non-admin users to Overview", () => {
    renderNav("operational", false);

    expect(screen.getByRole("link", { name: "Overview" })).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Adoption" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Reliability" })).toBeNull();
    expect(screen.getByRole("link", { name: "Usage" })).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Cost" })).toBeNull();
  });
});
