import { act, render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { IntlProvider } from "react-intl";
import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";

import { createDashboardOperations } from "../application/dashboard-operations";
import { DashboardUiProvider } from "../dashboard-ui-provider";
import { messages } from "../messages";
import { ProvisioningDashboardPage } from "./provisioning-dashboard-page";

const widgetLayoutMocks = vi.hoisted(() => ({
  gridLayoutProps: undefined as
    | {
        droppingWidgetType?: string;
        onTemplateChange: (template: ExtendedTemplateConfig) => void;
        template: ExtendedTemplateConfig;
      }
    | undefined,
  widgetDrawerProps: undefined as
    | {
        currentlyUsedWidgets: readonly string[];
        onWidgetDragEnd: () => void;
        onWidgetDragStart: (widgetType: string) => void;
      }
    | undefined,
}));

vi.mock("@patternfly/widgetized-dashboard", async () => {
  const React = await import("react");

  return {
    AddWidgetsButton: ({
      children,
      onClick,
    }: React.PropsWithChildren<{ onClick?: () => void }>) => (
      <button onClick={onClick} type="button">
        {children}
      </button>
    ),
    GridLayout: (props: unknown) => {
      widgetLayoutMocks.gridLayoutProps = props as never;
      return <div data-testid="provisioning-grid-layout" />;
    },
    WidgetDrawer: (props: unknown) => {
      widgetLayoutMocks.widgetDrawerProps = props as never;
      return (
        <div data-testid="provisioning-widget-drawer">
          {(props as { children?: React.ReactNode }).children}
        </div>
      );
    },
  };
});

function getGridLayoutProps() {
  if (!widgetLayoutMocks.gridLayoutProps) {
    throw new Error("GridLayout was not rendered");
  }

  return widgetLayoutMocks.gridLayoutProps;
}

function getWidgetDrawerProps() {
  if (!widgetLayoutMocks.widgetDrawerProps) {
    throw new Error("WidgetDrawer was not rendered");
  }

  return widgetLayoutMocks.widgetDrawerProps;
}

const messageCatalog = Object.fromEntries(
  Object.values(messages).map((message) => [
    message.id,
    message.defaultMessage,
  ]),
);

function renderProvisioningDashboard() {
  window.localStorage.clear();

  const dashboard = createDashboardOperations({
    controlPlane: {
      getOperationalMetrics: () =>
        Promise.resolve({
          lastSuccessfulRefresh: new Date(),
          metrics: [
            {
              id: "memory",
              total: "100",
              unit: "GiB",
              value: "5",
            },
          ],
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
        isAdmin
        navigation={{ collectionHref: "/", navigate: () => undefined }}
      >
        <ProvisioningDashboardPage
          metrics={{
            lastSuccessfulRefresh: new Date(),
            metrics: [
              {
                id: "memory",
                total: "100",
                unit: "GiB",
                value: "5",
              },
            ],
          }}
          onRefresh={() => undefined}
          onTimeRangeChange={() => undefined}
          timeRange={7}
        />
      </DashboardUiProvider>
    </IntlProvider>,
  );
}

function updateWidgetPresence(widgetType: string, include: boolean) {
  const gridLayout = getGridLayoutProps();

  const nextTemplate = Object.fromEntries(
    Object.entries(gridLayout.template).map(([variant, items]) => [
      variant,
      include
        ? [
            ...items,
            {
              h: 4,
              i: `${widgetType}#1`,
              title: widgetType,
              w: 1,
              widgetType,
              x: 0,
              y: 20,
            },
          ]
        : items.filter((item) => item.widgetType !== widgetType),
    ]),
  ) as ExtendedTemplateConfig;

  act(() => {
    gridLayout.onTemplateChange(nextTemplate);
  });
}

describe("ProvisioningDashboardPage widget drag and drop", () => {
  it("tracks drawer drags and guards repeated add-template updates", () => {
    renderProvisioningDashboard();

    const template = getGridLayoutProps().template;
    const nonSummaryWidgets = template.xl.filter(
      (item) => item.widgetType !== "system-summary",
    );
    expect(nonSummaryWidgets).toHaveLength(6);
    expect(nonSummaryWidgets.every((item) => item.h === 5)).toBe(true);
    expect(
      template.xl.filter((item) => item.y === 5).every((item) => item.h === 5),
    ).toBe(true);

    expect(getGridLayoutProps().droppingWidgetType).toBeUndefined();
    expect(getWidgetDrawerProps().currentlyUsedWidgets).toContain("memory");

    updateWidgetPresence("memory", false);
    expect(getWidgetDrawerProps().currentlyUsedWidgets).not.toContain("memory");

    act(() => {
      getWidgetDrawerProps().onWidgetDragStart("memory");
    });
    expect(getGridLayoutProps().droppingWidgetType).toBe("memory");

    updateWidgetPresence("memory", true);
    expect(
      getGridLayoutProps().template.xl.some(
        (item) => item.widgetType === "memory",
      ),
    ).toBe(true);

    act(() => {
      const gridLayout = getGridLayoutProps();
      gridLayout.onTemplateChange(gridLayout.template);
    });
    expect(
      getGridLayoutProps().template.xl.filter(
        (item) => item.widgetType === "memory",
      ),
    ).toHaveLength(1);

    act(() => {
      getWidgetDrawerProps().onWidgetDragEnd();
    });
    expect(getGridLayoutProps().droppingWidgetType).toBeUndefined();
  });
});
