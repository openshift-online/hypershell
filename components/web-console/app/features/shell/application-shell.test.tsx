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

async function openNavigation() {
  const user = userEvent.setup();
  const toggle = screen.getByRole("button", {
    name: "Toggle primary navigation",
  });
  if (toggle.getAttribute("aria-expanded") === "false") {
    await user.click(toggle);
  }

  return screen.getByRole("navigation", { name: "Primary navigation" });
}

describe("AdminShell", () => {
  it("provides infrastructure navigation only in the admin area", async () => {
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

    const navigation = await openNavigation();
    expect(within(navigation).getByText("Administration")).toBeTruthy();
    expect(
      within(navigation)
        .getByRole("link", { name: "Overview" })
        .getAttribute("aria-current"),
    ).toBe("page");
    expect(
      within(navigation).getByRole("link", { name: "Clusters" }),
    ).toBeTruthy();
    expect(
      within(navigation).getByRole("link", { name: "Gateways" }),
    ).toBeTruthy();
    expect(screen.queryByText("Connect with the CLI")).toBeNull();
  });

  it("collapses and expands the Administration group with the keyboard", async () => {
    const user = userEvent.setup();
    renderShell();
    const navigation = await openNavigation();
    const groupToggle = within(navigation).getByRole("button", {
      name: "Administration",
    });

    groupToggle.focus();
    await user.keyboard("{Enter}");

    expect(groupToggle.getAttribute("aria-expanded")).toBe("false");
    expect(
      within(navigation).queryByRole("link", { name: "Clusters" }),
    ).toBeNull();

    await user.keyboard("{Enter}");
    expect(groupToggle.getAttribute("aria-expanded")).toBe("true");
  });

  it("identifies an administrative gateway deep link", async () => {
    renderShell("/admin/gateways/gateway-b");

    const navigation = await openNavigation();
    expect(
      within(navigation)
        .getByRole("link", { name: "Gateways" })
        .getAttribute("aria-current"),
    ).toBe("page");

    const breadcrumb = screen.getByRole("navigation", { name: "Breadcrumb" });
    expect(within(breadcrumb).getByText("Administration")).toBeTruthy();
    expect(within(breadcrumb).getByText("Gateway gateway-b")).toBeTruthy();
  });

  it("marks the cluster destination and breadcrumb", async () => {
    renderShell("/admin/clusters");

    const navigation = await openNavigation();
    expect(
      within(navigation)
        .getByRole("link", { name: "Clusters" })
        .getAttribute("aria-current"),
    ).toBe("page");
    expect(
      within(screen.getByRole("navigation", { name: "Breadcrumb" })).getByText(
        "Clusters",
      ),
    ).toBeTruthy();
  });

  it("moves focus after administrative navigation", async () => {
    const user = userEvent.setup();
    renderShell();
    const navigation = await openNavigation();

    await user.click(
      within(navigation).getByRole("link", { name: "Clusters" }),
    );

    expect(document.activeElement).toBe(
      screen.getByRole("heading", { level: 1, name: "/admin/clusters" }),
    );
  });
});
