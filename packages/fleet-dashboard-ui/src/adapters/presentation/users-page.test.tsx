import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { describe, expect, it, vi } from "vitest";

import type { Plane } from "../../domain/plane";
import type { UsersData } from "../../domain/users";
import { UsersPage } from "./users-page";

// Fixture order deliberately differs from every sort order under test: the
// default (gateways descending) and username-ascending orders are distinct.
const mockUsers = vi.hoisted(() => [
  {
    username: "alpha-user",
    gateways: 7,
    sandboxes: 64,
    clusters: ["euwest-1", "useast-1"],
  },
  { username: "beta-user", gateways: 1, sandboxes: 2, clusters: ["useast-1"] },
  { username: "gamma-user", gateways: 12, sandboxes: 0, clusters: [] },
]);

// The hook layer is exercised through the page component; the vi.mock keeps
// this a presentation test with no FleetApi wiring.
vi.mock("../query/hooks", () => ({
  useUsers: () => ({
    data: {
      data: { users: mockUsers },
      generatedAt: "2026-01-01T00:00:00Z",
      stale: false,
      error: null,
    } satisfies Plane<UsersData>,
    isPending: false,
    isError: false,
  }),
}));

function renderPage(): void {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <IntlProvider locale="en" defaultLocale="en">
      <QueryClientProvider client={queryClient}>
        <UsersPage />
      </QueryClientProvider>
    </IntlProvider>,
  );
}

function rowTexts(): string[] {
  return screen
    .getAllByRole("row")
    .slice(1)
    .map((row) => row.textContent);
}

describe("UsersPage", () => {
  it("renders summary cards with fleet totals", () => {
    renderPage();
    expect(screen.getByText("Distinct users")).toBeDefined();
    expect(screen.getByText("Gateway owners")).toBeDefined();
    expect(screen.getByText("20")).toBeDefined(); // 7 + 1 + 12 gateways
    expect(screen.getByText("66")).toBeDefined(); // 64 + 2 + 0 sandboxes
  });

  it("sorts users by gateways descending by default", () => {
    renderPage();
    expect(rowTexts()).toEqual([
      expect.stringContaining("gamma-user"),
      expect.stringContaining("alpha-user"),
      expect.stringContaining("beta-user"),
    ]);
  });

  it("re-sorts by username ascending when the User header is activated", async () => {
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: "User" }));
    expect(rowTexts()).toEqual([
      expect.stringContaining("alpha-user"),
      expect.stringContaining("beta-user"),
      expect.stringContaining("gamma-user"),
    ]);
  });

  it("renders spoke clusters per user, none shown when the user owns none", () => {
    renderPage();
    const rows = rowTexts();
    expect(rows[0]).toContain("gamma-user");
    // gamma-user owns gateways but the fixture gives it no clusters, so its
    // cluster cell renders the placeholder, not any cluster name.
    expect(rows[0]).not.toContain("useast-1");
    expect(rows[0]).not.toContain("euwest-1");
    expect(rows[1]).toContain("euwest-1, useast-1");
    expect(rows[2]).toContain("useast-1");
  });
});
