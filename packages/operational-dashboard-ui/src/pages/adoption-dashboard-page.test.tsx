import { fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { IntlProvider } from "react-intl";

import { createDashboardOperations } from "../application/dashboard-operations";
import {
  mockAdoptionDashboardMetrics,
  mockAdoptionDashboardUnavailableMetrics,
} from "../fixtures/mock-adoption-dashboard-metrics";
import { messages } from "../messages";
import { DashboardUiProvider } from "../dashboard-ui-provider";
import { AdoptionDashboardPage } from "./adoption-dashboard-page";

const messageCatalog = Object.fromEntries(
  Object.values(messages).map((message) => [
    message.id,
    message.defaultMessage,
  ]),
);

function renderPage(
  metrics = mockAdoptionDashboardMetrics,
): ReturnType<typeof render> {
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

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <IntlProvider locale="en" messages={messageCatalog}>
      <QueryClientProvider client={queryClient}>
        <DashboardUiProvider
          dashboard={dashboard}
          isAdmin
          navigation={{ collectionHref: "/", navigate: vi.fn() }}
        >
          <AdoptionDashboardPage metrics={metrics} />
        </DashboardUiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

function getValue(label: string): string {
  const term = screen
    .getAllByText(label)
    .find((element) => element.tagName === "DT")
    ?.closest("div");
  expect(term).not.toBeNull();
  return (term?.querySelector("dd")?.textContent ?? "").trim();
}

describe("AdoptionDashboardPage", () => {
  it("renders aggregate adoption values", () => {
    renderPage();

    expect(
      screen.getByRole("heading", { name: /450 registered users/u }),
    ).toBeTruthy();
    expect(getValue("New registrations (selected period)")).toBe("48");
    expect(getValue("Daily active users")).toBe("186");
    expect(getValue("Weekly active users")).toBe("312");
    expect(getValue("Monthly active users")).toBe("398");
    expect(getValue("Inactive users (30+ days)")).toBe("52");
    expect(getValue("First gateway conversion")).toBe("306");
    expect(getValue("7-day retention")).toBe("78.0");
  });

  it("changes the selected registration window without a page reload", () => {
    renderPage();

    fireEvent.change(screen.getByLabelText("Time range"), {
      target: { value: "30" },
    });

    expect(getValue("New registrations (selected period)")).toBe("123");
  });

  it("renders localized unavailable states for omitted fields", () => {
    renderPage(mockAdoptionDashboardUnavailableMetrics);

    expect(
      screen.getByRole("heading", { name: /450 registered users/u }),
    ).toBeTruthy();
    expect(screen.getAllByText("Metric unavailable").length).toBeGreaterThan(1);
    expect(
      screen.getAllByText("This information is not currently available.")
        .length,
    ).toBeGreaterThan(1);
  });
});
