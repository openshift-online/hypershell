import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes } from "react-router";

import { englishMessages } from "../../i18n/catalog";
import { UserShell } from "./user-shell";

function renderShell(initialPath = "/") {
  return render(
    <IntlProvider locale="en" messages={englishMessages}>
      <MemoryRouter initialEntries={[initialPath]}>
        <Routes>
          <Route element={<UserShell />}>
            <Route
              path="*"
              element={
                <h1>{englishMessages["app.page.gatewayDirectory.title"]}</h1>
              }
            />
          </Route>
        </Routes>
      </MemoryRouter>
    </IntlProvider>,
  );
}

describe("UserShell", () => {
  it("keeps administration subtle in the branded HyperShell shell", () => {
    const { container } = renderShell();

    expect(screen.getByRole("link", { name: "HyperShell" })).toBeTruthy();
    expect(container.querySelector('img[src*="logo.png"]')).toBeTruthy();
    expect(screen.queryByRole("navigation")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Toggle primary navigation" }),
    ).toBeNull();
    expect(
      screen.getByRole("link", { name: "Administration" }).getAttribute("href"),
    ).toBe("/admin");
  });

  it("provides a gateway breadcrumb on detail routes", () => {
    renderShell("/gateways/gateway-a");

    const breadcrumb = screen.getByRole("navigation", { name: "Breadcrumb" });
    expect(
      within(breadcrumb).getByRole("link", { name: "Gateways" }),
    ).toBeTruthy();
    expect(within(breadcrumb).getByText("gateway-a")).toBeTruthy();
  });

  it("moves focus after user-facing navigation", async () => {
    const user = userEvent.setup();
    renderShell("/gateways/gateway-a");

    await user.click(screen.getByRole("link", { name: "HyperShell" }));

    expect(document.activeElement).toBe(
      screen.getByRole("heading", { level: 1, name: "OpenShell gateways" }),
    );
  });
});
