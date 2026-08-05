import { render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router";

import { englishMessages } from "../../i18n/catalog";
import {
  ClientsPage,
  GatewayPage,
  GatewaysPage,
  KeysPage,
  OverviewPage,
  SectorPage,
  SectorsPage,
  SettingsPage,
} from "./shell-pages";

function renderPage(Page: () => React.ReactNode) {
  return render(
    <IntlProvider locale="en" messages={englishMessages}>
      <MemoryRouter>
        <Page />
      </MemoryRouter>
    </IntlProvider>,
  );
}

describe("shell pages", () => {
  it("presents the console overview and its primary next step", () => {
    renderPage(OverviewPage);

    expect(
      screen.getByRole("heading", { level: 1, name: "Overview" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "View sectors" }).getAttribute("href"),
    ).toBe("/fleets");
    expect(screen.getAllByRole("heading", { level: 2 })).toHaveLength(3);
  });

  it.each([
    [SectorsPage, "Sectors", "No sectors to display"],
    [SectorPage, "Sector overview", "No resource activity to display"],
    [GatewaysPage, "Gateways", "No gateways to display"],
    [GatewayPage, "Gateway details", "Gateway details are not available"],
    [SettingsPage, "Settings", "No settings to display"],
    [ClientsPage, "Clients", "No clients to display"],
    [KeysPage, "Keys", "No keys to display"],
  ])(
    "renders the %s route purpose and empty state",
    (Page, title, emptyTitle) => {
      renderPage(Page);

      expect(
        screen.getByRole("heading", { level: 1, name: title }),
      ).toBeTruthy();
      expect(
        screen.getByRole("heading", { level: 2, name: emptyTitle }),
      ).toBeTruthy();
    },
  );
});
