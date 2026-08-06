import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";

import { englishMessages } from "../../i18n/catalog";
import { AdminShell } from "./application-shell";

function RouteContent() {
  const { pathname } = useLocation();

  return <h1>{pathname}</h1>;
}

function renderShell(initialPath = "/admin") {
  return render(
    <IntlProvider locale="en" messages={englishMessages}>
      <MemoryRouter initialEntries={[initialPath]}>
        <Routes>
          <Route element={<AdminShell />}>
            <Route path="*" element={<RouteContent />} />
          </Route>
        </Routes>
      </MemoryRouter>
    </IntlProvider>,
  );
}

describe("AdminShell", () => {
  it("provides a focused gateway administration shell", () => {
    const { container } = renderShell();

    expect(
      screen
        .getByRole("link", { name: "Skip to content" })
        .getAttribute("href"),
    ).toBe("#main-content");
    expect(
      screen.getByRole("link", { name: "HyperShell Administration" }),
    ).toBeTruthy();
    expect(container.querySelector('img[src*="logo.png"]')).toBeTruthy();
    expect(
      screen
        .getByRole("link", { name: "Gateway directory" })
        .getAttribute("href"),
    ).toBe("/");

    expect(
      screen.queryByRole("navigation", { name: "Primary navigation" }),
    ).toBeNull();
    expect(screen.queryByText("Connect with the CLI")).toBeNull();
  });

  it("identifies an administrative gateway deep link", () => {
    renderShell("/admin/gateways/gateway-b");

    const breadcrumb = screen.getByRole("navigation", { name: "Breadcrumb" });
    expect(
      within(breadcrumb)
        .getByRole("link", { name: "Administration" })
        .getAttribute("href"),
    ).toBe("/admin");
    expect(within(breadcrumb).getByText("Gateway gateway-b")).toBeTruthy();
  });

  it("identifies the gateway provisioning route", () => {
    renderShell("/admin/gateways/new");

    const breadcrumb = screen.getByRole("navigation", { name: "Breadcrumb" });
    expect(within(breadcrumb).getByText("Administration")).toBeTruthy();
    expect(within(breadcrumb).getByText("Provision gateway")).toBeTruthy();
  });

  it("moves focus after administrative navigation", async () => {
    const user = userEvent.setup();
    renderShell("/admin/gateways/gateway-b");

    await user.click(
      screen.getByRole("link", { name: "HyperShell Administration" }),
    );

    expect(document.activeElement).toBe(
      screen.getByRole("heading", { level: 1, name: "/admin" }),
    );
  });
});
