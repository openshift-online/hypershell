import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router";

import { englishMessages } from "../../i18n/catalog";
import { getSectorDestination, SectorSelector } from "./sector-selector";

function renderSelector(
  overrides: Partial<React.ComponentProps<typeof SectorSelector>> = {},
) {
  return render(
    <IntlProvider locale="en" messages={englishMessages}>
      <MemoryRouter>
        <SectorSelector
          onSelectSector={() => undefined}
          sectors={[{ id: "sector-a", name: "Alpha sector" }]}
          selectedSectorId="sector-a"
          {...overrides}
        />
      </MemoryRouter>
    </IntlProvider>,
  );
}

describe("getSectorDestination", () => {
  it.each([
    ["/fleets/sector-a", "/fleets/sector-b"],
    ["/fleets/sector-a/gateways", "/fleets/sector-b/gateways"],
    ["/fleets/sector-a/gateways/gateway-a", "/fleets/sector-b/gateways"],
    ["/fleets/sector-a/clients", "/fleets/sector-b/clients"],
    ["/fleets/sector-a/keys", "/fleets/sector-b/keys"],
    ["/fleets/sector-a/settings", "/fleets/sector-b/settings"],
  ])("maps %s to %s", (pathname, expected) => {
    expect(getSectorDestination(pathname, "sector-b")).toBe(expected);
  });
});

describe("SectorSelector", () => {
  it("exposes the selected value and reports a list failure", async () => {
    const user = userEvent.setup();
    renderSelector({ hasError: true });

    await user.click(
      screen.getByRole("button", {
        name: "Select sector, currently Alpha sector",
      }),
    );

    expect(screen.getByText("Sectors could not be loaded")).toBeTruthy();
    expect(screen.getByRole("link", { name: "View sectors" })).toBeTruthy();
  });
});
