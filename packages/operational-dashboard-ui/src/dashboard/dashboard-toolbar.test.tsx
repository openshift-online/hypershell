import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { IntlProvider } from "react-intl";

import { createDashboardOperations } from "../application/dashboard-operations";
import { DashboardUiProvider } from "../dashboard-ui-provider";
import { messages } from "../messages";
import {
  DashboardFilterToolbar,
  DashboardNavigation,
} from "./dashboard-toolbar";

const messageCatalog = Object.fromEntries(
  Object.values(messages).map((message) => [
    message.id,
    message.defaultMessage,
  ]),
);

function renderNavigation(updatedMinutesAgo?: number) {
  const onRefresh = vi.fn();
  const dashboard = createDashboardOperations({
    controlPlane: {
      getOperationalMetrics: () =>
        Promise.resolve({ lastSuccessfulRefresh: new Date(), metrics: [] }),
      getReliabilityMetrics: () =>
        Promise.resolve({ lastSuccessfulRefresh: new Date(), metrics: [] }),
      getAdoptionMetrics: () =>
        Promise.resolve({ lastSuccessfulRefresh: new Date(), metrics: [] }),
    },
  });

  render(
    <IntlProvider locale="en" messages={messageCatalog}>
      <DashboardUiProvider
        dashboard={dashboard}
        isAdmin
        navigation={{ collectionHref: "/", navigate: () => undefined }}
      >
        <DashboardNavigation
          active="provisioning"
          isRefreshing={false}
          onRefresh={onRefresh}
          updatedMinutesAgo={updatedMinutesAgo}
        />
      </DashboardUiProvider>
    </IntlProvider>,
  );

  return { onRefresh };
}

describe("DashboardNavigation", () => {
  it("renders the shared tabs, refresh control, and update label", () => {
    const { onRefresh } = renderNavigation(2);

    expect(screen.getByRole("link", { name: "Provisioning" })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Refresh dashboard metrics" }),
    ).toBeTruthy();
    expect(screen.getByText("Updated 2 minutes ago")).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", { name: "Refresh dashboard metrics" }),
    );
    expect(onRefresh).toHaveBeenCalledTimes(1);
  });
});

describe("DashboardFilterToolbar", () => {
  it("renders and invokes the shared range and layout actions", () => {
    const onAddWidgets = vi.fn();
    const onResetToDefault = vi.fn();
    const onTimeRangeChange = vi.fn();

    render(
      <IntlProvider locale="en" messages={messageCatalog}>
        <DashboardFilterToolbar
          onAddWidgets={onAddWidgets}
          onResetToDefault={onResetToDefault}
          onTimeRangeChange={onTimeRangeChange}
          timeRange={7}
        />
      </IntlProvider>,
    );

    expect(screen.getByRole("combobox", { name: "Time range" })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Reset to default layout" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Add widgets" })).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", { name: "Reset to default layout" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Add widgets" }));
    fireEvent.change(screen.getByRole("combobox", { name: "Time range" }), {
      target: { value: "30" },
    });

    expect(onResetToDefault).toHaveBeenCalledTimes(1);
    expect(onAddWidgets).toHaveBeenCalledTimes(1);
    expect(onTimeRangeChange).toHaveBeenCalledWith(30);
  });

  it("can render layout actions without a time-range selector", () => {
    render(
      <IntlProvider locale="en" messages={messageCatalog}>
        <DashboardFilterToolbar
          onAddWidgets={() => undefined}
          onResetToDefault={() => undefined}
          showTimeRange={false}
        />
      </IntlProvider>,
    );

    expect(screen.queryByRole("combobox", { name: "Time range" })).toBeNull();
    expect(
      screen.getByRole("button", { name: "Reset to default layout" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Add widgets" })).toBeTruthy();
  });
});
